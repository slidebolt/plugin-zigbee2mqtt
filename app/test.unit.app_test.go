package app_test

import (
	"testing"

	"github.com/slidebolt/plugin-zigbee2mqtt/app"
)

func TestHelloManifest(t *testing.T) {
	got := app.New().Hello()
	if got.ID != app.PluginID {
		t.Fatalf("Hello().ID = %q, want %q", got.ID, app.PluginID)
	}
	if got.Definitions == nil || len(got.Definitions.EntityTypes) != 10 {
		t.Fatalf("Hello().Definitions = %#v", got.Definitions)
	}
	if err := got.Setup.Validate(); err != nil {
		t.Fatalf("Hello().Setup: %v", err)
	}
	if got.Setup.Mode != "selectable" || len(got.Setup.Fields) != 6 {
		t.Fatalf("Hello().Setup = %#v", got.Setup)
	}
	if got.Setup.Fields[0].Environment != "Z2M_MQTT_BROKER" || got.Setup.Fields[5].Generate != "uuid" {
		t.Fatalf("Hello().Setup.Fields = %#v", got.Setup.Fields)
	}
}
