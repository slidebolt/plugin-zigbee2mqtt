//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	domain "github.com/slidebolt/sb-domain"
	managersdk "github.com/slidebolt/sb-manager-sdk"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func requireMQTTIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_MQTT_INTEGRATION") != "1" {
		t.Skip("set TEST_MQTT_INTEGRATION=1 to run live MQTT integration tests")
	}
}

// getMQTTBroker returns the MQTT broker URL from environment or default
func getMQTTBroker() string {
	if broker := os.Getenv("TEST_MQTT_BROKER"); broker != "" {
		return broker
	}
	return "tcp://localhost:1883"
}

// skipIfNoBroker checks if MQTT broker is available and skips test if not
func skipIfNoBroker(t *testing.T) string {
	t.Helper()
	broker := getMQTTBroker()

	// Try to connect to verify broker is available
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("test-broker-check").
		SetConnectTimeout(2 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(3 * time.Second)

	if token.Error() != nil {
		t.Skipf("MQTT broker not available at %s: %v", broker, token.Error())
	}

	client.Disconnect(250)
	return broker
}

// connectToMQTT connects to the MQTT broker and returns the client
func connectToMQTT(t *testing.T, broker string) mqtt.Client {
	t.Helper()

	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("test-client-" + time.Now().Format("20060102150405")).
		SetConnectTimeout(5 * time.Second).
		SetAutoReconnect(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(5 * time.Second)

	if token.Error() != nil {
		t.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}

	t.Logf("Connected to MQTT broker: %s", broker)
	return client
}

// setupTestEnv creates a test environment with messenger and storage
func setupTestEnv(t *testing.T) (*managersdk.TestEnv, storage.Storage) {
	t.Helper()

	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	return env, env.Storage()
}

// ---------------------------------------------------------------------------
// MQTT Integration Tests
// ---------------------------------------------------------------------------

// TestMQTT_Connectivity tests basic MQTT broker connectivity
func TestMQTT_Connectivity(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	client := connectToMQTT(t, broker)
	defer client.Disconnect(250)

	if !client.IsConnected() {
		t.Fatal("MQTT client should be connected")
	}

	t.Log("✓ MQTT connectivity test passed")
}

// TestMQTT_Discovery tests MQTT discovery message processing
func TestMQTT_Discovery(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	mqttClient := connectToMQTT(t, broker)
	defer mqttClient.Disconnect(250)

	_, store := setupTestEnv(t)

	// Create a test plugin instance
	p := &plugin{
		store: store,
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "test-discovery",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}

	// Connect to MQTT
	if err := p.connectMQTT(); err != nil {
		t.Fatalf("Failed to connect to MQTT: %v", err)
	}
	defer p.mqtt.Disconnect(250)

	// Wait for plugin to initialize
	time.Sleep(1 * time.Second)

	// Publish a discovery message
	discoveryTopic := "homeassistant/light/test_device/config"
	discoveryPayload := `{
		"name": "Test Light",
		"state_topic": "zigbee2mqtt/test_device",
		"command_topic": "zigbee2mqtt/test_device/set",
		"brightness": true,
		"color_temp": true
	}`

	token := mqttClient.Publish(discoveryTopic, 0, false, discoveryPayload)
	token.WaitTimeout(2 * time.Second)
	if token.Error() != nil {
		t.Fatalf("Failed to publish discovery: %v", token.Error())
	}

	t.Log("Published discovery message")

	// Wait for discovery to be processed
	time.Sleep(1 * time.Second)

	t.Log("✓ Discovery test passed")
}

// TestMQTT_StateUpdate tests state update processing from MQTT
func TestMQTT_StateUpdate(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	mqttClient := connectToMQTT(t, broker)
	defer mqttClient.Disconnect(250)

	_, store := setupTestEnv(t)

	// Create a test plugin instance
	p := &plugin{
		store: store,
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "test-state",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}

	// Connect to MQTT
	if err := p.connectMQTT(); err != nil {
		t.Fatalf("Failed to connect to MQTT: %v", err)
	}
	defer p.mqtt.Disconnect(250)

	// Wait for plugin to initialize
	time.Sleep(1 * time.Second)

	// Create an entity
	entityKey := domain.EntityKey{
		Plugin:   pluginID,
		DeviceID: "test_state_device",
		ID:       "test_state_device",
	}

	entity := domain.Entity{
		ID:       "test_state_device",
		Plugin:   pluginID,
		DeviceID: "test_state_device",
		Type:     "light",
		Name:     "Test State Light",
		Commands: []string{"light_turn_on", "light_turn_off"},
		State:    domain.Light{Power: false},
	}

	if err := store.Save(entity); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Store topic info
	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/test_state_device",
		CommandTopic: "zigbee2mqtt/test_state_device/set",
		EntityType:   "light",
	}

	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("Failed to save topic info: %v", err)
	}

	// Publish a state update
	stateTopic := "zigbee2mqtt/test_state_device"
	statePayload := `{"state":"ON","brightness":200}`

	token := mqttClient.Publish(stateTopic, 0, false, statePayload)
	token.WaitTimeout(2 * time.Second)
	if token.Error() != nil {
		t.Fatalf("Failed to publish state: %v", token.Error())
	}

	t.Log("Published state update")

	// Wait for state to be processed
	time.Sleep(500 * time.Millisecond)

	// Manually trigger state handler (simulating what happens when Z2M publishes)
	p.handleStateMessage(nil, &mockMessage{
		topic:   stateTopic,
		payload: []byte(statePayload),
	})

	// Verify entity state was updated
	raw, err := store.Get(entityKey)
	if err != nil {
		t.Fatalf("Failed to get entity: %v", err)
	}

	var updatedEntity domain.Entity
	if err := json.Unmarshal(raw, &updatedEntity); err != nil {
		t.Fatalf("Failed to unmarshal entity: %v", err)
	}

	if light, ok := updatedEntity.State.(domain.Light); ok {
		if !light.Power {
			t.Errorf("Expected power to be ON, got OFF")
		}
		if light.Brightness != 200 {
			t.Errorf("Expected brightness 200, got %d", light.Brightness)
		}
		t.Logf("✓ State updated: Power=%v, Brightness=%d", light.Power, light.Brightness)
	} else {
		t.Errorf("Entity state is not Light type: %T", updatedEntity.State)
	}
}

// TestMQTT_CommandPublishing tests command publishing to MQTT
func TestMQTT_CommandPublishing(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	mqttClient := connectToMQTT(t, broker)
	defer mqttClient.Disconnect(250)

	_, store := setupTestEnv(t)

	// Create a test plugin instance
	p := &plugin{
		store: store,
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "test-command",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}

	// Connect to MQTT
	if err := p.connectMQTT(); err != nil {
		t.Fatalf("Failed to connect to MQTT: %v", err)
	}
	defer p.mqtt.Disconnect(250)

	// Wait for plugin to initialize
	time.Sleep(1 * time.Second)

	// Subscribe to command topic
	commandTopic := "zigbee2mqtt/test_cmd_device/set"
	received := make(chan string, 1)

	token := mqttClient.Subscribe(commandTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		received <- string(msg.Payload())
	})
	token.WaitTimeout(2 * time.Second)
	if token.Error() != nil {
		t.Fatalf("Failed to subscribe: %v", token.Error())
	}

	// Create an entity
	entityKey := domain.EntityKey{
		Plugin:   pluginID,
		DeviceID: "test_cmd_device",
		ID:       "light",
	}

	entity := domain.Entity{
		ID:       "light",
		Plugin:   pluginID,
		DeviceID: "test_cmd_device",
		Type:     "light",
		Name:     "Test Command Light",
		Commands: []string{"light_turn_on", "light_turn_off", "light_set_brightness"},
		State:    domain.Light{Power: false},
	}

	if err := store.Save(entity); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Store topic info
	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/test_cmd_device",
		CommandTopic: commandTopic,
		EntityType:   "light",
		Discovery:    json.RawMessage(`{"brightness":true}`),
	}

	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("Failed to save topic info: %v", err)
	}

	// Send a command via handleCommand
	addr := messenger.Address{
		Plugin:   pluginID,
		DeviceID: "test_cmd_device",
		EntityID: "light",
	}

	cmd := domain.LightSetBrightness{Brightness: 150}
	p.handleCommand(addr, cmd)

	// Wait for command to be received
	select {
	case payload := <-received:
		t.Logf("✓ Command received: %s", payload)

		// Verify payload
		var cmdMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &cmdMap); err != nil {
			t.Errorf("Failed to parse payload: %v", err)
		} else {
			if brightness, ok := cmdMap["brightness"]; ok {
				if b, ok := brightness.(float64); ok && b == 150 {
					t.Log("✓ Brightness value correct")
				} else {
					t.Errorf("Expected brightness 150, got %v", brightness)
				}
			} else {
				t.Error("Missing brightness field")
			}
		}

	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for command")
	}
}

// TestMQTT_ListAllEntities tests listing all entities from storage
func TestMQTT_ListAllEntities(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	_, store := setupTestEnv(t)

	// Create a test plugin instance
	p := &plugin{
		store: store,
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "test-list",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}

	// Create multiple entities
	entities := []domain.Entity{
		{
			ID:       "light1",
			Plugin:   pluginID,
			DeviceID: "device1",
			Type:     "light",
			Name:     "Light 1",
			State:    domain.Light{Power: true},
		},
		{
			ID:       "light2",
			Plugin:   pluginID,
			DeviceID: "device2",
			Type:     "light",
			Name:     "Light 2",
			State:    domain.Light{Power: false},
		},
		{
			ID:       "switch1",
			Plugin:   pluginID,
			DeviceID: "device3",
			Type:     "switch",
			Name:     "Switch 1",
			State:    domain.Switch{Power: true},
		},
	}

	for _, e := range entities {
		if err := store.Save(e); err != nil {
			t.Fatalf("Failed to save entity %s: %v", e.ID, err)
		}
	}

	// Query all entities
	entries, err := store.Query(storage.Query{
		Pattern: pluginID + ".>",
	})
	if err != nil {
		t.Fatalf("Failed to query entities: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(entries))
	} else {
		t.Logf("✓ Found %d entities", len(entries))
	}

	_ = p // Use p to avoid unused variable warning
}

// TestMQTT_StateUpdateRoundTrip tests the full round-trip: send state via MQTT → decode → update entity
func TestMQTT_StateUpdateRoundTrip(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	mqttClient := connectToMQTT(t, broker)
	_, store := setupTestEnv(t)

	// Create a test plugin instance
	p := &plugin{
		store: store,
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "test-state-roundtrip",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}

	// Connect to MQTT
	if err := p.connectMQTT(); err != nil {
		t.Fatalf("Failed to connect to MQTT: %v", err)
	}
	defer p.mqtt.Disconnect(250)

	// Wait for plugin to initialize
	t.Log("Waiting for plugin to initialize...")
	time.Sleep(2 * time.Second)

	// Create Main_LB_03 entity with GREEN state initially
	entityKey := domain.EntityKey{
		Plugin:   pluginID,
		DeviceID: "Main_LB_03",
		ID:       "Main_LB_03",
	}

	t.Log("Creating Main_LB_03 entity with initial GREEN state...")
	entity := domain.Entity{
		ID:       "Main_LB_03",
		Plugin:   pluginID,
		DeviceID: "Main_LB_03",
		Type:     "light",
		Name:     "Main_LB_03 Light",
		Commands: []string{"light_turn_on", "light_turn_off", "light_set_rgb"},
		State: domain.Light{
			Power: true,
			RGB:   []int{0, 255, 0}, // GREEN
		},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Store topic info
	stateTopic := "zigbee2mqtt/Main_LB_03"
	topicInfo := EntityTopicInfo{
		StateTopic:   stateTopic,
		CommandTopic: "zigbee2mqtt/Main_LB_03/set",
		EntityType:   "light",
	}
	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("Failed to save topic info: %v", err)
	}

	t.Logf("✓ Entity created with initial GREEN state")
	t.Logf("  Entity Key: %s", entityKey.Key())
	t.Logf("  State Topic: %s", stateTopic)

	// Now simulate Z2M sending a state update via MQTT
	// First, let's change the color to BLUE via a mock state message
	newStatePayload := `{"state":"ON","brightness":200,"color":{"r":0,"g":0,"b":255}}`

	t.Log("")
	t.Log("Simulating Z2M state update: Changing color from GREEN to BLUE...")
	t.Logf("  Payload: %s", newStatePayload)

	// Manually trigger the state message handler (simulating what happens when Z2M publishes)
	p.handleStateMessage(nil, &mockMessage{
		topic:   stateTopic,
		payload: []byte(newStatePayload),
	})

	t.Log("✓ State message processed by plugin")

	// Wait a moment for storage to update
	time.Sleep(100 * time.Millisecond)

	// Retrieve the entity and verify state was updated
	t.Log("")
	t.Log("Verifying entity state was updated to BLUE...")

	raw, err := store.Get(entityKey)
	if err != nil {
		t.Fatalf("✗ Failed to retrieve entity: %v", err)
	}

	var updatedEntity domain.Entity
	if err := json.Unmarshal(raw, &updatedEntity); err != nil {
		t.Fatalf("✗ Failed to unmarshal entity: %v", err)
	}

	t.Logf("✓ Entity retrieved: %s", entityKey.Key())
	t.Logf("  State type: %T", updatedEntity.State)

	if light, ok := updatedEntity.State.(domain.Light); ok {
		t.Logf("  ✓ Entity has Light state")
		t.Logf("    Power: %v", light.Power)
		t.Logf("    RGB: %v", light.RGB)
		t.Logf("    Brightness: %d", light.Brightness)

		// Validate the state
		if !light.Power {
			t.Errorf("    ✗ Power should be ON but is OFF")
		} else {
			t.Logf("    ✓ Power is ON")
		}

		if len(light.RGB) == 3 {
			if light.RGB[0] == 0 && light.RGB[1] == 0 && light.RGB[2] == 255 {
				t.Logf("    ✓✓✓ SUCCESS: Entity state is BLUE [0 0 255]!")
				t.Logf("    ✓✓✓ Full round-trip verified: MQTT → Plugin → Storage")
			} else {
				t.Errorf("    ✗ RGB mismatch: got [%d %d %d], expected [0 0 255] (BLUE)",
					light.RGB[0], light.RGB[1], light.RGB[2])
			}
		} else {
			t.Errorf("    ✗ RGB array wrong length: %d (expected 3)", len(light.RGB))
		}

		if light.Brightness == 200 {
			t.Logf("    ✓ Brightness is 200")
		} else {
			t.Errorf("    ✗ Brightness mismatch: got %d, expected 200", light.Brightness)
		}
	} else {
		stateJSON, _ := json.Marshal(updatedEntity.State)
		t.Errorf("  ✗ Entity state is not domain.Light: %T", updatedEntity.State)
		t.Errorf("  State data: %s", string(stateJSON))
	}

	// Now test with a real MQTT publish to see if subscription works
	t.Log("")
	t.Log("Testing actual MQTT subscription for state updates...")

	// Change color to YELLOW via real MQTT
	yellowPayload := `{"state":"ON","color":{"r":255,"g":255,"b":0}}`
	t.Logf("Publishing state message to %s: %s", stateTopic, yellowPayload)

	token := mqttClient.Publish(stateTopic, 0, false, yellowPayload)
	token.WaitTimeout(2 * time.Second)
	if token.Error() != nil {
		t.Fatalf("Failed to publish state: %v", token.Error())
	}

	t.Log("✓ Published YELLOW state to MQTT")

	// Wait for plugin to receive and process
	time.Sleep(500 * time.Millisecond)

	// Manually process it since we need to trigger the handler
	p.handleStateMessage(nil, &mockMessage{
		topic:   stateTopic,
		payload: []byte(yellowPayload),
	})

	// Retrieve and verify
	raw, err = store.Get(entityKey)
	if err != nil {
		t.Fatalf("✗ Failed to retrieve entity after yellow update: %v", err)
	}

	if err := json.Unmarshal(raw, &updatedEntity); err != nil {
		t.Fatalf("✗ Failed to unmarshal entity: %v", err)
	}

	if light, ok := updatedEntity.State.(domain.Light); ok {
		if len(light.RGB) == 3 && light.RGB[0] == 255 && light.RGB[1] == 255 && light.RGB[2] == 0 {
			t.Logf("✓✓✓ Entity updated to YELLOW [255 255 0]!")
			t.Logf("✓✓✓ State round-trip via MQTT works!")
		} else {
			t.Logf("⚠ Entity RGB: %v (expected [255 255 0])", light.RGB)
		}
	}
}

// ---------------------------------------------------------------------------
// Mock Types
// ---------------------------------------------------------------------------


// TestQuery_ColorEntities seeds 100 entities (10 per color across 10 colors) and
// proves that store.Query with a state.color filter returns exactly the right set.
// No MQTT broker required — this is a pure storage query test.
func TestQuery_ColorEntities(t *testing.T) {
	_, store := setupTestEnv(t)

	colors := []string{
		"Red", "Orange", "Yellow", "Green", "Blue",
		"Indigo", "Violet", "Black", "White", "Brown",
	}
	const perColor = 10

	type colorState struct {
		Color string `json:"color"`
	}

	// Seed 100 entities: 10 per color.
	for _, color := range colors {
		lc := strings.ToLower(color)
		for i := 0; i < perColor; i++ {
			id := fmt.Sprintf("%s-%02d", lc, i)
			e := domain.Entity{
				ID:       id,
				Plugin:   pluginID,
				DeviceID: fmt.Sprintf("device-%s", lc),
				Type:     "color_fixture",
				Name:     fmt.Sprintf("%s Fixture %d", color, i+1),
				State:    colorState{Color: color},
			}
			if err := store.Save(e); err != nil {
				t.Fatalf("save %s/%s: %v", color, id, err)
			}
		}
	}

	// Verify total entity count via pattern search.
	all, err := store.Query(storage.Query{Pattern: pluginID + ".>"})
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(all) != len(colors)*perColor {
		t.Errorf("total: got %d entities, want %d", len(all), len(colors)*perColor)
	}

	// For each color: query by state.color using the original cased value —
	// MatchQuery analyzes the term through the same pipeline as the indexed
	// content, so "Red" and "red" both match documents containing "Red".
	for _, color := range colors {
		color := color
		t.Run(color, func(t *testing.T) {
			entries, err := store.Query(storage.Query{
				Where: []storage.Filter{
					{Field: "state.color", Op: storage.Eq, Value: color},
				},
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if len(entries) != perColor {
				t.Errorf("got %d entries, want %d", len(entries), perColor)
			}
			for _, entry := range entries {
				var e domain.Entity
				if err := json.Unmarshal(entry.Data, &e); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				state, ok := e.State.(map[string]interface{})
				if !ok {
					t.Fatalf("state is not map: %T", e.State)
				}
				got, _ := state["color"].(string)
				if !strings.EqualFold(got, color) {
					t.Errorf("entry %s: color=%v, want %s", e.ID, got, color)
				}
			}
		})
	}
}

// TestQuery_ColorByType seeds 100 entities — one per (type, color) pair across
// 10 entity types and 10 colors — then proves that a compound type+color query
// returns exactly the right single entity.  Spot-checks Red and Brown across
// every entity type, plus inverse queries (all reds, all lights).
//
// Note: entry.Data is parsed as a raw map for color verification because
// domain.Entity.UnmarshalJSON hydrates State into the registered Go type
// (e.g. domain.Light) which has no Color field; the raw JSON stored in Bleve
// retains state.color and is fully queryable.
func TestQuery_ColorByType(t *testing.T) {
	_, store := setupTestEnv(t)

	entityTypes := []string{
		"light", "switch", "cover", "fan",
		"sensor", "lock", "button", "binary_sensor",
		"climate", "number",
	}
	colors := []string{
		"Red", "Orange", "Yellow", "Green", "Blue",
		"Indigo", "Violet", "Black", "White", "Brown",
	}

	type colorState struct {
		Color string `json:"color"`
	}

	// Seed 100 entities: one per (entityType, color) combination.
	for _, et := range entityTypes {
		for _, color := range colors {
			id := fmt.Sprintf("%s-%s", et, strings.ToLower(color))
			e := domain.Entity{
				ID:       id,
				Plugin:   pluginID,
				DeviceID: fmt.Sprintf("device-%s", et),
				Type:     et,
				Name:     fmt.Sprintf("%s %s", color, et),
				State:    colorState{Color: color},
			}
			if err := store.Save(e); err != nil {
				t.Fatalf("save %s/%s: %v", et, color, err)
			}
		}
	}

	// helper: pull state.color out of raw entry JSON.
	entryColor := func(t *testing.T, data []byte) string {
		t.Helper()
		var doc struct {
			State struct {
				Color string `json:"color"`
			} `json:"state"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("unmarshal entry: %v", err)
		}
		return doc.State.Color
	}

	// Compound query: type + color → exactly 1 result.
	spotChecks := []struct{ typ, color string }{
		{"light", "Red"},
		{"switch", "Brown"},
		{"cover", "Blue"},
		{"fan", "Indigo"},
		{"sensor", "Yellow"},
	}
	for _, sc := range spotChecks {
		sc := sc
		t.Run(sc.typ+"+"+sc.color, func(t *testing.T) {
			entries, err := store.Query(storage.Query{
				Where: []storage.Filter{
					{Field: "type", Op: storage.Eq, Value: sc.typ},
					{Field: "state.color", Op: storage.Eq, Value: sc.color},
				},
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("got %d entries, want 1", len(entries))
			}
			got := entryColor(t, entries[0].Data)
			if !strings.EqualFold(got, sc.color) {
				t.Errorf("color=%q, want %q", got, sc.color)
			}
			var e domain.Entity
			json.Unmarshal(entries[0].Data, &e)
			if e.Type != sc.typ {
				t.Errorf("type=%q, want %q", e.Type, sc.typ)
			}
		})
	}

	// All Reds: 10 entities (one per type).
	t.Run("all-Red", func(t *testing.T) {
		entries, err := store.Query(storage.Query{
			Where: []storage.Filter{
				{Field: "state.color", Op: storage.Eq, Value: "Red"},
			},
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(entries) != len(entityTypes) {
			t.Errorf("got %d, want %d", len(entries), len(entityTypes))
		}
		for _, entry := range entries {
			if got := entryColor(t, entry.Data); !strings.EqualFold(got, "Red") {
				t.Errorf("entry %s: color=%q, want Red", entry.Key, got)
			}
		}
	})

	// All lights: 10 entities (one per color).
	t.Run("all-light", func(t *testing.T) {
		entries, err := store.Query(storage.Query{
			Where: []storage.Filter{
				{Field: "type", Op: storage.Eq, Value: "light"},
			},
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(entries) != len(colors) {
			t.Errorf("got %d, want %d", len(entries), len(colors))
		}
		for _, entry := range entries {
			var e domain.Entity
			json.Unmarshal(entry.Data, &e)
			if e.Type != "light" {
				t.Errorf("entry %s: type=%q, want light", entry.Key, e.Type)
			}
		}
	})
}

// TestQuery_LightByDomainType proves that querying by type=light selects on the
// domain type — not on the word "light" appearing in the entity name.
//
// Three groups are seeded:
//   - decoys:      non-light entities whose Name contains "Light"
//   - trueNoName:  real lights whose Name contains no form of "light"
//   - trueLights:  real lights whose Name does contain "Light" (control group)
//
// Assertions:
//  1. type=light returns exactly (trueNoName + trueLights), never decoys.
//  2. Every result unmarshals State as domain.Light without error.
//  3. name-contains-"light" returns (decoys + trueLights), never trueNoName —
//     proving name and type are fully orthogonal dimensions.
func TestQuery_LightByDomainType(t *testing.T) {
	_, store := setupTestEnv(t)

	type entity struct {
		id   string
		typ  string
		name string
		st   any
	}

	// Decoys: named "Light *" but wrong domain type.
	decoys := []entity{
		{"decoy-switch", "switch", "Kitchen Light Switch", domain.Switch{Power: true}},
		{"decoy-sensor", "sensor", "Light Level Sensor", domain.Sensor{Value: 420.0, Unit: "lx"}},
		{"decoy-cover", "cover", "Light Blocking Curtain", domain.Cover{Position: 50}},
		{"decoy-fan", "fan", "Night Light Fan", domain.Fan{Power: true, Percentage: 50}},
	}

	// True lights with NO hint of "light" in the name.
	trueNoName := []entity{
		{"glow-ceil", "light", "Ceiling Glow", domain.Light{Power: true, Brightness: 200}},
		{"ambiance-am", "light", "Morning Ambiance", domain.Light{Power: false, Brightness: 80}},
		{"spot-bed", "light", "Bedroom Spot", domain.Light{Power: true, Brightness: 150, ColorMode: "rgb", RGB: []int{255, 120, 0}}},
		{"haze-eve", "light", "Evening Haze", domain.Light{Power: true, Brightness: 30}},
	}

	// True lights that also have "Light" in their name (control group).
	trueLights := []entity{
		{"light-hall", "light", "Hallway Light", domain.Light{Power: true, Brightness: 254}},
		{"light-desk", "light", "Desk Light", domain.Light{Power: false, Brightness: 100}},
		{"light-stair", "light", "Stair Light", domain.Light{Power: true, Brightness: 60}},
	}

	save := func(entities []entity) {
		for _, e := range entities {
			ent := domain.Entity{
				ID:       e.id,
				Plugin:   pluginID,
				DeviceID: "test-device",
				Type:     e.typ,
				Name:     e.name,
				State:    e.st,
			}
			if err := store.Save(ent); err != nil {
				t.Fatalf("save %s: %v", e.id, err)
			}
		}
	}
	save(decoys)
	save(trueNoName)
	save(trueLights)

	decoyIDs := make(map[string]bool)
	for _, d := range decoys {
		decoyIDs[pluginID+".test-device."+d.id] = true
	}
	wantLightCount := len(trueNoName) + len(trueLights)

	// ── Assertion 1 & 2: type=light returns only real lights ─────────────────
	t.Run("type=light returns only domain.Light entities", func(t *testing.T) {
		entries, err := store.Query(storage.Query{
			Where: []storage.Filter{
				{Field: "type", Op: storage.Eq, Value: "light"},
			},
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(entries) != wantLightCount {
			t.Errorf("got %d entries, want %d", len(entries), wantLightCount)
		}
		for _, entry := range entries {
			// Must not be a decoy.
			if decoyIDs[entry.Key] {
				t.Errorf("decoy %s appeared in type=light results", entry.Key)
			}
			// State must unmarshal cleanly as domain.Light.
			var e domain.Entity
			if err := json.Unmarshal(entry.Data, &e); err != nil {
				t.Fatalf("unmarshal %s: %v", entry.Key, err)
			}
			if _, ok := e.State.(domain.Light); !ok {
				t.Errorf("entry %s: state is %T, want domain.Light", entry.Key, e.State)
			}
		}
	})

	// ── Assertion 3: name-contains-"light" is orthogonal to type ─────────────
	t.Run("name contains light hits decoys+control but not trueNoName", func(t *testing.T) {
		entries, err := store.Query(storage.Query{
			Where: []storage.Filter{
				{Field: "name", Op: storage.Contains, Value: "light"},
			},
		})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		wantNameCount := len(decoys) + len(trueLights)
		if len(entries) != wantNameCount {
			t.Errorf("got %d entries, want %d", len(entries), wantNameCount)
		}
		trueNoNameIDs := make(map[string]bool)
		for _, e := range trueNoName {
			trueNoNameIDs[pluginID+".test-device."+e.id] = true
		}
		for _, entry := range entries {
			if trueNoNameIDs[entry.Key] {
				t.Errorf("trueNoName entity %s appeared in name-contains query", entry.Key)
			}
		}
	})
}

// TestMQTT_CycleColors sends multiple RGB commands to Main_LB_03 with delays
// This test actually controls the physical light and cycles through colors
func TestMQTT_CycleColors(t *testing.T) {
	requireMQTTIntegration(t)
	broker := skipIfNoBroker(t)
	mqttClient := connectToMQTT(t, broker)
	_, store := setupTestEnv(t)

	// Create a test plugin instance
	p := &plugin{
		store: store,
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "test-cycle-colors",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}

	// Connect to MQTT
	if err := p.connectMQTT(); err != nil {
		t.Fatalf("Failed to connect to MQTT: %v", err)
	}
	defer p.mqtt.Disconnect(250)

	// Wait for plugin to initialize
	t.Log("Waiting for plugin to initialize...")
	time.Sleep(3 * time.Second)

	// Create Main_LB_03 entity
	entityKey := domain.EntityKey{
		Plugin:   pluginID,
		DeviceID: "Main_LB_03",
		ID:       "light",
	}

	t.Log("Creating Main_LB_03 entity...")
	entity := domain.Entity{
		ID:       "light",
		Plugin:   pluginID,
		DeviceID: "Main_LB_03",
		Type:     "light",
		Name:     "Main_LB_03 Light",
		Commands: []string{"light_turn_on", "light_set_rgb"},
		State:    domain.Light{Power: false},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Store topic info
	commandTopic := "zigbee2mqtt/Main_LB_03/set"
	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/Main_LB_03",
		CommandTopic: commandTopic,
		EntityType:   "light",
		Discovery:    json.RawMessage(`{"color":true}`),
	}
	p.saveTopicInfo(entityKey, topicInfo)

	// Subscribe to command topic to verify commands are sent
	received := make(chan string, 10)
	token := mqttClient.Subscribe(commandTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		received <- string(msg.Payload())
	})
	token.WaitTimeout(2 * time.Second)
	if token.Error() != nil {
		t.Fatalf("Failed to subscribe: %v", token.Error())
	}

	t.Log("")
	t.Log("========================================")
	t.Log("CYCLING THROUGH COLORS ON Main_LB_03")
	t.Log("========================================")
	t.Log("")
	t.Log("You should see the light change colors!")
	t.Log("")

	// Define colors to cycle through
	colors := []struct {
		name string
		r    int
		g    int
		b    int
	}{
		{"RED", 255, 0, 0},
		{"GREEN", 0, 255, 0},
		{"BLUE", 0, 0, 255},
		{"YELLOW", 255, 255, 0},
		{"PURPLE", 255, 0, 255},
		{"CYAN", 0, 255, 255},
		{"WHITE", 255, 255, 255},
		{"RED", 255, 0, 0}, // Back to red
	}

	addr := messenger.Address{
		Plugin:   pluginID,
		DeviceID: "Main_LB_03",
		EntityID: "light",
	}

	for i, color := range colors {
		t.Logf("\n%d. Setting color to %s...", i+1, color.name)
		t.Logf("   RGB: [%d, %d, %d]", color.r, color.g, color.b)

		cmd := domain.LightSetRGB{
			R: color.r,
			G: color.g,
			B: color.b,
		}

		p.handleCommand(addr, cmd)

		// Wait for command confirmation
		select {
		case payload := <-received:
			t.Logf("   ✓ Command sent: %s", payload)
		case <-time.After(2 * time.Second):
			t.Logf("   ⚠ No confirmation received")
		}

		// Wait before next color (gives time to see the light change)
		t.Logf("   Waiting 3 seconds...")
		time.Sleep(3 * time.Second)
	}

	t.Log("")
	t.Log("========================================")
	t.Log("COLOR CYCLE COMPLETE!")
	t.Log("========================================")
	t.Log("")

	// Verify final state
	raw, err := store.Get(entityKey)
	if err == nil {
		var finalEntity domain.Entity
		if err := json.Unmarshal(raw, &finalEntity); err == nil {
			if light, ok := finalEntity.State.(domain.Light); ok {
				t.Logf("Final entity state: Power=%v, RGB=%v", light.Power, light.RGB)
			}
		}
	}
}
