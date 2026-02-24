package device

import (
	"encoding/json"
	"fmt"
	"math"
	"github.com/slidebolt/plugin-zigbee2mqtt/pkg/logic"
	"github.com/slidebolt/plugin-sdk"
	"strings"
	"sync"
)

type MQTTAdapter struct {
	bundle    sdk.Bundle
	client    logic.MQTTClient
	mu        sync.Mutex
	wired     map[string]bool            // entity UUID → already wired
	devWired  map[sdk.UUID]bool          // device UUID → device OnCommand registered
	topicSubs map[string][]func([]byte)  // MQTT topic → registered state callbacks
	wg        sync.WaitGroup
}

func NewMQTTAdapter(b sdk.Bundle, client logic.MQTTClient) *MQTTAdapter {
	return &MQTTAdapter{
		bundle:    b,
		client:    client,
		wired:     make(map[string]bool),
		devWired:  make(map[sdk.UUID]bool),
		topicSubs: make(map[string][]func([]byte)),
	}
}

func (a *MQTTAdapter) Wait() {
	a.wg.Wait()
}

func (a *MQTTAdapter) HandleDiscovery(topic string, payload []byte) {
	data, haType, err := logic.ParseDiscovery(topic, payload)
	if err != nil {
		return
	}
	if len(data.Device.Identifiers) == 0 {
		return
	}

	payloadOn := logic.PayloadToString(data.PayloadOn)
	payloadOff := logic.PayloadToString(data.PayloadOff)

	deviceID := fmt.Sprintf("mqtt-%v", data.Device.Identifiers[0])
	sid := sdk.SourceID(deviceID)

	var dev sdk.Device
	if obj, ok := a.bundle.GetBySourceID(sid); ok {
		dev = obj.(sdk.Device)
	} else {
		dev, _ = a.bundle.CreateDevice()
		dev.UpdateMetadata(data.Device.Name, sid)
		dev.UpdateRaw(map[string]interface{}{
			"model":        data.Device.Model,
			"manufacturer": data.Device.Manufacturer,
		})
	}

	// Entity
	entID := sdk.SourceID(data.UniqueID)
	var ent sdk.Entity
	if obj, ok := a.bundle.GetBySourceID(entID); ok {
		ent = obj.(sdk.Entity)
		// Refresh metadata and capabilities on every discovery pass so that
		// a reload_bundle acts as a forced full refresh (Z2M sends retained
		// discovery messages immediately on subscribe).
		ent.UpdateMetadata(data.Name, entID)
		newCaps := capsForType(haType)
		if len(newCaps) > 0 {
			existing := ent.Metadata().Capabilities
			if !slicesEqual(existing, newCaps) {
				ent.UpdateCapabilities(newCaps)
			}
		}
	} else {
		sdkType := mapType(haType)
		caps := capsForType(haType)
		if len(caps) > 0 {
			ent, _ = dev.CreateEntityEx(sdkType, caps)
		} else {
			ent, _ = dev.CreateEntity(sdkType)
		}
		ent.UpdateMetadata(data.Name, entID)
	}

	valueKey := logic.ExtractValueKey(data.ValueTemplate)

	ent.UpdateRaw(map[string]interface{}{
		"state_topic":   data.StateTopic,
		"command_topic": data.CommandTopic,
		"payload_on":    payloadOn,
		"payload_off":   payloadOff,
		"value_key":     valueKey,
	})

	a.ensureWired(ent, dev, data.StateTopic, data.CommandTopic, payloadOn, payloadOff)
}

func (a *MQTTAdapter) WireExistingEntity(ent sdk.Entity) {
	raw := ent.Raw()
	stateTopic, _ := raw["state_topic"].(string)
	commandTopic, _ := raw["command_topic"].(string)
	payloadOn, _ := raw["payload_on"].(string)
	payloadOff, _ := raw["payload_off"].(string)
	if stateTopic == "" && commandTopic == "" {
		return
	}
	var dev sdk.Device
	if d, err := a.bundle.GetDevice(ent.DeviceID()); err == nil {
		dev = d
	}
	a.ensureWired(ent, dev, stateTopic, commandTopic, payloadOn, payloadOff)
}

func (a *MQTTAdapter) ensureWired(ent sdk.Entity, dev sdk.Device, stateTopic, commandTopic, payloadOn, payloadOff string) {
	entKey := string(ent.ID())

	a.mu.Lock()
	if a.wired[entKey] {
		a.mu.Unlock()
		return
	}
	a.wired[entKey] = true
	a.mu.Unlock()

	// State: fan-out per MQTT topic so each topic is subscribed exactly once
	// regardless of how many entities share the same state_topic.
	if stateTopic != "" {
		stateHandler := func(p []byte) {
			stateMap := parseStatePayload(p)
			power, _ := payloadToState(p, stateMap, payloadOn)

			finalProps := make(map[string]interface{})
			raw := ent.Raw()
			vKey, _ := raw["value_key"].(string)

			if vKey != "" {
				// Specific binding: Extract only the target value (supports nested paths like "update.state")
				if val, ok := getPropertyAtPath(stateMap, vKey); ok {
					finalProps["value"] = val
				}
				finalProps["power"] = power
			} else {
				// No specific binding: Pass through everything (e.g. LIGHT control)
				for k, v := range stateMap {
					finalProps[k] = v
				}
				finalProps["power"] = power
			}

			if len(finalProps) > 0 {
				_ = ent.SetProperties(finalProps)
			}
		}

		a.mu.Lock()
		existing := a.topicSubs[stateTopic]
		a.topicSubs[stateTopic] = append(existing, stateHandler)
		firstSub := len(existing) == 0
		a.mu.Unlock()

		if firstSub {
			// First entity on this topic — subscribe to the MQTT broker once.
			a.wg.Add(1)
			go func() {
				defer a.wg.Done()
				if err := a.client.Subscribe(stateTopic, func(t string, p []byte) {
					a.mu.Lock()
					handlers := a.topicSubs[t]
					a.mu.Unlock()
					for _, h := range handlers {
						h(p)
					}
				}); err != nil {
					a.bundle.Log().Error("MQTT state subscribe failed (%s): %v", stateTopic, err)
				}
			}()
		}
	}

	// Command handler (entity-level and, for controllable types, device-level).
	cmdHandler := func(cmd string, p map[string]interface{}) {
		if commandTopic == "" {
			return
		}
		mqttPayload, statePatch := buildMQTTPayloadForCommand(cmd, p, payloadOn, payloadOff)
		if mqttPayload != "" {
			_ = a.client.Publish(commandTopic, mqttPayload)
			power, _ := payloadToState([]byte(fmt.Sprintf("%v", statePatch["state"])), statePatch, payloadOn)

			// Same filtering logic for commands
			finalPatch := make(map[string]interface{})
			vKey, _ := ent.Raw()["value_key"].(string)

			if vKey != "" {
				if val, ok := getPropertyAtPath(statePatch, vKey); ok {
					finalPatch["value"] = val
				}
				finalPatch["power"] = power
			} else {
				for k, v := range statePatch {
					finalPatch[k] = v
				}
				finalPatch["power"] = power
			}

			_ = ent.SetProperties(finalPatch)
		}
	}

	ent.OnCommand(cmdHandler)

	// Also register on the device for controllable types so that
	// commands sent to device.{uuid}.command are handled.
	if dev != nil && isControllable(ent) {
		a.mu.Lock()
		alreadyDev := a.devWired[dev.ID()]
		if !alreadyDev {
			a.devWired[dev.ID()] = true
		}
		a.mu.Unlock()
		if !alreadyDev {
			dev.OnCommand(cmdHandler)
		}
	}
}

func getPropertyAtPath(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = m
	for _, p := range parts {
		if curMap, ok := current.(map[string]interface{}); ok {
			if val, ok := curMap[p]; ok {
				current = val
			} else {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	return current, true
}

func isControllable(ent sdk.Entity) bool {
	t := ent.Metadata().Type
	return t == sdk.TYPE_LIGHT || t == sdk.TYPE_SWITCH || t == sdk.TYPE_COVER
}

func capsForType(haType string) []string {
	switch haType {
	case "light":
		return []string{sdk.CAP_BRIGHTNESS, sdk.CAP_RGB, sdk.CAP_TEMPERATURE}
	default:
		return nil
	}
}

func parseStatePayload(raw []byte) map[string]interface{} {
	out := map[string]interface{}{}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err == nil {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func payloadToState(raw []byte, parsed map[string]interface{}, payloadOn string) (bool, string) {
	val := strings.TrimSpace(string(raw))
	if val != "" {
		if payloadOn != "" && val == payloadOn {
			return true, "on"
		}
		switch strings.ToLower(val) {
		case "on", "true", "1":
			return true, "on"
		case "off", "false", "0":
			return false, "off"
		}
	}

	for _, k := range []string{"state", "power", "value"} {
		v, ok := parsed[k]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case bool:
			if x {
				return true, "on"
			}
			return false, "off"
		case string:
			if payloadOn != "" && x == payloadOn {
				return true, "on"
			}
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "on", "true", "1":
				return true, "on"
			case "off", "false", "0":
				return false, "off"
			}
		case float64:
			if x != 0 {
				return true, "on"
			}
			return false, "off"
		}
	}

	return false, "off"
}

func buildMQTTPayloadForCommand(cmd string, p map[string]interface{}, payloadOn, payloadOff string) (string, map[string]interface{}) {
	switch cmd {
	case "TurnOn":
		state := map[string]interface{}{"state": "ON", "power": true}
		if payloadOn != "" {
			return payloadOn, state
		}
		return marshalJSON(map[string]interface{}{"state": "ON"}), state
	case "TurnOff":
		state := map[string]interface{}{"state": "OFF", "power": false}
		if payloadOff != "" {
			return payloadOff, state
		}
		return marshalJSON(map[string]interface{}{"state": "OFF"}), state
	case "SetBrightness":
		level := intFromAny(p["level"])
		return marshalJSON(map[string]interface{}{"state": "ON", "brightness": level}), map[string]interface{}{
			"state": "ON", "power": true, "brightness": level,
		}
	case "SetTemperature":
		kelvin := intFromAny(p["kelvin"])
		if kelvin <= 0 {
			return "", map[string]interface{}{}
		}
		mired := int(math.Round(1000000.0 / float64(kelvin)))
		return marshalJSON(map[string]interface{}{
			"state":             "ON",
			"color_temp_kelvin": kelvin,
			"color_temp":        mired,
		}), map[string]interface{}{
			"state": "ON", "power": true, "kelvin": kelvin, "temperature": kelvin, "color_temp": mired,
		}
	case "SetRGB":
		r := intFromAny(p["r"])
		g := intFromAny(p["g"])
		b := intFromAny(p["b"])
		return marshalJSON(map[string]interface{}{
			"state": "ON",
			"color": map[string]interface{}{"r": r, "g": g, "b": b},
		}), map[string]interface{}{
			"state": "ON", "power": true, "r": r, "g": g, "b": b,
		}
	default:
		return "", map[string]interface{}{}
	}
}

func marshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func intFromAny(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case string:
		n := strings.TrimSpace(x)
		if n == "" {
			return 0
		}
		var f float64
		if err := json.Unmarshal([]byte(n), &f); err == nil {
			return int(f)
		}
	}
	return 0
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mapType(haType string) sdk.EntityType {
	switch haType {
	case "switch":
		return sdk.TYPE_SWITCH
	case "light":
		return sdk.TYPE_LIGHT
	case "binary_sensor":
		return sdk.TYPE_BINARY_SENSOR
	case "sensor":
		return sdk.TYPE_SENSOR
	case "cover":
		return sdk.TYPE_COVER
	default:
		return sdk.TYPE_SENSOR
	}
}
