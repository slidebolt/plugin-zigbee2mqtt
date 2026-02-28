package main

import (
	"testing"

	"github.com/slidebolt/sdk-types"
)

func TestPluginDiscoveryWall(t *testing.T) {
	plugin := NewPlugin()

	// 1. Simulate SDK loading an existing device from disk (User renamed it)
	deviceID := z2mDeviceID("hw-mac-001")
	existingDevice := types.Device{
		ID:         deviceID,
		SourceID:   "hw-mac-001",
		SourceName: "Original HW Name",
		LocalName:  "Basement Bar 01", // The User's custom name
	}

	// 2. Simulate an MQTT message arriving from the hardware with a new name
	discoveryPayload := []byte(`{
		"unique_id": "ent-123",
		"name": "Light Entity",
		"device": {
			"identifiers": ["hw-mac-001"],
			"name": "Updated Firmware Name"
		},
		"state_topic": "z2m/state",
		"command_topic": "z2m/set"
	}`)

	plugin.handleDiscoveryMessage("homeassistant/light/node1/obj1/config", discoveryPayload)
	t.Logf("Discovered state: %+v", plugin.discovered["ent-123"])

	// 3. The Gateway asks the plugin for the final list
	// We pass the existing device (simulating the SDK's state)
	results, err := plugin.OnDevicesList([]types.Device{existingDevice})
	if err != nil {
		t.Fatalf("OnDevicesList failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 device, got %d", len(results))
	}

	finalDevice := results[0]
	t.Logf("Final Device: %+v", finalDevice)
	
	// VERIFY THE WALL

	// A. Hardware won the SourceName
	if finalDevice.SourceName != "Updated Firmware Name" {
		t.Errorf("Expected SourceName to update to 'Updated Firmware Name', got %q", finalDevice.SourceName)
	}

	// B. User won the LocalName
	if finalDevice.LocalName != "Basement Bar 01" {
		t.Errorf("The Wall failed! User's LocalName was overwritten: got %q", finalDevice.LocalName)
	}

	// C. UUID remained stable
	if finalDevice.ID != deviceID {
		t.Errorf("Expected stable ID, got %q", finalDevice.ID)
	}
}

func TestPluginGhostDevices(t *testing.T) {
	plugin := NewPlugin()
	
	// Simulate an existing device on disk that is NOT in the active discovery map
	// (e.g. it was unplugged)
	existingDevice := types.Device{
		ID:         "z2m-device-offline-001",
		SourceID:   "offline-001",
		SourceName: "Unplugged Lamp",
		LocalName:  "My Lamp",
	}
	
	results, err := plugin.OnDevicesList([]types.Device{existingDevice})
	if err != nil {
		t.Fatalf("OnDevicesList failed: %v", err)
	}
	
	// The device should STILL be in the list, even though it's not in plugin.discovered.
	// This prevents the "dropped off the list" bug.
	if len(results) != 1 {
		t.Fatalf("Expected 1 device (ghost/offline), got %d", len(results))
	}
	
	if results[0].ID != "z2m-device-offline-001" {
		t.Errorf("Lost the offline device")
	}
}
