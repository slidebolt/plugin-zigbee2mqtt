package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slidebolt/sdk-entities/light"
	"github.com/slidebolt/sdk-types"
)

type stubMQTTClient struct {
	publishedTopic   string
	publishedPayload string
}

func (s *stubMQTTClient) Connect() error { return nil }
func (s *stubMQTTClient) Subscribe(topic string, callback func(topic string, payload []byte)) error {
	return nil
}
func (s *stubMQTTClient) Publish(topic string, payload interface{}) error {
	s.publishedTopic = topic
	switch v := payload.(type) {
	case string:
		s.publishedPayload = v
	case []byte:
		s.publishedPayload = string(v)
	}
	return nil
}
func (s *stubMQTTClient) Disconnect() {}

func TestOnEventNormalizesBrightnessToSDKRange(t *testing.T) {
	p := NewPlugin()
	entity := types.Entity{
		ID:       "z2m-entity-light-1",
		DeviceID: "z2m-device-1",
		Domain:   light.Type,
	}
	normalized, ok := normalizeLightPayloadToEventPayload([]byte(`{"state":"ON","brightness":139}`))
	if !ok {
		t.Fatal("failed to normalize raw z2m payload")
	}
	var payload types.GenericPayload
	if err := json.Unmarshal(normalized, &payload); err != nil {
		t.Fatalf("failed to decode normalized payload: %v", err)
	}
	evt := types.EventTyped[types.GenericPayload]{Payload: payload}

	updated, err := p.OnEventTyped(evt, entity)
	if err != nil {
		t.Fatalf("OnEventTyped failed: %v", err)
	}

	var state light.State
	if err := json.Unmarshal(updated.Data.Reported, &state); err != nil {
		t.Fatalf("reported state is not light.State json: %v", err)
	}
	if state.Brightness != 55 {
		t.Fatalf("expected brightness 55, got %d", state.Brightness)
	}
}

func TestOnCommandRejectsOutOfRangeBrightness(t *testing.T) {
	client := &stubMQTTClient{}
	p := NewPlugin()
	p.client = client
	p.discovered = map[string]discoveredEntity{
		"u1": {
			UniqueID:           "u1",
			CommandTopic:       "zigbee2mqtt/lamp/set",
			SupportsBrightness: true,
		},
	}
	v := 101
	req := types.CommandRequest[types.GenericPayload]{
		CommandID: "cmd-1",
		Payload: types.GenericPayload{
			"type":       light.ActionSetBrightness,
			"brightness": v,
		},
	}
	entity := types.Entity{
		ID:       z2mEntityID("u1"),
		DeviceID: "z2m-device-1",
		Domain:   light.Type,
	}

	_, err := p.OnCommandTyped(req, entity)
	if err == nil {
		t.Fatal("expected error for brightness > 100")
	}
	if !strings.Contains(err.Error(), "brightness") {
		t.Fatalf("expected brightness validation error, got: %v", err)
	}
	if client.publishedTopic != "" || client.publishedPayload != "" {
		t.Fatal("command should not be published when brightness is out of range")
	}
}

func TestOnCommandRejectsUnsupportedLightAction(t *testing.T) {
	client := &stubMQTTClient{}
	p := NewPlugin()
	p.client = client
	p.discovered = map[string]discoveredEntity{
		"u1": {
			UniqueID:           "u1",
			CommandTopic:       "zigbee2mqtt/lamp/set",
			SupportsBrightness: false,
		},
	}
	v := 55
	req := types.CommandRequest[types.GenericPayload]{
		CommandID: "cmd-1",
		Payload: types.GenericPayload{
			"type":       light.ActionSetBrightness,
			"brightness": v,
		},
	}
	entity := types.Entity{
		ID:       z2mEntityID("u1"),
		DeviceID: "z2m-device-1",
		Domain:   light.Type,
	}

	_, err := p.OnCommandTyped(req, entity)
	if err == nil {
		t.Fatal("expected unsupported action error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected not supported error, got: %v", err)
	}
	if client.publishedTopic != "" || client.publishedPayload != "" {
		t.Fatal("unsupported command should not be published")
	}
}
