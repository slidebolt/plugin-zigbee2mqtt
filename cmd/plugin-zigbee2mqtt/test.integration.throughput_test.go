//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	domain "github.com/slidebolt/sb-domain"
	managersdk "github.com/slidebolt/sb-manager-sdk"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storageserver "github.com/slidebolt/sb-storage-server"
	storage "github.com/slidebolt/sb-storage-sdk"
)

// TestThroughput_CommandPipeline sends 10,000 commands through the full
// handleCommand pipeline (storage lookup → encode → MQTT publish) and reports
// throughput, total latency, and per-command p50/p95/p99.
//
// Run:
//
//	go test -v -tags integration -run TestThroughput_CommandPipeline ./cmd/plugin-zigbee2mqtt/
func TestThroughput_CommandPipeline(t *testing.T) {
	const (
		total       = 1_000
		deviceID    = "throughput_device"
		entityID    = "light"
		commandTopic = "zigbee2mqtt/" + deviceID + "/set"
	)

	// --- setup env ---
	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")
	store := env.Storage()

	// --- setup plugin (no real MQTT — measures storage + encode overhead) ---
	p := &plugin{
		store:           store,
		stateTopicIndex: make(map[string][]domain.EntityKey),
		mqttCfg: MQTTConfig{
			Broker:          "",
			ClientID:        "throughput-test",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}
	// Leave p.mqtt nil so MQTT publish is skipped — isolates storage cost.

	// --- seed entity + topic info ---
	entity := domain.Entity{
		ID:       entityID,
		Plugin:   pluginID,
		DeviceID: deviceID,
		Type:     "light",
		Name:     "Throughput Test Light",
		Commands: []string{"light_set_rgb"},
		State:    domain.Light{Power: true, Brightness: 128},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("save entity: %v", err)
	}

	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/" + deviceID,
		CommandTopic: commandTopic,
		EntityType:   "light",
		Discovery:    json.RawMessage(`{}`),
	}
	entityKey := domain.EntityKey{Plugin: pluginID, DeviceID: deviceID, ID: entityID}
	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("save topic info: %v", err)
	}

	addr := messenger.Address{
		Plugin:   pluginID,
		DeviceID: deviceID,
		EntityID: entityID,
	}

	// --- run throughput test ---
	latencies := make([]time.Duration, total)
	start := time.Now()

	for i := 0; i < total; i++ {
		cmd := domain.LightSetRGB{R: i % 256, G: 0, B: 0}
		t0 := time.Now()
		p.handleCommand(addr, cmd)
		latencies[i] = time.Since(t0)
	}

	elapsed := time.Since(start)
	tps := float64(total) / elapsed.Seconds()

	// --- compute percentiles ---
	sortedLatencies := make([]time.Duration, total)
	copy(sortedLatencies, latencies)
	sortDurations(sortedLatencies)

	p50 := sortedLatencies[total*50/100]
	p95 := sortedLatencies[total*95/100]
	p99 := sortedLatencies[total*99/100]
	pMax := sortedLatencies[total-1]

	t.Logf("=== Throughput Results (MQTT disabled — storage only) ===")
	t.Logf("  Total commands : %d", total)
	t.Logf("  Total time     : %s", elapsed.Round(time.Millisecond))
	t.Logf("  Throughput     : %.1f cmd/sec", tps)
	t.Logf("  Latency p50    : %s", p50.Round(time.Microsecond))
	t.Logf("  Latency p95    : %s", p95.Round(time.Microsecond))
	t.Logf("  Latency p99    : %s", p99.Round(time.Microsecond))
	t.Logf("  Latency max    : %s", pMax.Round(time.Microsecond))

	// Fail if throughput is suspiciously low — helps catch regressions
	if tps < 10 {
		t.Errorf("throughput too low: %.1f cmd/sec (expected >10)", tps)
	}
	fmt.Printf("\n[THROUGHPUT] %.1f cmd/sec  p50=%s  p95=%s  p99=%s  total=%s\n",
		tps, p50.Round(time.Microsecond), p95.Round(time.Microsecond),
		p99.Round(time.Microsecond), elapsed.Round(time.Millisecond))
}

// TestThroughput_WithMQTT is the same test but with a real MQTT broker,
// measuring the full pipeline including MQTT publish overhead.
//
// Run:
//
//	go test -v -tags integration -run TestThroughput_WithMQTT ./cmd/plugin-zigbee2mqtt/
func TestThroughput_WithMQTT(t *testing.T) {
	const (
		total       = 1_000
		deviceID    = "throughput_mqtt_device"
		entityID    = "light"
		commandTopic = "zigbee2mqtt/" + deviceID + "/set"
	)

	broker := skipIfNoBroker(t)

	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")
	store := env.Storage()

	p := &plugin{
		store:           store,
		stateTopicIndex: make(map[string][]domain.EntityKey),
		mqttCfg: MQTTConfig{
			Broker:          broker,
			ClientID:        "throughput-mqtt-test",
			DiscoveryPrefix: "homeassistant",
			BaseTopic:       "zigbee2mqtt",
		},
	}
	if err := p.connectMQTT(); err != nil {
		t.Fatalf("connect MQTT: %v", err)
	}
	defer p.mqtt.Disconnect(250)

	entity := domain.Entity{
		ID: entityID, Plugin: pluginID, DeviceID: deviceID,
		Type: "light", Name: "Throughput MQTT Light",
		Commands: []string{"light_set_rgb"},
		State:    domain.Light{Power: true},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("save entity: %v", err)
	}

	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/" + deviceID,
		CommandTopic: commandTopic,
		EntityType:   "light",
		Discovery:    json.RawMessage(`{}`),
	}
	entityKey := domain.EntityKey{Plugin: pluginID, DeviceID: deviceID, ID: entityID}
	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("save topic info: %v", err)
	}

	addr := messenger.Address{
		Plugin:   pluginID,
		DeviceID: deviceID,
		EntityID: entityID,
	}

	latencies := make([]time.Duration, total)
	start := time.Now()

	for i := 0; i < total; i++ {
		cmd := domain.LightSetRGB{R: i % 256, G: 0, B: 0}
		t0 := time.Now()
		p.handleCommand(addr, cmd)
		latencies[i] = time.Since(t0)
	}

	elapsed := time.Since(start)
	tps := float64(total) / elapsed.Seconds()

	sortedLatencies := make([]time.Duration, total)
	copy(sortedLatencies, latencies)
	sortDurations(sortedLatencies)

	p50 := sortedLatencies[total*50/100]
	p95 := sortedLatencies[total*95/100]
	p99 := sortedLatencies[total*99/100]
	pMax := sortedLatencies[total-1]

	t.Logf("=== Throughput Results (with MQTT broker at %s) ===", broker)
	t.Logf("  Total commands : %d", total)
	t.Logf("  Total time     : %s", elapsed.Round(time.Millisecond))
	t.Logf("  Throughput     : %.1f cmd/sec", tps)
	t.Logf("  Latency p50    : %s", p50.Round(time.Microsecond))
	t.Logf("  Latency p95    : %s", p95.Round(time.Microsecond))
	t.Logf("  Latency p99    : %s", p99.Round(time.Microsecond))
	t.Logf("  Latency max    : %s", pMax.Round(time.Microsecond))

	fmt.Printf("\n[THROUGHPUT+MQTT] %.1f cmd/sec  p50=%s  p95=%s  p99=%s  total=%s\n",
		tps, p50.Round(time.Microsecond), p95.Round(time.Microsecond),
		p99.Round(time.Microsecond), elapsed.Round(time.Millisecond))
}

// TestThroughput_ViaNATS proves whether the NATS subscriber is the bottleneck
// by sending 1,000 commands through the full NATS message bus (publish →
// subscriber goroutine → handleCommand) and comparing to the direct-call baseline.
//
// Run:
//
//	go test -v -tags integration -run TestThroughput_ViaNATS ./cmd/plugin-zigbee2mqtt/
func TestThroughput_ViaNATS(t *testing.T) {
	const (
		total    = 1_000
		deviceID = "nats_throughput_device"
		entityID = "light"
	)

	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")
	store := env.Storage()
	msg := env.Messenger()

	// --- seed entity + topic info ---
	p := &plugin{store: store, stateTopicIndex: make(map[string][]domain.EntityKey)}
	entity := domain.Entity{
		ID: entityID, Plugin: pluginID, DeviceID: deviceID,
		Type: "light", Name: "NATS Throughput Light",
		Commands: []string{"light_set_rgb"},
		State:    domain.Light{Power: true},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("save entity: %v", err)
	}
	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/" + deviceID,
		CommandTopic: "zigbee2mqtt/" + deviceID + "/set",
		EntityType:   "light",
		Discovery:    json.RawMessage(`{}`),
	}
	entityKey := domain.EntityKey{Plugin: pluginID, DeviceID: deviceID, ID: entityID}
	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("save topic info: %v", err)
	}

	// --- wire up NATS subscriber (same as production) ---
	done := make(chan struct{})
	received := 0
	cmds := messenger.NewCommands(msg, domain.LookupCommand)
	_, err := cmds.Receive(pluginID+".>", func(addr messenger.Address, cmd any) {
		p.handleCommand(addr, cmd)
		received++
		if received == total {
			close(done)
		}
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// --- send all commands as fast as possible ---
	sender := messenger.NewCommands(msg, domain.LookupCommand)
	start := time.Now()
	for i := 0; i < total; i++ {
		if err := sender.Send(entity, domain.LightSetRGB{R: i % 256, G: 0, B: 0}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	publishDone := time.Since(start)

	// --- wait for all to be consumed ---
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatalf("timeout: only %d/%d commands processed", received, total)
	}
	totalElapsed := time.Since(start)

	publishTPS := float64(total) / publishDone.Seconds()
	consumeTPS := float64(total) / totalElapsed.Seconds()
	queuedFor := totalElapsed - publishDone

	t.Logf("=== NATS-Routed Throughput (storage only, no MQTT) ===")
	t.Logf("  Total commands   : %d", total)
	t.Logf("  Publish time     : %s (%.0f msg/sec)", publishDone.Round(time.Millisecond), publishTPS)
	t.Logf("  Consume time     : %s (%.0f msg/sec)", totalElapsed.Round(time.Millisecond), consumeTPS)
	t.Logf("  Queue drain time : %s", queuedFor.Round(time.Millisecond))
	if queuedFor > 100*time.Millisecond {
		t.Logf("  *** NATS subscriber is a bottleneck — commands queued for %s after publish finished ***", queuedFor.Round(time.Millisecond))
	} else {
		t.Logf("  NATS subscriber kept up (no significant queue buildup)")
	}
	fmt.Printf("\n[NATS] publish=%.0f/s consume=%.0f/s drain=%s total=%s\n",
		publishTPS, consumeTPS, queuedFor.Round(time.Millisecond), totalElapsed.Round(time.Millisecond))
}

// sortDurations sorts a slice of time.Duration in ascending order.
func sortDurations(d []time.Duration) {
	n := len(d)
	for i := 1; i < n; i++ {
		key := d[i]
		j := i - 1
		for j >= 0 && d[j] > key {
			d[j+1] = d[j]
			j--
		}
		d[j+1] = key
	}
}

// TestThroughput_ScriptBacklog replicates the exact production issue:
// a script fires 6 commands every 100ms (like the Flicker/RedBalloon scripts),
// runs for 3 seconds, then stops. The test measures how long commands keep
// draining after the script stops — proving the "lights keep going" bug.
//
// Run:
//
//	go test -v -tags integration -run TestThroughput_ScriptBacklog ./cmd/plugin-zigbee2mqtt/
func TestThroughput_ScriptBacklog(t *testing.T) {
	const (
		deviceID        = "backlog_device"
		entityID        = "light"
		tickInterval    = 100 * time.Millisecond // script interval
		cmdsPerTick     = 6                      // 3 lights × rgb + brightness
		scriptDuration  = 3 * time.Second        // how long script "runs"
	)

	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")
	store := env.Storage()
	msg := env.Messenger()

	p := &plugin{store: store, stateTopicIndex: make(map[string][]domain.EntityKey)}

	entity := domain.Entity{
		ID: entityID, Plugin: pluginID, DeviceID: deviceID,
		Type: "light", Name: "Backlog Test Light",
		Commands: []string{"light_set_rgb"},
		State:    domain.Light{Power: true},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("save entity: %v", err)
	}
	topicInfo := EntityTopicInfo{
		StateTopic:   "zigbee2mqtt/" + deviceID,
		CommandTopic: "zigbee2mqtt/" + deviceID + "/set",
		EntityType:   "light",
		Discovery:    json.RawMessage(`{}`),
	}
	entityKey := domain.EntityKey{Plugin: pluginID, DeviceID: deviceID, ID: entityID}
	if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
		t.Fatalf("save topic info: %v", err)
	}

	// Track when each command is consumed
	var mu sync.Mutex
	var consumedAt []time.Time
	cmds := messenger.NewCommands(msg, domain.LookupCommand)
	_, err := cmds.Receive(pluginID+".>", func(addr messenger.Address, cmd any) {
		p.handleCommand(addr, cmd)
		mu.Lock()
		consumedAt = append(consumedAt, time.Now())
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Simulate script: fire cmdsPerTick every tickInterval for scriptDuration
	sender := messenger.NewCommands(msg, domain.LookupCommand)
	scriptStart := time.Now()
	totalSent := 0
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for time.Since(scriptStart) < scriptDuration {
		<-ticker.C
		for i := 0; i < cmdsPerTick; i++ {
			sender.Send(entity, domain.LightSetRGB{R: i % 256, G: 0, B: 0})
			totalSent++
		}
	}
	scriptStopped := time.Now()
	t.Logf("Script stopped at %s — sent %d commands over %s",
		scriptStopped.Format("15:04:05.000"), totalSent, scriptDuration)

	// Wait for queue to fully drain
	deadline := time.After(30 * time.Second)
	for {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		n := len(consumedAt)
		mu.Unlock()
		if n >= totalSent {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			n = len(consumedAt)
			mu.Unlock()
			t.Fatalf("timeout: only %d/%d consumed", n, totalSent)
		default:
		}
	}

	mu.Lock()
	lastConsumed := consumedAt[len(consumedAt)-1]
	mu.Unlock()

	drainTime := lastConsumed.Sub(scriptStopped)

	t.Logf("=== Script Backlog Results ===")
	t.Logf("  Script duration  : %s", scriptDuration)
	t.Logf("  Tick interval    : %s", tickInterval)
	t.Logf("  Cmds per tick    : %d", cmdsPerTick)
	t.Logf("  Total sent       : %d", totalSent)
	t.Logf("  Total consumed   : %d", len(consumedAt))
	t.Logf("  Script stopped   : %s", scriptStopped.Format("15:04:05.000"))
	t.Logf("  Last consumed at : %s", lastConsumed.Format("15:04:05.000"))
	t.Logf("  Drain time after stop: %s", drainTime.Round(time.Millisecond))

	if drainTime > 500*time.Millisecond {
		t.Logf("  *** BUG REPRODUCED: commands kept draining for %s after script stopped ***", drainTime.Round(time.Millisecond))
	} else {
		t.Logf("  Queue drained quickly — no significant backlog")
	}

	fmt.Printf("\n[BACKLOG] sent=%d drain=%s (bug reproduced=%v)\n",
		totalSent, drainTime.Round(time.Millisecond), drainTime > 500*time.Millisecond)
}

// TestThroughput_ScriptBacklog_Disk is identical to TestThroughput_ScriptBacklog
// but uses a disk-backed storage server (write-through to a temp dir) to match
// production behaviour. If the drain time here is significantly higher than the
// in-memory variant, disk I/O is confirmed as the bottleneck.
//
// Run:
//
//go test -v -tags integration -run TestThroughput_ScriptBacklog_Disk ./cmd/plugin-zigbee2mqtt/
func TestThroughput_ScriptBacklog_Disk(t *testing.T) {
const (
deviceID       = "backlog_disk_device"
entityID       = "light"
tickInterval   = 100 * time.Millisecond
cmdsPerTick    = 6
scriptDuration = 3 * time.Second
)

// Start messenger only — we'll wire storage manually with a real dataDir.
env := managersdk.NewTestEnv(t)
env.Start("messenger")
msg := env.Messenger()

// Disk-backed storage server using a temp dir.
dataDir := t.TempDir()
t.Logf("Storage dataDir: %s", dataDir)

handler, err := storageserver.NewHandlerWithDir(dataDir)
if err != nil {
t.Fatalf("new handler: %v", err)
}
if err := handler.Register(msg); err != nil {
t.Fatalf("register handler: %v", err)
}
store := storage.ClientFrom(msg)

p := &plugin{store: store, stateTopicIndex: make(map[string][]domain.EntityKey)}

entity := domain.Entity{
ID: entityID, Plugin: pluginID, DeviceID: deviceID,
Type: "light", Name: "Backlog Disk Light",
Commands: []string{"light_set_rgb"},
State:    domain.Light{Power: true},
}
if err := store.Save(entity); err != nil {
t.Fatalf("save entity: %v", err)
}
topicInfo := EntityTopicInfo{
StateTopic:   "zigbee2mqtt/" + deviceID,
CommandTopic: "zigbee2mqtt/" + deviceID + "/set",
EntityType:   "light",
Discovery:    json.RawMessage(`{}`),
}
entityKey := domain.EntityKey{Plugin: pluginID, DeviceID: deviceID, ID: entityID}
if err := p.saveTopicInfo(entityKey, topicInfo); err != nil {
t.Fatalf("save topic info: %v", err)
}

var mu sync.Mutex
var consumedAt []time.Time
cmds := messenger.NewCommands(msg, domain.LookupCommand)
_, err = cmds.Receive(pluginID+".>", func(addr messenger.Address, cmd any) {
p.handleCommand(addr, cmd)
mu.Lock()
consumedAt = append(consumedAt, time.Now())
mu.Unlock()
})
if err != nil {
t.Fatalf("subscribe: %v", err)
}

sender := messenger.NewCommands(msg, domain.LookupCommand)
scriptStart := time.Now()
totalSent := 0
ticker := time.NewTicker(tickInterval)
defer ticker.Stop()
for time.Since(scriptStart) < scriptDuration {
<-ticker.C
for i := 0; i < cmdsPerTick; i++ {
sender.Send(entity, domain.LightSetRGB{R: i % 256, G: 0, B: 0})
totalSent++
}
}
scriptStopped := time.Now()
t.Logf("Script stopped at %s — sent %d commands over %s",
scriptStopped.Format("15:04:05.000"), totalSent, scriptDuration)

deadline := time.After(60 * time.Second)
for {
time.Sleep(100 * time.Millisecond)
mu.Lock()
n := len(consumedAt)
mu.Unlock()
if n >= totalSent {
break
}
select {
case <-deadline:
mu.Lock()
n = len(consumedAt)
mu.Unlock()
t.Fatalf("timeout: only %d/%d consumed", n, totalSent)
default:
}
}

mu.Lock()
lastConsumed := consumedAt[len(consumedAt)-1]
mu.Unlock()

drainTime := lastConsumed.Sub(scriptStopped)

t.Logf("=== Script Backlog Results (DISK-BACKED) ===")
t.Logf("  Storage          : disk (%s)", dataDir)
t.Logf("  Script duration  : %s", scriptDuration)
t.Logf("  Tick interval    : %s", tickInterval)
t.Logf("  Cmds per tick    : %d", cmdsPerTick)
t.Logf("  Total sent       : %d", totalSent)
t.Logf("  Total consumed   : %d", len(consumedAt))
t.Logf("  Script stopped   : %s", scriptStopped.Format("15:04:05.000"))
t.Logf("  Last consumed at : %s", lastConsumed.Format("15:04:05.000"))
t.Logf("  Drain time after stop: %s", drainTime.Round(time.Millisecond))

if drainTime > 500*time.Millisecond {
t.Logf("  *** DISK IS THE BOTTLENECK: drain=%s (in-memory was ~5ms) ***", drainTime.Round(time.Millisecond))
} else {
t.Logf("  Disk overhead negligible — bottleneck is elsewhere")
}

fmt.Printf("\n[BACKLOG-DISK] sent=%d drain=%s (disk bottleneck=%v)\n",
totalSent, drainTime.Round(time.Millisecond), drainTime > 500*time.Millisecond)
}
