//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	testkit "github.com/slidebolt/sb-testkit"
	storage "github.com/slidebolt/sb-storage-sdk"
)

// TestDiscovery_PrintAllDevices connects to the real MQTT broker, waits for
// Zigbee2MQTT device discovery, and prints every device/entity ID so we can
// map them to production_groups.csv.
//
// Run: Z2M_MQTT_BROKER=tcp://localhost:1883 go test -tags integration -v -run TestDiscovery_PrintAllDevices ./cmd/plugin-zigbee2mqtt/
func TestDiscovery_PrintAllDevices(t *testing.T) {
	broker := os.Getenv("Z2M_MQTT_BROKER")
	if broker == "" {
		broker = "tcp://localhost:1883"
	}
	t.Logf("MQTT broker: %s", broker)

	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	store := env.Storage()

	// Start the plugin — it will connect to MQTT and discover devices.
	p := &plugin{}
	deps := map[string]json.RawMessage{
		"messenger": env.MessengerPayload(),
	}
	if _, err := p.OnStart(deps); err != nil {
		t.Fatalf("plugin OnStart: %v", err)
	}
	t.Cleanup(func() { p.OnShutdown() })

	if p.mqtt == nil || !p.mqtt.IsConnected() {
		t.Fatal("MQTT not connected — is the broker running?")
	}

	// Wait for discovery messages to arrive from Z2M.
	t.Log("waiting 5s for MQTT discovery messages...")
	time.Sleep(5 * time.Second)

	// Query all entities registered by the plugin.
	entries, err := store.Search("plugin-zigbee2mqtt.*.*")
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no entities discovered — check MQTT broker and Zigbee2MQTT")
	}

	type entityRow struct {
		Key          string
		Type         string
		Name         string
		DeviceID     string
		ID           string
		Commands     []string
		StateTopic   string
		CommandTopic string
		FriendlyName string
	}
	var rows []entityRow

	t.Logf("\n=== Zigbee2MQTT Discovery Results: %d entities ===", len(entries))
	t.Logf("%-100s %-16s %s", "ENTITY_KEY", "TYPE", "NAME")
	t.Log("---")

	for _, e := range entries {
		var ent struct {
			ID       string   `json:"id"`
			Plugin   string   `json:"plugin"`
			DeviceID string   `json:"deviceID"`
			Type     string   `json:"type"`
			Name     string   `json:"name"`
			Commands []string `json:"commands"`
		}
		if json.Unmarshal(e.Data, &ent) != nil {
			continue
		}

		// Try to get internal topic info.
		stateTopic, cmdTopic, friendlyName := "", "", ""
		internalKey := struct{ plugin, deviceID, id string }{ent.Plugin, ent.DeviceID, ent.ID}
		_ = internalKey
		rawInternal, err := store.ReadFile(storage.Internal, entityKeyHelper{ent.Plugin, ent.DeviceID, ent.ID})
		if err == nil {
			var info struct {
				StateTopic   string `json:"state_topic"`
				CommandTopic string `json:"command_topic"`
				FriendlyName string `json:"friendly_name"`
			}
			if json.Unmarshal(rawInternal, &info) == nil {
				stateTopic = info.StateTopic
				cmdTopic = info.CommandTopic
				friendlyName = info.FriendlyName
			}
		}

		rows = append(rows, entityRow{
			Key: e.Key, Type: ent.Type, Name: ent.Name,
			DeviceID: ent.DeviceID, ID: ent.ID,
			Commands: ent.Commands,
			StateTopic: stateTopic, CommandTopic: cmdTopic,
			FriendlyName: friendlyName,
		})
		t.Logf("%-100s %-16s %s", e.Key, ent.Type, ent.Name)
	}

	// Print light entities specifically (the ones we care about for groups).
	t.Log("\n=== Light Entities Only ===")
	t.Logf("%-100s %-30s %s", "ENTITY_KEY", "FRIENDLY_NAME", "MQTT_CMD_TOPIC")
	t.Log("---")
	lightCount := 0
	for _, r := range rows {
		if r.Type == "light" {
			lightCount++
			t.Logf("%-100s %-30s %s", r.Key, r.FriendlyName, r.CommandTopic)
		}
	}
	t.Logf("total lights: %d", lightCount)

	// Print CSV mapping.
	t.Log("\n=== CSV Mapping ===")
	t.Log("entity_key,device_id,entity_id,type,name,friendly_name,state_topic,command_topic")
	for _, r := range rows {
		t.Logf("%s,%s,%s,%s,%s,%s,%s,%s", r.Key, r.DeviceID, r.ID, r.Type, r.Name, r.FriendlyName, r.StateTopic, r.CommandTopic)
	}

	fmt.Fprintf(os.Stderr, "\nZigbee2MQTT discovery complete: %d entities (%d lights)\n", len(rows), lightCount)
}

// entityKeyHelper implements the Key() interface for storage.ReadFile.
type entityKeyHelper struct {
	plugin   string
	deviceID string
	id       string
}

func (e entityKeyHelper) Key() string {
	return e.plugin + "." + e.deviceID + "." + e.id
}
