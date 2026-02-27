package main

import "testing"

func TestParseDiscovery(t *testing.T) {
	topic := "homeassistant/light/node/object/config"
	payload := []byte(`{"name":"Kitchen","unique_id":"abc123","device":{"name":"Kitchen Lamp"},"state_topic":"zigbee2mqtt/kitchen","command_topic":"zigbee2mqtt/kitchen/set","payload_on":"ON","payload_off":"OFF","value_template":"{{ value_json['state'] }}"}`)

	data, entityType, err := parseDiscovery(topic, payload)
	if err != nil {
		t.Fatalf("parseDiscovery failed: %v", err)
	}
	if entityType != "light" {
		t.Fatalf("unexpected entity type: %q", entityType)
	}
	if data.UniqueID != "abc123" {
		t.Fatalf("unexpected unique_id: %q", data.UniqueID)
	}
	if got := extractValueKey(data.ValueTemplate); got != "state" {
		t.Fatalf("unexpected value key: %q", got)
	}
}

func TestParseDiscoveryRejectsInvalidTopic(t *testing.T) {
	if _, _, err := parseDiscovery("zigbee2mqtt/bridge/devices", []byte(`{}`)); err == nil {
		t.Fatal("expected invalid discovery topic error")
	}
}
