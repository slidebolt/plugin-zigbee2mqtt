package app

import (
	"encoding/json"
	"reflect"
	"testing"

	domain "github.com/slidebolt/sb-domain"
	testkit "github.com/slidebolt/sb-testkit"
)

func TestStorageContract_SeedDemoPersistsEntities(t *testing.T) {
	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	p := &plugin{store: env.Storage()}
	if err := p.seedDemo(); err != nil {
		t.Fatalf("seedDemo: %v", err)
	}

	raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "demo_device", ID: "demo_light"})
	if err != nil {
		t.Fatalf("get demo light: %v", err)
	}
	var got domain.Entity
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantCommands := []string{"light_turn_on", "light_turn_off", "light_set_brightness", "light_set_color_temp"}
	if !reflect.DeepEqual(got.Commands, wantCommands) {
		t.Fatalf("commands = %v, want %v", got.Commands, wantCommands)
	}
	light, ok := got.State.(domain.Light)
	if !ok {
		t.Fatalf("state type = %T", got.State)
	}
	if light.Brightness != 128 || light.Power {
		t.Fatalf("state = %+v", light)
	}
}
