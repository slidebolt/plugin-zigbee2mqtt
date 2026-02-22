package logic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type HADiscoveryPayload struct {
	Name         string            `json:"name"`
	UniqueID     string            `json:"unique_id"`
	Device       HADeviceInfo      `json:"device"`
	StateTopic   string            `json:"state_topic"`
	CommandTopic string            `json:"command_topic"`
	PayloadOn    interface{}       `json:"payload_on"`
	PayloadOff   interface{}       `json:"payload_off"`
	ValueTemplate string           `json:"value_template"`
}

type HADeviceInfo struct {
	Identifiers  []interface{} `json:"identifiers"`
	Name         string        `json:"name"`
	Model        string        `json:"model"`
	Manufacturer string        `json:"manufacturer"`
}

func ParseDiscovery(topic string, payload []byte) (*HADiscoveryPayload, string, error) {
	// homeassistant/<type>/<node_id>/<object_id>/config
	parts := strings.Split(topic, "/")
	if len(parts) < 5 || parts[len(parts)-1] != "config" {
		return nil, "", fmt.Errorf("invalid discovery topic")
	}

	entityType := parts[1]
	var data HADiscoveryPayload
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, "", err
	}

	if data.UniqueID == "" {
		return nil, "", fmt.Errorf("missing unique_id")
	}

	return &data, entityType, nil
}

func PayloadToString(v interface{}) string {
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
