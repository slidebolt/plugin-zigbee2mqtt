//go:build bdd

// Lua scripting integration tests for plugin-zigbee2mqtt.
//
// These tests prove that the sb-script engine drives commands to real
// Zigbee light entities through the shared NATS bus — no MQTT broker
// required. Entities are seeded directly into storage and the script
// engine resolves them via QueryService.Find on each timer tick.
//
// Run with:
//
//	go test -tags bdd -v -run TestLuaFade ./cmd/plugin-zigbee2mqtt/...
package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	domain "github.com/slidebolt/sb-domain"
	managersdk "github.com/slidebolt/sb-manager-sdk"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	scriptserver "github.com/slidebolt/sb-script/server"
	storage "github.com/slidebolt/sb-storage-sdk"
)

// scriptResp is the envelope returned by all script.* NATS endpoints.
type scriptResp struct {
	OK    bool   `json:"ok"`
	Hash  string `json:"hash,omitempty"`
	Error string `json:"error,omitempty"`
}

// scriptAPI makes a request/response call to the sb-script service and
// fails the test immediately if the transport fails.
func scriptAPI(t *testing.T, msg messenger.Messenger, subject string, body any) scriptResp {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("scriptAPI marshal %s: %v", subject, err)
	}
	resp, err := msg.Request(subject, data, 5*time.Second)
	if err != nil {
		t.Fatalf("scriptAPI request %s: %v", subject, err)
	}
	var r scriptResp
	if err := json.Unmarshal(resp.Data, &r); err != nil {
		t.Fatalf("scriptAPI parse %s: %v", subject, err)
	}
	return r
}

// seedZigbeeLight writes a light entity into storage using the Z2M
// friendly-name convention (device == entity, matching real Z2M topology).
func seedZigbeeLight(t *testing.T, store storage.Storage, name string) domain.Entity {
	t.Helper()
	e := domain.Entity{
		ID:       name,
		Plugin:   pluginID,
		DeviceID: name,
		Type:     "light",
		Name:     name,
		Commands: []string{"light_turn_on", "light_turn_off", "light_set_brightness"},
		State:    domain.Light{Power: true, Brightness: 0},
	}
	if err := store.Save(e); err != nil {
		t.Fatalf("seedZigbeeLight %s: %v", name, err)
	}
	return e
}

// luaFadeEnv creates a test environment with the messenger, storage, and
// sb-script services started and the fade script definition already saved.
func luaFadeEnv(t *testing.T) (*managersdk.TestEnv, storage.Storage, messenger.Messenger) {
	t.Helper()
	e := managersdk.NewTestEnv(t)
	e.Start("messenger")
	e.Start("storage")

	// Start sb-script directly using MessengerPayload (separate connection, same bus).
	scriptMsg, err := messenger.Connect(map[string]json.RawMessage{
		"messenger": e.MessengerPayload(),
	})
	if err != nil {
		t.Fatalf("sb-script messenger: %v", err)
	}
	svc, err := scriptserver.New(scriptMsg, e.Storage())
	if err != nil {
		t.Fatalf("sb-script start: %v", err)
	}
	// Flush ensures NATS has registered subscriptions before first request.
	if err := scriptMsg.Flush(); err != nil {
		t.Fatalf("sb-script flush: %v", err)
	}
	t.Cleanup(func() { svc.Shutdown(); scriptMsg.Close() })

	src, err := os.ReadFile("lua_fade_test.lua")
	if err != nil {
		t.Fatalf("read lua_fade_test.lua: %v", err)
	}
	r := scriptAPI(t, e.Messenger(), "script.save_definition", map[string]string{
		"name":   "zigbee_fade",
		"source": string(src),
	})
	if !r.OK {
		t.Fatalf("save_definition: %s", r.Error)
	}
	return e, e.Storage(), e.Messenger()
}

// brightnessSubject returns the NATS subject on which light_set_brightness
// commands will arrive for a given Zigbee friendly-name light.
func brightnessSubject(name string) string {
	return pluginID + "." + name + "." + name + ".command.light_set_brightness"
}

// ==========================================================================
// TestLuaFade_ZigbeeLights
//
// Proves that the fade script sends ascending brightness values to each of
// Main_LB_01, Main_LB_02, and Main_LB_03 concurrently.
// ==========================================================================

func TestLuaFade_ZigbeeLights(t *testing.T) {
	e, store, msg := luaFadeEnv(t)
	_ = e

	lights := []string{"Main_LB_01", "Main_LB_02", "Main_LB_03"}
	for _, name := range lights {
		seedZigbeeLight(t, store, name)
	}

	type entry struct {
		name       string
		brightness int
	}
	received := make(chan entry, 300)

	for _, name := range lights {
		name := name
		sub, err := msg.Subscribe(brightnessSubject(name), func(m *messenger.Message) {
			var cmd struct {
				Brightness int `json:"brightness"`
			}
			if err := json.Unmarshal(m.Data, &cmd); err != nil {
				return
			}
			received <- entry{name, cmd.Brightness}
		})
		if err != nil {
			t.Fatalf("subscribe %s: %v", name, err)
		}
		defer sub.Unsubscribe()
	}

	r := scriptAPI(t, msg, "script.start", map[string]string{
		"name":  "zigbee_fade",
		"query": "?type=light",
	})
	if !r.OK {
		t.Fatalf("script.start: %s", r.Error)
	}

	// Collect 4 brightness ticks × 3 lights = 12 readings.
	const wantTicks = 4
	readings := make(map[string][]int, len(lights))

	deadline := time.After(2 * time.Second)
	total := 0
	want := wantTicks * len(lights)
	for total < want {
		select {
		case cmd := <-received:
			readings[cmd.name] = append(readings[cmd.name], cmd.brightness)
			total++
		case <-deadline:
			t.Fatalf("timeout: collected %d/%d brightness readings", total, want)
		}
	}

	// Each light must receive the same ascending sequence: 0, 50, 100, 150.
	wantSeq := []int{0, 50, 100, 150}
	for _, name := range lights {
		got := readings[name]
		if len(got) < wantTicks {
			t.Errorf("%s: got %d readings, want >=%d", name, len(got), wantTicks)
			continue
		}
		for i, want := range wantSeq {
			if got[i] != want {
				t.Errorf("%s tick %d: brightness=%d, want %d", name, i, got[i], want)
			}
		}
	}
}

// ==========================================================================
// TestLuaFade_StopHaltsCommands
//
// Proves that stopping the script via script.stop immediately silences the
// timer — no further brightness commands arrive after the call returns.
// ==========================================================================

func TestLuaFade_StopHaltsCommands(t *testing.T) {
	e, store, msg := luaFadeEnv(t)
	_ = e

	seedZigbeeLight(t, store, "Main_LB_01")

	arrived := make(chan int, 50)
	sub, err := msg.Subscribe(brightnessSubject("Main_LB_01"), func(m *messenger.Message) {
		var cmd struct {
			Brightness int `json:"brightness"`
		}
		if err := json.Unmarshal(m.Data, &cmd); err != nil {
			return
		}
		arrived <- cmd.Brightness
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	r := scriptAPI(t, msg, "script.start", map[string]string{
		"name":  "zigbee_fade",
		"query": "?type=light",
	})
	if !r.OK {
		t.Fatalf("script.start: %s", r.Error)
	}

	// Wait for at least 3 commands to confirm the script is running.
	count := 0
	deadline := time.After(time.Second)
	for count < 3 {
		select {
		case <-arrived:
			count++
		case <-deadline:
			t.Fatalf("only %d commands arrived before stop", count)
		}
	}

	// Stop the script.
	stopResp := scriptAPI(t, msg, "script.stop", map[string]string{
		"name":  "zigbee_fade",
		"query": "?type=light",
	})
	if !stopResp.OK {
		t.Fatalf("script.stop: %s", stopResp.Error)
	}

	// Drain any in-flight messages, then assert silence for 300 ms.
	for len(arrived) > 0 {
		<-arrived
	}
	select {
	case v := <-arrived:
		t.Fatalf("received brightness=%d after StopScript — timer not halted", v)
	case <-time.After(300 * time.Millisecond):
		// silence confirmed
	}
}

// ==========================================================================
// TestLuaFade_NewDevicePickedUp
//
// Proves that QueryService.Find re-resolves on every timer tick, so a light
// added to storage after script.start begins receiving commands by the next
// tick without restarting the script.
// ==========================================================================

func TestLuaFade_NewDevicePickedUp(t *testing.T) {
	e, store, msg := luaFadeEnv(t)
	_ = e

	// Start with only Main_LB_01.
	seedZigbeeLight(t, store, "Main_LB_01")

	r := scriptAPI(t, msg, "script.start", map[string]string{
		"name":  "zigbee_fade",
		"query": "?type=light",
	})
	if !r.OK {
		t.Fatalf("script.start: %s", r.Error)
	}

	// Confirm Main_LB_01 is receiving commands.
	firstArrived := make(chan struct{}, 10)
	sub1, err := msg.Subscribe(brightnessSubject("Main_LB_01"), func(_ *messenger.Message) {
		firstArrived <- struct{}{}
	})
	if err != nil {
		t.Fatalf("subscribe Main_LB_01: %v", err)
	}
	defer sub1.Unsubscribe()

	select {
	case <-firstArrived:
	case <-time.After(time.Second):
		t.Fatal("Main_LB_01 never received a brightness command")
	}

	// Now add Main_LB_02 mid-run.
	seedZigbeeLight(t, store, "Main_LB_02")

	newArrived := make(chan struct{}, 10)
	sub2, err := msg.Subscribe(brightnessSubject("Main_LB_02"), func(_ *messenger.Message) {
		newArrived <- struct{}{}
	})
	if err != nil {
		t.Fatalf("subscribe Main_LB_02: %v", err)
	}
	defer sub2.Unsubscribe()

	// QueryService.Find picks up the new entity on the very next 50 ms tick.
	select {
	case <-newArrived:
		// new device picked up dynamically
	case <-time.After(time.Second):
		t.Fatal("Main_LB_02 was not picked up by the running script within 1s")
	}
}
