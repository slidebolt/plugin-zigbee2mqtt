package main

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/slidebolt/sdk-entities/light"
	entityswitch "github.com/slidebolt/sdk-entities/switch"
	"github.com/slidebolt/sdk-types"
)

func normalizePayloadForDomain(domain string, payload []byte) ([]byte, bool) {
	switch domain {
	case light.Type:
		return normalizeLightPayloadToEventPayload(payload)
	case entityswitch.Type:
		return normalizeSwitchPayloadToEventPayload(payload)
	case "binary_sensor":
		return normalizeBinarySensorPayloadToEventPayload(payload)
	case "sensor":
		return normalizeSensorPayloadToEventPayload(payload)
	case "cover":
		return normalizeCoverPayloadToEventPayload(payload)
	default:
		return nil, false
	}
}

func normalizeLightPayloadToEventPayload(payload []byte) ([]byte, bool) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, false
	}
	if strings.EqualFold(raw, "ON") {
		b, _ := json.Marshal(light.Event{Type: light.ActionTurnOn})
		return b, true
	}
	if strings.EqualFold(raw, "OFF") {
		b, _ := json.Marshal(light.Event{Type: light.ActionTurnOff})
		return b, true
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false
	}

	var evt light.Event
	if state, ok := m["state"].(string); ok && strings.EqualFold(state, "OFF") {
		evt.Type = light.ActionTurnOff
	} else if b, ok := toFloat64(m["brightness"]); ok {
		brightness := z2mBrightnessToSDKBrightness(b)
		evt.Type = light.ActionSetBrightness
		evt.Brightness = &brightness
	} else if ct, ok := toFloat64(m["color_temp"]); ok && ct > 0 {
		temp := int(math.Round(1000000.0 / ct))
		evt.Type = light.ActionSetTemperature
		evt.Temperature = &temp
	} else if color, ok := m["color"].(map[string]any); ok {
		r, rok := toFloat64(color["r"])
		g, gok := toFloat64(color["g"])
		b, bok := toFloat64(color["b"])
		if rok && gok && bok {
			rgb := []int{
				clampInt(int(math.Round(r)), 0, 255),
				clampInt(int(math.Round(g)), 0, 255),
				clampInt(int(math.Round(b)), 0, 255),
			}
			evt.Type = light.ActionSetRGB
			evt.RGB = &rgb
		}
	}

	if evt.Type == "" {
		if state, ok := m["state"].(string); ok && strings.EqualFold(state, "ON") {
			evt.Type = light.ActionTurnOn
		} else {
			return nil, false
		}
	}

	if err := light.ValidateEvent(evt); err != nil {
		return nil, false
	}
	out, err := json.Marshal(evt)
	if err != nil {
		return nil, false
	}
	return out, true
}

func applyLightStateFromZ2MPayload(store light.Store, payload []byte) error {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil
	}

	if strings.EqualFold(raw, "ON") {
		return store.SetReportedFromEvent(light.Event{Type: light.ActionTurnOn})
	}
	if strings.EqualFold(raw, "OFF") {
		return store.SetReportedFromEvent(light.Event{Type: light.ActionTurnOff})
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return err
	}

	if state, ok := m["state"].(string); ok {
		if strings.EqualFold(state, "ON") {
			if err := store.SetReportedFromEvent(light.Event{Type: light.ActionTurnOn}); err != nil {
				return err
			}
		}
		if strings.EqualFold(state, "OFF") {
			if err := store.SetReportedFromEvent(light.Event{Type: light.ActionTurnOff}); err != nil {
				return err
			}
		}
	}

	if b, ok := toFloat64(m["brightness"]); ok {
		brightness := z2mBrightnessToSDKBrightness(b)
		if err := store.SetReportedFromEvent(light.Event{Type: light.ActionSetBrightness, Brightness: &brightness}); err != nil {
			return err
		}
	}

	if ct, ok := toFloat64(m["color_temp"]); ok && ct > 0 {
		temp := int(math.Round(1000000.0 / ct))
		if err := store.SetReportedFromEvent(light.Event{Type: light.ActionSetTemperature, Temperature: &temp}); err != nil {
			return err
		}
	}

	if color, ok := m["color"].(map[string]any); ok {
		r, rok := toFloat64(color["r"])
		g, gok := toFloat64(color["g"])
		b, bok := toFloat64(color["b"])
		if rok && gok && bok {
			rgb := []int{
				clampInt(int(math.Round(r)), 0, 255),
				clampInt(int(math.Round(g)), 0, 255),
				clampInt(int(math.Round(b)), 0, 255),
			}
			if err := store.SetReportedFromEvent(light.Event{Type: light.ActionSetRGB, RGB: &rgb}); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizeSwitchPayloadToEventPayload(payload []byte) ([]byte, bool) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, false
	}
	if strings.EqualFold(raw, "ON") {
		b, _ := json.Marshal(entityswitch.Event{Type: entityswitch.ActionTurnOn})
		return b, true
	}
	if strings.EqualFold(raw, "OFF") {
		b, _ := json.Marshal(entityswitch.Event{Type: entityswitch.ActionTurnOff})
		return b, true
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false
	}

	var evt entityswitch.Event
	if state, ok := m["state"].(string); ok {
		if strings.EqualFold(state, "ON") {
			evt.Type = entityswitch.ActionTurnOn
		} else if strings.EqualFold(state, "OFF") {
			evt.Type = entityswitch.ActionTurnOff
		}
	}
	if evt.Type == "" {
		if v, ok := m["state"].(bool); ok {
			if v {
				evt.Type = entityswitch.ActionTurnOn
			} else {
				evt.Type = entityswitch.ActionTurnOff
			}
		}
	}
	if evt.Type == "" {
		return nil, false
	}
	if err := entityswitch.ValidateEvent(evt); err != nil {
		return nil, false
	}
	out, err := json.Marshal(evt)
	if err != nil {
		return nil, false
	}
	return out, true
}

func normalizeBinarySensorPayloadToEventPayload(payload []byte) ([]byte, bool) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, false
	}
	if strings.EqualFold(raw, "ON") {
		b, _ := json.Marshal(map[string]any{"type": "active", "state": true})
		return b, true
	}
	if strings.EqualFold(raw, "OFF") {
		b, _ := json.Marshal(map[string]any{"type": "inactive", "state": false})
		return b, true
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false
	}
	if v, ok := m["state"].(bool); ok {
		m["type"] = "inactive"
		if v {
			m["type"] = "active"
		}
		out, err := json.Marshal(m)
		return out, err == nil
	}
	if v, ok := m["state"].(string); ok {
		active := strings.EqualFold(v, "ON") || strings.EqualFold(v, "true") || strings.EqualFold(v, "active")
		m["state"] = active
		m["type"] = "inactive"
		if active {
			m["type"] = "active"
		}
		out, err := json.Marshal(m)
		return out, err == nil
	}
	return nil, false
}

func normalizeSensorPayloadToEventPayload(payload []byte) ([]byte, bool) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, false
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		m["type"] = "value_changed"
		out, err := json.Marshal(m)
		return out, err == nil
	}

	if f, ok := toFloat64(raw); ok {
		b, _ := json.Marshal(map[string]any{"type": "value_changed", "value": f})
		return b, true
	}
	return nil, false
}

func normalizeCoverPayloadToEventPayload(payload []byte) ([]byte, bool) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, false
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		m["type"] = "state_changed"
		out, err := json.Marshal(m)
		return out, err == nil
	}
	return nil, false
}

func applySwitchStateFromZ2MPayload(store entityswitch.Store, payload []byte) error {
	if evtPayload, ok := normalizeSwitchPayloadToEventPayload(payload); ok {
		evt := types.Event{Payload: evtPayload}
		se, err := entityswitch.ParseEvent(evt)
		if err != nil {
			return err
		}
		return store.SetReportedFromEvent(se)
	}
	return nil
}

func z2mBrightnessToSDKBrightness(v float64) int {
	v = math.Max(0, math.Min(254, v))
	return clampInt(int(math.Round(v*100.0/254.0)), 0, 100)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
