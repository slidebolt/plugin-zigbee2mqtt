package main

import (
	"testing"

	runner "github.com/slidebolt/sdk-runner"
	"github.com/slidebolt/sdk-types"
)

func TestNullStateThenDiscoveryDoesNotPanic(t *testing.T) {
	p := NewPlugin()
	p.OnInitialize(runner.Config{}, types.Storage{Data: []byte("null")})

	topic := "homeassistant/light/test-node/bedroom_lamp/config"
	payload := []byte(`{
		"name":"Bedroom Lamp",
		"unique_id":"z2m_bedroom_lamp",
		"device":{
			"identifiers":["zigbee2mqtt/bedroom_lamp"],
			"name":"Bedroom Lamp"
		},
		"state_topic":"zigbee2mqtt/bedroom_lamp",
		"command_topic":"zigbee2mqtt/bedroom_lamp/set",
		"payload_on":"ON",
		"payload_off":"OFF"
	}`)

	// This currently panics in production when discovered map is nil.
	p.handleDiscoveryMessage(topic, payload)

	p.mu.RLock()
	defer p.mu.RUnlock()
	if got := len(p.discovered); got != 1 {
		t.Fatalf("expected 1 discovered entity, got %d", got)
	}
}
