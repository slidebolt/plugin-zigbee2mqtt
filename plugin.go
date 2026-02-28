package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

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
}

func NewPlugin() *PluginZigbee2mqttPlugin {
	return &PluginZigbee2mqttPlugin{discovered: map[string]discoveredEntity{}}
}

func (p *PluginZigbee2mqttPlugin) OnInitialize(_ runner.Config, state types.Storage) (types.Manifest, types.Storage) {
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
	if err := client.Subscribe(p.cfg.DiscoveryTopic, p.handleDiscoveryMessage); err != nil {
		log.Printf("plugin-zigbee2mqtt: discovery subscribe failed: %v", err)
		client.Disconnect()
		return
	}
	p.client = client
	log.Printf("plugin-zigbee2mqtt: subscribed to %q", p.cfg.DiscoveryTopic)
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
	dev.Config = types.Storage{Meta: "plugin-zigbee2mqtt-metadata"}
	return dev, nil
}

func (p *PluginZigbee2mqttPlugin) OnDeviceUpdate(dev types.Device) (types.Device, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// If the update only contains a subset of fields, we should ideally merge it
	// but for now, we just ensure it's not empty.
	return dev, nil
}
func (p *PluginZigbee2mqttPlugin) OnDeviceDelete(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find the device key from the ID (reverse of z2mDeviceID)
	// Actually, we can just iterate and remove anything that matches this device ID
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
		
		discoveredDev := types.Device{
			ID:         deviceID,
			SourceID:   ent.DeviceKey,
			SourceName: name,
			// LocalName intentionally left blank; the Wall handles it
			Config:     types.Storage{Meta: "z2m-device", Data: cfgData},
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

	byID := make(map[string]types.Entity, len(c))
	for _, ent := range c {
		byID[ent.ID] = ent
	}

	for _, discovered := range p.discovered {
		if z2mDeviceID(discovered.DeviceKey) != d {
			continue
		}
		entityID := z2mEntityID(discovered.UniqueID)
		cfgData, _ := json.Marshal(map[string]any{
			"state_topic":   discovered.StateTopic,
			"command_topic": discovered.CommandTopic,
			"payload_on":    discovered.PayloadOn,
			"payload_off":   discovered.PayloadOff,
			"value_key":     discovered.ValueKey,
		})
		name := strings.TrimSpace(discovered.Name)
		if name == "" {
			name = discovered.UniqueID
		}
		byID[entityID] = types.Entity{
			ID:        entityID,
			DeviceID:  d,
			Domain:    mapDomain(discovered.EntityType),
			LocalName: name,
			Config:    types.Storage{Meta: "z2m-entity", Data: cfgData},
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
	return entity, nil
}

func (p *PluginZigbee2mqttPlugin) OnEvent(evt types.Event, entity types.Entity) (types.Entity, error) {
	return entity, nil
}

func (p *PluginZigbee2mqttPlugin) handleDiscoveryMessage(topic string, payload []byte) {
	data, entityType, err := parseDiscovery(topic, payload)
	if err != nil {
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

	p.mu.Lock()
	p.discovered[data.UniqueID] = entry
	p.mu.Unlock()
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
	out = strings.ReplaceAll(out, "--", "-")
	if out == "" {
		return "unknown"
	}
	return out
}
