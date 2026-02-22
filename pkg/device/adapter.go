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
	bundle sdk.Bundle
	client logic.MQTTClient
	mu     sync.Mutex
	wired  map[string]bool
}

func NewMQTTAdapter(b sdk.Bundle, client logic.MQTTClient) *MQTTAdapter {
	return &MQTTAdapter{
		bundle: b,
		client: client,
		wired:  make(map[string]bool),
	}
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
	} else {
		sdkType := mapType(haType)
		ent, _ = dev.CreateEntity(sdkType)
		ent.UpdateMetadata(data.Name, entID)
	}
	ent.UpdateRaw(map[string]interface{}{
		"state_topic":   data.StateTopic,
		"command_topic": data.CommandTopic,
		"payload_on":    payloadOn,
		"payload_off":   payloadOff,
	})

	a.ensureWired(string(entID), ent, data.StateTopic, data.CommandTopic, payloadOn, payloadOff)
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
	a.ensureWired(string(ent.ID()), ent, stateTopic, commandTopic, payloadOn, payloadOff)
}

func (a *MQTTAdapter) ensureWired(entKey string, ent sdk.Entity, stateTopic, commandTopic, payloadOn, payloadOff string) {
	a.mu.Lock()
	if a.wired[entKey] {
		a.mu.Unlock()
		return
	}
	a.wired[entKey] = true
	a.mu.Unlock()

	// Listen for state (including JSON payloads like Zigbee2MQTT)
	if stateTopic != "" {
		go func() {
			if err := a.client.Subscribe(stateTopic, func(t string, p []byte) {
				stateMap := parseStatePayload(p)
				power, state := payloadToState(p, stateMap, payloadOn)
				_ = ent.UpdateState("active")

				// Publish richer live state for UI cards/controls.
				if len(stateMap) > 0 {
					stateMap["power"] = state
					stateMap["state"] = power
					_ = ent.Publish(fmt.Sprintf("entity.%s.state", ent.ID()), stateMap)
				}
			}); err != nil {
				a.bundle.Log().Error("MQTT state subscribe failed (%s): %v", stateTopic, err)
			}
		}()
	}

	// Handle Commands
	ent.OnCommand(func(cmd string, p map[string]interface{}) {
		if commandTopic == "" {
			return
		}
		mqttPayload, statePatch := buildMQTTPayloadForCommand(cmd, p, payloadOn, payloadOff)
		if mqttPayload != "" {
			_ = a.client.Publish(commandTopic, mqttPayload)
			power, state := payloadToState([]byte(fmt.Sprintf("%v", statePatch["state"])), statePatch, payloadOn)
			statePatch["power"] = state
			statePatch["state"] = power
			_ = ent.UpdateState("active")
			_ = ent.Publish(fmt.Sprintf("entity.%s.state", ent.ID()), statePatch)
		}
	})
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
