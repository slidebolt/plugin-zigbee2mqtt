package device

import (
	"os"
	"github.com/slidebolt/plugin-framework"
	"github.com/slidebolt/plugin-sdk"
	"strings"
	"testing"
	"time"
)

type mockMQTTClient struct {
	subs      map[string]func(topic string, payload []byte)
	published []publishedMsg
}

type publishedMsg struct {
	topic   string
	payload string
}

func newMockMQTTClient() *mockMQTTClient {
	return &mockMQTTClient{subs: map[string]func(string, []byte){}}
}

func (m *mockMQTTClient) Connect() error { return nil }

func (m *mockMQTTClient) Subscribe(topic string, callback func(topic string, payload []byte)) error {
	m.subs[topic] = callback
	return nil
}

func (m *mockMQTTClient) Publish(topic string, payload interface{}) error {
	s, _ := payload.(string)
	m.published = append(m.published, publishedMsg{topic: topic, payload: s})
	return nil
}

func (m *mockMQTTClient) Disconnect() {}

func TestHandleDiscovery_WiresExistingEntity(t *testing.T) {
	_ = os.RemoveAll("state")
	_ = os.RemoveAll("logs")
	t.Cleanup(func() {
		_ = os.RemoveAll("state")
		_ = os.RemoveAll("logs")
	})

	b, err := framework.RegisterBundle("plugin-mqtt-adapter-test")
	if err != nil {
		t.Fatalf("register bundle: %v", err)
	}
	dev, _ := b.CreateDevice()
	dev.UpdateMetadata("Test Device", sdk.SourceID("mqtt-test-device"))
	ent, _ := dev.CreateEntity(sdk.TYPE_LIGHT)
	ent.UpdateMetadata("Test Light", sdk.SourceID("test-unique-light"))

	client := newMockMQTTClient()
	adapter := NewMQTTAdapter(b, client)

	discoveryTopic := "homeassistant/light/node/test/config"
	discoveryPayload := []byte(`{
		"name":"Test Light",
		"unique_id":"test-unique-light",
		"device":{"identifiers":["test-device"],"name":"Test Device","model":"M","manufacturer":"X"},
		"state_topic":"zigbee2mqtt/test_light",
		"command_topic":"zigbee2mqtt/test_light/set",
		"payload_on":"ON",
		"payload_off":"OFF"
	}`)
	adapter.HandleDiscovery(discoveryTopic, discoveryPayload)

	time.Sleep(100 * time.Millisecond)
	cb := client.subs["zigbee2mqtt/test_light"]
	if cb == nil {
		t.Fatalf("expected state subscription for existing entity")
	}

	cb("zigbee2mqtt/test_light", []byte(`{"state":"ON","brightness":128}`))
	time.Sleep(100 * time.Millisecond)

	if !ent.State().Enabled || ent.State().Status != "active" {
		t.Fatalf("expected state update from MQTT callback, got enabled=%v status=%s", ent.State().Enabled, ent.State().Status)
	}

	light := ent.(sdk.Light)
	_ = light.TurnOff()
	time.Sleep(100 * time.Millisecond)

	if len(client.published) == 0 {
		t.Fatalf("expected command publish for existing entity")
	}
	last := client.published[len(client.published)-1]
	if last.topic != "zigbee2mqtt/test_light/set" || last.payload != "OFF" {
		t.Fatalf("unexpected command publish: topic=%s payload=%s", last.topic, last.payload)
	}

	_ = light.SetTemperature(3200)
	time.Sleep(100 * time.Millisecond)
	last = client.published[len(client.published)-1]
	if !strings.Contains(last.payload, "\"color_temp\"") {
		t.Fatalf("expected temperature command JSON payload, got=%s", last.payload)
	}
}
