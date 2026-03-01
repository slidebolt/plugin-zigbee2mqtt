package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
)

type haDiscoveryPayload struct {
	Name          string       `json:"name"`
	UniqueID      string       `json:"unique_id"`
	Device        haDeviceInfo `json:"device"`
	StateTopic    string       `json:"state_topic"`
	CommandTopic  string       `json:"command_topic"`
	PayloadOn     interface{}  `json:"payload_on"`
	PayloadOff    interface{}  `json:"payload_off"`
	ValueTemplate string       `json:"value_template"`
}

type haDeviceInfo struct {
	Identifiers  []interface{} `json:"identifiers"`
	Name         string        `json:"name"`
	Model        string        `json:"model"`
	Manufacturer string        `json:"manufacturer"`
}

func parseDiscovery(topic string, payload []byte) (*haDiscoveryPayload, string, error) {
	log.Printf("plugin-zigbee2mqtt: parsing discovery on %q", topic)
	// homeassistant/<type>/<node_id>/<object_id>/config OR homeassistant/<type>/<id>/config
	parts := strings.Split(topic, "/")
	if len(parts) < 4 || parts[len(parts)-1] != "config" {
		return nil, "", fmt.Errorf("invalid discovery topic")
	}

	entityType := parts[1]
	var data haDiscoveryPayload
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Printf("plugin-zigbee2mqtt: unmarshal failed for %q: %v", topic, err)
		return nil, "", err
	}

	if data.UniqueID == "" {
		return nil, "", fmt.Errorf("missing unique_id")
	}

	return &data, entityType, nil
}

func payloadToString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func extractValueKey(template string) string {
	if template == "" {
		return ""
	}

	t := strings.ReplaceAll(template, " ", "")
	t = strings.ReplaceAll(t, "']['", ".")
	t = strings.ReplaceAll(t, "'][\"", ".")
	t = strings.ReplaceAll(t, "\"]['", ".")
	t = strings.ReplaceAll(t, "\"][\"", ".")
	t = strings.ReplaceAll(t, "['", ".")
	t = strings.ReplaceAll(t, "[\"", ".")
	t = strings.ReplaceAll(t, "']", "")
	t = strings.ReplaceAll(t, "\"]", "")

	if strings.Contains(t, "value_json.") {
		start := strings.Index(t, "value_json.") + len("value_json.")
		end := start
		for end < len(t) {
			ch := t[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' {
				end++
				continue
			}
			break
		}
		if end > start {
			return t[start:end]
		}
	}
	return ""
}
