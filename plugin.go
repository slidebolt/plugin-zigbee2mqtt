package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/slidebolt/sdk-entities/light"
	runner "github.com/slidebolt/sdk-runner"
	"github.com/slidebolt/sdk-types"
)

type discoveredEntity struct {
	UniqueID     string `json:"unique_id"`
	Name         string `json:"name"`
	DeviceName   string `json:"device_name"`
	DeviceKey    string `json:"device_key"`
	EntityType   string `json:"entity_type"`
	StateTopic   string `json:"state_topic"`
	CommandTopic string `json:"command_topic"`
	PayloadOn    string `json:"payload_on"`
	PayloadOff   string `json:"payload_off"`
	ValueKey     string `json:"value_key"`
}

type PluginZigbee2mqttPlugin struct {
	cfg    z2mConfig
	client mqttClient

	mu         sync.RWMutex
	discovered map[string]discoveredEntity
	rawStore   runner.RawStore
	eventSink  runner.EventSink

	discoveryTimer     *time.Timer
	isInitialDiscovery bool
}

func NewPlugin() *PluginZigbee2mqttPlugin {
	return &PluginZigbee2mqttPlugin{discovered: map[string]discoveredEntity{}}
}

func (p *PluginZigbee2mqttPlugin) OnInitialize(config runner.Config, state types.Storage) (types.Manifest, types.Storage) {
	p.rawStore = config.RawStore
	p.eventSink = config.EventSink
	p.cfg = loadZ2MConfigFromEnv()
	p.discovered = make(map[string]discoveredEntity)
	if len(state.Data) > 0 {
		_ = json.Unmarshal(state.Data, &p.discovered)
	}
	if p.discovered == nil {
		p.discovered = make(map[string]discoveredEntity)
	}
	return types.Manifest{ID: "plugin-zigbee2mqtt", Name: "Plugin Zigbee2mqtt", Version: "1.0.0"}, state
}

func (p *PluginZigbee2mqttPlugin) OnReady() {
	if p.cfg.MQTTURL == "" {
		log.Printf("plugin-zigbee2mqtt: no MQTT URL configured; discovery disabled")
		return
	}

	client := newRealMQTTClient(p.cfg)
	if err := client.Connect(); err != nil {
		log.Printf("plugin-zigbee2mqtt: MQTT connect failed: %v", err)
		return
	}

	p.mu.Lock()
	p.isInitialDiscovery = true
	// Start a timer that will signal we are "ready" after 300ms of MQTT silence
	p.discoveryTimer = time.NewTimer(300 * time.Millisecond)
	p.mu.Unlock()

	done := make(chan struct{})
	go func() {
		<-p.discoveryTimer.C
		p.mu.Lock()
		p.isInitialDiscovery = false
		p.mu.Unlock()
		close(done)
	}()

	if err := client.Subscribe(p.cfg.DiscoveryTopic, p.handleDiscoveryMessage); err != nil {
		log.Printf("plugin-zigbee2mqtt: discovery subscribe failed: %v", err)
		client.Disconnect()
		return
	}

	p.client = client
	log.Printf("plugin-zigbee2mqtt: subscribed to %q, waiting for discovery burst (ready when silent for 300ms)...", p.cfg.DiscoveryTopic)

	// Block OnReady until we have processed the initial burst or timeout
	select {
	case <-done:
		log.Printf("plugin-zigbee2mqtt: initial discovery burst complete")
	case <-time.After(5 * time.Second):
		log.Printf("plugin-zigbee2mqtt: discovery burst wait timed out (hard timeout 5s), proceeding")
	}
}

func (p *PluginZigbee2mqttPlugin) OnShutdown() {
	if p.client != nil {
		p.client.Disconnect()
	}
}

func (p *PluginZigbee2mqttPlugin) OnHealthCheck() (string, error) {
	return "perfect", nil
}

func (p *PluginZigbee2mqttPlugin) OnStorageUpdate(current types.Storage) (types.Storage, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := current
	data, _ := json.Marshal(p.discovered)
	out.Data = data
	return out, nil
}

func (p *PluginZigbee2mqttPlugin) OnDeviceCreate(dev types.Device) (types.Device, error) {
	return dev, nil
}

func (p *PluginZigbee2mqttPlugin) OnDeviceUpdate(dev types.Device) (types.Device, error) {
	return dev, nil
}

func (p *PluginZigbee2mqttPlugin) OnDeviceDelete(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range p.discovered {
		if z2mDeviceID(v.DeviceKey) == id {
			delete(p.discovered, k)
		}
	}
	return nil
}

func (p *PluginZigbee2mqttPlugin) OnDevicesList(current []types.Device) ([]types.Device, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	byID := make(map[string]types.Device, len(current))
	for _, dev := range current {
		byID[dev.ID] = dev
	}

	for _, ent := range p.discovered {
		if ent.DeviceKey == "" {
			continue
		}
		deviceID := z2mDeviceID(ent.DeviceKey)
		name := strings.TrimSpace(ent.DeviceName)
		if name == "" {
			name = ent.DeviceKey
		}
		cfgData, _ := json.Marshal(map[string]any{
			"device_key":  ent.DeviceKey,
			"device_name": ent.DeviceName,
		})
		
		if p.rawStore != nil {
			_ = p.rawStore.WriteRawDevice(deviceID, cfgData)
		}

		discoveredDev := types.Device{
			ID:         deviceID,
			SourceID:   ent.DeviceKey,
			SourceName: name,
		}

		if existing, ok := byID[deviceID]; ok {
			byID[deviceID] = runner.ReconcileDevice(existing, discoveredDev)
		} else {
			byID[deviceID] = runner.ReconcileDevice(types.Device{}, discoveredDev)
		}
	}

	out := make([]types.Device, 0, len(byID))
	for _, dev := range byID {
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *PluginZigbee2mqttPlugin) OnDeviceSearch(q types.SearchQuery, res []types.Device) ([]types.Device, error) {
	return res, nil
}

func (p *PluginZigbee2mqttPlugin) OnEntityCreate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginZigbee2mqttPlugin) OnEntityUpdate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginZigbee2mqttPlugin) OnEntityDelete(d, e string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range p.discovered {
		if z2mDeviceID(v.DeviceKey) == d && z2mEntityID(v.UniqueID) == e {
			delete(p.discovered, k)
			break
		}
	}
	return nil
}

func (p *PluginZigbee2mqttPlugin) OnEntitiesList(d string, c []types.Entity) ([]types.Entity, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	log.Printf("plugin-zigbee2mqtt: OnEntitiesList called for device %q, current entities count: %d", d, len(c))

	byID := make(map[string]types.Entity, len(c))
	for _, ent := range c {
		byID[ent.ID] = ent
	}

	for _, discovered := range p.discovered {
		if z2mDeviceID(discovered.DeviceKey) != d {
			continue
		}
		entityID := z2mEntityID(discovered.UniqueID)
		log.Printf("plugin-zigbee2mqtt: match found! entity %q (domain=%s) for device %q", entityID, discovered.EntityType, d)
		cfgData, _ := json.Marshal(map[string]any{
			"state_topic":   discovered.StateTopic,
			"command_topic": discovered.CommandTopic,
			"payload_on":    discovered.PayloadOn,
			"payload_off":   discovered.PayloadOff,
			"value_key":     discovered.ValueKey,
		})
		if p.rawStore != nil {
			_ = p.rawStore.WriteRawEntity(d, entityID, cfgData)
		}
		name := strings.TrimSpace(discovered.Name)
		if name == "" {
			name = discovered.UniqueID
		}

		ent := types.Entity{
			ID:        entityID,
			DeviceID:  d,
			Domain:    mapDomain(discovered.EntityType),
			LocalName: name,
		}

		if existing, ok := byID[entityID]; ok {
			ent.Data = existing.Data
			byID[entityID] = ent
		} else {
			byID[entityID] = ent
		}
	}

	out := make([]types.Entity, 0, len(byID))
	for _, ent := range byID {
		out = append(out, ent)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *PluginZigbee2mqttPlugin) OnCommand(cmd types.Command, entity types.Entity) (types.Entity, error) {
	if p.client == nil {
		return entity, fmt.Errorf("MQTT client not connected")
	}

	p.mu.RLock()
	var ent discoveredEntity
	found := false
	for _, v := range p.discovered {
		if z2mEntityID(v.UniqueID) == entity.ID {
			ent = v
			found = true
			break
		}
	}
	p.mu.RUnlock()

	if !found || ent.CommandTopic == "" {
		return entity, fmt.Errorf("command topic not found for entity %s", entity.ID)
	}

	lc, err := light.ParseCommand(cmd)
	if err != nil {
		return entity, err
	}

	var payload string
	switch lc.Type {
	case light.ActionTurnOn:
		payload = ent.PayloadOn
	case light.ActionTurnOff:
		payload = ent.PayloadOff
	case light.ActionSetRGB:
		if lc.RGB == nil || len(*lc.RGB) != 3 {
			return entity, fmt.Errorf("invalid rgb payload")
		}
		rgb := *lc.RGB
		payload = fmt.Sprintf(`{"color":{"r":%v,"g":%v,"b":%v}}`, rgb[0], rgb[1], rgb[2])
	default:
		return entity, fmt.Errorf("unsupported command type: %s", lc.Type)
	}

	if err := p.client.Publish(ent.CommandTopic, payload); err != nil {
		return entity, err
	}

	entity.Data.SyncStatus = "pending"

	if p.eventSink != nil {
		deviceID := entity.DeviceID
		entityID := entity.ID
		correlationID := cmd.ID
		go func() {
			time.Sleep(20 * time.Millisecond)
			_ = p.eventSink.EmitEvent(types.InboundEvent{
				DeviceID:      deviceID,
				EntityID:      entityID,
				CorrelationID: correlationID,
				Payload:       []byte(payload),
			})
		}()
	}

	return entity, nil
}

func (p *PluginZigbee2mqttPlugin) OnEvent(evt types.Event, entity types.Entity) (types.Entity, error) {
	entity.Data.Reported = evt.Payload
	entity.Data.SyncStatus = "in_sync"
	return entity, nil
}

func (p *PluginZigbee2mqttPlugin) handleDiscoveryMessage(topic string, payload []byte) {
	log.Printf("plugin-zigbee2mqtt: [MQTT] received discovery on %q", topic)
	data, entityType, err := parseDiscovery(topic, payload)
	if err != nil {
		log.Printf("plugin-zigbee2mqtt: [MQTT] discovery parse failed: %v", err)
		return
	}
	entry := discoveredEntity{
		UniqueID:     data.UniqueID,
		Name:         data.Name,
		DeviceName:   data.Device.Name,
		DeviceKey:    deviceKeyFromDiscovery(data),
		EntityType:   entityType,
		StateTopic:   data.StateTopic,
		CommandTopic: data.CommandTopic,
		PayloadOn:    payloadToString(data.PayloadOn),
		PayloadOff:   payloadToString(data.PayloadOff),
		ValueKey:     extractValueKey(data.ValueTemplate),
	}
	log.Printf("plugin-zigbee2mqtt: [DISCOVERY] unique_id=%q type=%s device_name=%q device_key=%q", entry.UniqueID, entry.EntityType, entry.DeviceName, entry.DeviceKey)

	p.mu.Lock()
	p.discovered[data.UniqueID] = entry
	if p.isInitialDiscovery && p.discoveryTimer != nil {
		p.discoveryTimer.Reset(100 * time.Millisecond)
	}
	p.mu.Unlock()

	if p.client != nil && entry.StateTopic != "" {
		topic := entry.StateTopic
		go func() {
			log.Printf("plugin-zigbee2mqtt: [STATE] subscribing to %q for entity %q", topic, entry.UniqueID)
			_ = p.client.Subscribe(topic, func(topic string, payload []byte) {
				if p.eventSink != nil {
					p.eventSink.EmitEvent(types.InboundEvent{
						DeviceID: z2mDeviceID(entry.DeviceKey),
						EntityID: z2mEntityID(entry.UniqueID),
						Payload:  payload,
					})
				}
			})
		}()
	}
}

func deviceKeyFromDiscovery(data *haDiscoveryPayload) string {
	if data == nil || len(data.Device.Identifiers) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", data.Device.Identifiers[0])
}

func z2mDeviceID(deviceKey string) string {
	return "z2m-device-" + sanitizeID(deviceKey)
}

func z2mEntityID(uniqueID string) string {
	return "z2m-entity-" + sanitizeID(uniqueID)
}

func mapDomain(entityType string) string {
	switch entityType {
	case "light", "switch", "binary_sensor", "sensor", "cover":
		return entityType
	default:
		return "sensor"
	}
}

func sanitizeID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteByte(ch)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if out == "" {
		return "unknown"
	}
	return out
}