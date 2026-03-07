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
	evt := types.Event{Payload: json.RawMessage(normalized)}

	updated, err := p.OnEvent(evt, entity)
	if err != nil {
		t.Fatalf("OnEvent failed: %v", err)
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
	payload, _ := json.Marshal(map[string]any{
		"type":       light.ActionSetBrightness,
		"brightness": v,
	})
	req := types.Command{
		ID:      "cmd-1",
		Payload: json.RawMessage(payload),
	}
	entity := types.Entity{
		ID:       z2mEntityID("u1"),
		DeviceID: "z2m-device-1",
		Domain:   light.Type,
	}

	_, err := p.OnCommand(req, entity)
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
	payload, _ := json.Marshal(map[string]any{
		"type":       light.ActionSetBrightness,
		"brightness": v,
	})
	req := types.Command{
		ID:      "cmd-1",
		Payload: json.RawMessage(payload),
	}
	entity := types.Entity{
		ID:       z2mEntityID("u1"),
		DeviceID: "z2m-device-1",
		Domain:   light.Type,
	}

	_, err := p.OnCommand(req, entity)
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

func TestNormalizeBinarySensorPayloadToEventPayload(t *testing.T) {
	got, ok := normalizePayloadForDomain("binary_sensor", []byte(`{"state":true}`))
	if !ok {
		t.Fatal("expected binary_sensor payload to normalize")
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal normalized payload failed: %v", err)
	}
	if out["type"] != "active" {
		t.Fatalf("type=%v want=active", out["type"])
	}
	if out["state"] != true {
		t.Fatalf("state=%v want=true", out["state"])
	}
}

func TestNormalizeSensorPayloadToEventPayload(t *testing.T) {
	got, ok := normalizePayloadForDomain("sensor", []byte(`{"value":23.5}`))
	if !ok {
		t.Fatal("expected sensor payload to normalize")
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal normalized payload failed: %v", err)
	}
	if out["type"] != "value_changed" {
		t.Fatalf("type=%v want=value_changed", out["type"])
	}
	if out["value"] != 23.5 {
		t.Fatalf("value=%v want=23.5", out["value"])
	}
}

func TestNormalizeCoverPayloadToEventPayload(t *testing.T) {
	got, ok := normalizePayloadForDomain("cover", []byte(`{"position":42}`))
	if !ok {
		t.Fatal("expected cover payload to normalize")
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal normalized payload failed: %v", err)
	}
	if out["type"] != "state_changed" {
		t.Fatalf("type=%v want=state_changed", out["type"])
	}
	if out["position"] != float64(42) {
		t.Fatalf("position=%v want=42", out["position"])
	}
}
