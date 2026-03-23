// plugin-zigbee2mqtt - SlideBolt plugin for Zigbee2MQTT integration
//
// This plugin connects to an MQTT broker and integrates with Zigbee2MQTT
// to control Zigbee devices through SlideBolt.
//
// Architecture:
//   - Subscribes to homeassistant/# for device discovery
//   - Subscribes to zigbee2mqtt/<device> for device state updates
//   - Publishes to zigbee2mqtt/<device>/set for device commands
//   - Stores entities in SlideBolt storage
//   - Stores MQTT topic mappings in internal storage
package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	translate "github.com/slidebolt/plugin-zigbee2mqtt/internal/translate"
	contract "github.com/slidebolt/sb-contract"
	domain "github.com/slidebolt/sb-domain"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
)

const pluginID = "plugin-zigbee2mqtt"
const PluginID = pluginID

type DiscoveryPayload = translate.DiscoveryPayload
type App = plugin

func New() *App { return &plugin{} }

func DecodeWithMeta(entityType string, raw json.RawMessage, valueField, unit, deviceClass string) (any, bool) {
	return translate.DecodeWithMeta(entityType, raw, valueField, unit, deviceClass)
}

func Encode(cmd any, internal json.RawMessage) (json.RawMessage, error) {
	return translate.Encode(cmd, internal)
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// MQTTConfig holds MQTT broker configuration
// Can be set via environment variables:
//
//	Z2M_MQTT_BROKER - MQTT broker URL (default: tcp://localhost:1883)
//	Z2M_MQTT_USERNAME - MQTT username (optional)
//	Z2M_MQTT_PASSWORD - MQTT password (optional)
//	Z2M_DISCOVERY_PREFIX - HA discovery prefix (default: homeassistant)
//	Z2M_BASE_TOPIC - Z2M base topic (default: zigbee2mqtt)
type MQTTConfig struct {
	Broker          string `json:"broker"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	DiscoveryPrefix string `json:"discovery_prefix"`
	BaseTopic       string `json:"base_topic"`
	ClientID        string `json:"client_id"`
}

func loadMQTTConfig() MQTTConfig {
	cfg := MQTTConfig{
		Broker:          getEnv("Z2M_MQTT_BROKER", "tcp://localhost:1883"),
		Username:        getEnv("Z2M_MQTT_USERNAME", ""),
		Password:        getEnv("Z2M_MQTT_PASSWORD", ""),
		DiscoveryPrefix: getEnv("Z2M_DISCOVERY_PREFIX", "homeassistant"),
		BaseTopic:       getEnv("Z2M_BASE_TOPIC", "zigbee2mqtt"),
		ClientID:        getEnv("Z2M_CLIENT_ID", "slidebolt-z2m-plugin"),
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// EntityTopicInfo stores MQTT topic mappings for an entity in internal storage
type EntityTopicInfo struct {
	StateTopic   string          `json:"state_topic"`
	CommandTopic string          `json:"command_topic"`
	Availability string          `json:"availability_topic,omitempty"`
	Discovery    json.RawMessage `json:"discovery,omitempty"`
	EntityType   string          `json:"entity_type"`
	DeviceID     string          `json:"device_id"`
	FriendlyName string          `json:"friendly_name"`
	// Sensor-specific: extracted from HA discovery value_template for field-accurate decoding
	ValueField        string `json:"value_field,omitempty"`
	UnitOfMeasurement string `json:"unit_of_measurement,omitempty"`
	SensorDeviceClass string `json:"sensor_device_class,omitempty"`
}

// ---------------------------------------------------------------------------
// Plugin struct
// ---------------------------------------------------------------------------

type plugin struct {
	msg     messenger.Messenger
	store   storage.Storage
	cmds    *messenger.Commands
	subs    []messenger.Subscription
	mqtt    mqtt.Client
	mqttCfg MQTTConfig

	// stateTopicIndex maps MQTT state topics (e.g. "zigbee2mqtt/Main_LB_01")
	// to the entity keys that share that topic. Built during discovery.
	mu              sync.RWMutex
	stateTopicIndex map[string][]domain.EntityKey
}

func (p *plugin) Hello() contract.HelloResponse {
	return contract.HelloResponse{
		ID:              pluginID,
		Kind:            contract.KindPlugin,
		ContractVersion: contract.ContractVersion,
		DependsOn:       []string{"messenger", "storage"},
	}
}

func (p *plugin) OnStart(deps map[string]json.RawMessage) (json.RawMessage, error) {
	// Load MQTT configuration
	p.mqttCfg = loadMQTTConfig()
	p.stateTopicIndex = make(map[string][]domain.EntityKey)

	// Connect to Messenger SDK
	msg, err := messenger.Connect(deps)
	if err != nil {
		return nil, fmt.Errorf("connect messenger: %w", err)
	}
	p.msg = msg

	// Connect to Storage SDK
	store, err := storage.Connect(deps)
	if err != nil {
		return nil, fmt.Errorf("connect storage: %w", err)
	}
	p.store = store

	// Wire up typed command dispatch
	p.cmds = messenger.NewCommands(msg, domain.LookupCommand)
	sub, err := p.cmds.Receive(pluginID+".>", p.handleCommand)
	if err != nil {
		return nil, fmt.Errorf("subscribe commands: %w", err)
	}
	p.subs = append(p.subs, sub)

	// Connect to MQTT broker
	if err := p.connectMQTT(); err != nil {
		log.Printf("plugin-zigbee2mqtt: MQTT connection failed: %v", err)
		log.Println("plugin-zigbee2mqtt: continuing without MQTT - set Z2M_MQTT_BROKER to enable")
	} else {
		log.Println("plugin-zigbee2mqtt: MQTT connected successfully")
	}

	// Seed demo device (only if MQTT is not connected - real devices will come from Z2M)
	if p.mqtt == nil || !p.mqtt.IsConnected() {
		if err := p.seedDemo(); err != nil {
			return nil, fmt.Errorf("seed demo: %w", err)
		}
	}

	log.Println("plugin-zigbee2mqtt: started")
	return nil, nil
}

// connectMQTT establishes connection to MQTT broker and subscribes to topics
func (p *plugin) connectMQTT() error {
	opts := mqtt.NewClientOptions().
		AddBroker(p.mqttCfg.Broker).
		SetClientID(p.mqttCfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(p.onMQTTConnect).
		SetConnectionLostHandler(p.onMQTTDisconnect)

	if p.mqttCfg.Username != "" {
		opts.SetUsername(p.mqttCfg.Username)
		opts.SetPassword(p.mqttCfg.Password)
	}

	p.mqtt = mqtt.NewClient(opts)

	// Connect with timeout
	token := p.mqtt.Connect()
	token.WaitTimeout(10 * time.Second)
	if token.Error() != nil {
		return fmt.Errorf("MQTT connect: %w", token.Error())
	}

	return nil
}

func parseDiscoveryTopic(topic string) (entityType, deviceID, entityID string, ok bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 4 || parts[len(parts)-1] != "config" {
		return "", "", "", false
	}
	entityType = parts[1]
	deviceID = parts[2]
	if len(parts) == 4 {
		// homeassistant/<type>/<id>/config — single entity per device.
		// Use the entity type as the ID so it doesn't collide with
		// other entity types on the same device.
		entityID = entityType
	} else {
		// homeassistant/<type>/<deviceID>/<subID>/config — multiple
		// entities per device. Use the sub-ID directly.
		entityID = parts[len(parts)-2]
	}
	return entityType, deviceID, entityID, true
}

func resolveEntityName(discovery DiscoveryPayload, entityType, objectID string) string {
	if discovery.Name != "" {
		return discovery.Name
	}
	if deviceName := discovery.DeviceName(); deviceName != "" {
		return deviceName
	}
	if discovery.StateTopic != "" {
		parts := strings.Split(discovery.StateTopic, "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			return parts[len(parts)-1]
		}
	}
	return fmt.Sprintf("%s_%s", entityType, objectID)
}

// extractValueField parses "{{ value_json.fieldname }}" → "fieldname".
// Returns "" if the template doesn't match this pattern.
func extractValueField(valueTemplate string) string {
	t := strings.TrimSpace(valueTemplate)
	t = strings.TrimPrefix(t, "{{")
	t = strings.TrimSuffix(t, "}}")
	t = strings.TrimSpace(t)
	// expect "value_json.fieldname"
	if !strings.HasPrefix(t, "value_json.") {
		return ""
	}
	return strings.TrimPrefix(t, "value_json.")
}

// onMQTTConnect is called when MQTT connection is established (initial or reconnect)
func (p *plugin) onMQTTConnect(client mqtt.Client) {
	log.Println("plugin-zigbee2mqtt: MQTT connected, subscribing to topics...")

	// Subscribe to HA discovery topic
	discoveryTopic := p.mqttCfg.DiscoveryPrefix + "/#"
	token := client.Subscribe(discoveryTopic, 0, p.handleDiscoveryMessage)
	token.WaitTimeout(5 * time.Second)
	if token.Error() != nil {
		log.Printf("plugin-zigbee2mqtt: failed to subscribe to discovery: %v", token.Error())
	} else {
		log.Printf("plugin-zigbee2mqtt: subscribed to %s", discoveryTopic)
	}

	// Subscribe to all Z2M state topics via wildcard — avoids per-device subscriptions
	// from within MQTT callbacks (which deadlocks the Paho inbound goroutine).
	stateTopic := p.mqttCfg.BaseTopic + "/#"
	token2 := client.Subscribe(stateTopic, 0, p.handleStateMessage)
	token2.WaitTimeout(5 * time.Second)
	if token2.Error() != nil {
		log.Printf("plugin-zigbee2mqtt: failed to subscribe to state: %v", token2.Error())
	} else {
		log.Printf("plugin-zigbee2mqtt: subscribed to %s", stateTopic)
	}
}

// onMQTTDisconnect is called when MQTT connection is lost
func (p *plugin) onMQTTDisconnect(client mqtt.Client, err error) {
	log.Printf("plugin-zigbee2mqtt: MQTT disconnected: %v", err)
	log.Println("plugin-zigbee2mqtt: will auto-reconnect...")
}

// handleDiscoveryMessage processes HA discovery messages
func (p *plugin) handleDiscoveryMessage(client mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	// Parse discovery topic: homeassistant/<domain>/<deviceID>[/<subID>]/config
	entityType, deviceID, entityID, ok := parseDiscoveryTopic(topic)
	if !ok {
		return // Not a discovery message
	}

	// Parse discovery payload to get topics
	var discovery DiscoveryPayload
	if err := json.Unmarshal(payload, &discovery); err != nil {
		log.Printf("plugin-zigbee2mqtt: failed to parse discovery for %s/%s: %v", entityType, entityID, err)
		return
	}

	// Log discovery with name for debugging
	if discovery.Name != "" {
		log.Printf("plugin-zigbee2mqtt: discovered %s: %s (name: %s)", entityType, entityID, discovery.Name)
	} else {
		log.Printf("plugin-zigbee2mqtt: discovered %s: %s (no name)", entityType, entityID)
	}

	entityKey := domain.EntityKey{
		Plugin:   pluginID,
		DeviceID: deviceID,
		ID:       entityID,
	}

	entityName := resolveEntityName(discovery, entityType, entityID)

	// Check if entity already exists
	existingRaw, err := p.store.Get(entityKey)
	if err == nil {
		// Entity exists, update with latest discovery info.
		var existingEntity domain.Entity
		if err := json.Unmarshal(existingRaw, &existingEntity); err == nil {
			existingEntity.Type = entityType
			existingEntity.Name = entityName
			existingEntity.Commands = p.getCommandsForType(entityType)
			p.store.Save(existingEntity)
		}

		topicInfo := EntityTopicInfo{
			StateTopic:        discovery.StateTopic,
			CommandTopic:      discovery.CommandTopic,
			Availability:      discovery.AvailabilityTopic,
			Discovery:         json.RawMessage(payload),
			EntityType:        entityType,
			DeviceID:          deviceID,
			FriendlyName:      entityName,
			ValueField:        extractValueField(discovery.ValueTemplate),
			UnitOfMeasurement: discovery.UnitOfMeasurement,
			SensorDeviceClass: discovery.DeviceClass,
		}
		p.saveTopicInfo(entityKey, topicInfo)

		log.Printf("plugin-zigbee2mqtt: updated entity %s (%s)", entityKey.Key(), entityName)
		return
	}

	// Create new entity from discovery
	entity := domain.Entity{
		ID:       entityID,
		Plugin:   pluginID,
		DeviceID: deviceID,
		Type:     entityType,
		Name:     entityName,
		Commands: p.getCommandsForType(entityType),
		State:    nil, // Will be populated when state message arrives
	}

	if err := p.store.Save(entity); err != nil {
		log.Printf("plugin-zigbee2mqtt: failed to save entity %s: %v", entityKey.Key(), err)
		return
	}

	topicInfo := EntityTopicInfo{
		StateTopic:        discovery.StateTopic,
		CommandTopic:      discovery.CommandTopic,
		Availability:      discovery.AvailabilityTopic,
		Discovery:         json.RawMessage(payload),
		EntityType:        entityType,
		DeviceID:          deviceID,
		FriendlyName:      entityName,
		ValueField:        extractValueField(discovery.ValueTemplate),
		UnitOfMeasurement: discovery.UnitOfMeasurement,
		SensorDeviceClass: discovery.DeviceClass,
	}
	p.saveTopicInfo(entityKey, topicInfo)

	log.Printf("plugin-zigbee2mqtt: created entity %s (%s)", entityKey.Key(), entityName)
}

// handleStateMessage processes device state updates.
// Multiple entities can share the same Z2M state topic (e.g. a light and its
// update_available sensor both subscribe to zigbee2mqtt/<device>). We find all
// matching entities via the in-memory stateTopicIndex built during discovery.
func (p *plugin) handleStateMessage(client mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	// Skip bridge system messages (not device state updates)
	if strings.Contains(topic, "/bridge/") {
		return
	}

	p.mu.RLock()
	keys := p.stateTopicIndex[topic]
	p.mu.RUnlock()

	if len(keys) == 0 {
		return
	}

	updated := 0
	for _, key := range keys {
		topicInfo, err := p.getTopicInfo(key)
		if err != nil {
			continue
		}
		raw, err := p.store.Get(key)
		if err != nil {
			continue
		}
		var entity domain.Entity
		if err := json.Unmarshal(raw, &entity); err != nil {
			continue
		}
		state, ok := DecodeWithMeta(entity.Type, payload, topicInfo.ValueField, topicInfo.UnitOfMeasurement, topicInfo.SensorDeviceClass)
		if !ok {
			continue
		}
		entity.State = state
		if err := p.store.Save(entity); err != nil {
			log.Printf("plugin-zigbee2mqtt: failed to update entity %s: %v", key.Key(), err)
			continue
		}
		updated++
	}
	if updated > 0 {
		log.Printf("plugin-zigbee2mqtt: state update on %s — updated %d entities", topic, updated)
	}
}

// getTopicInfo retrieves topic mappings from internal storage
func (p *plugin) getTopicInfo(key domain.EntityKey) (EntityTopicInfo, error) {
	var info EntityTopicInfo
	data, err := p.store.ReadFile(storage.Internal, key)
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return info, err
	}
	return info, nil
}

// saveTopicInfo stores topic mappings in internal storage
func (p *plugin) saveTopicInfo(key domain.EntityKey, info EntityTopicInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := p.store.WriteFile(storage.Internal, key, data); err != nil {
		return err
	}
	// Index the state topic for fast lookup in handleStateMessage.
	if info.StateTopic != "" {
		p.mu.Lock()
		p.stateTopicIndex[info.StateTopic] = appendUniqueKey(p.stateTopicIndex[info.StateTopic], key)
		p.mu.Unlock()
	}
	return nil
}

func appendUniqueKey(keys []domain.EntityKey, key domain.EntityKey) []domain.EntityKey {
	for _, k := range keys {
		if k == key {
			return keys
		}
	}
	return append(keys, key)
}

// getCommandsForType returns the list of supported commands for an entity type
func (p *plugin) getCommandsForType(entityType string) []string {
	switch entityType {
	case "light":
		return []string{"light_turn_on", "light_turn_off", "light_set_brightness", "light_set_color_temp", "light_set_rgb", "light_set_effect"}
	case "switch":
		return []string{"switch_turn_on", "switch_turn_off", "switch_toggle"}
	case "cover":
		return []string{"cover_open", "cover_close", "cover_set_position"}
	case "lock":
		return []string{"lock_lock", "lock_unlock"}
	case "fan":
		return []string{"fan_turn_on", "fan_turn_off", "fan_set_speed"}
	case "climate":
		return []string{"climate_set_mode", "climate_set_temperature"}
	case "button":
		return []string{"button_press"}
	case "number":
		return []string{"number_set_value"}
	case "select":
		return []string{"select_option"}
	case "text":
		return []string{"text_set_value"}
	default:
		return []string{}
	}
}

func (p *plugin) OnShutdown() error {
	// Disconnect MQTT
	if p.mqtt != nil && p.mqtt.IsConnected() {
		p.mqtt.Disconnect(250)
	}

	// Unsubscribe from messenger
	for _, sub := range p.subs {
		sub.Unsubscribe()
	}

	// Close storage
	if p.store != nil {
		p.store.Close()
	}

	// Close messenger
	if p.msg != nil {
		p.msg.Close()
	}

	return nil
}

// ---------------------------------------------------------------------------
// Command handler — dispatches to per-entity-type handlers and publishes to MQTT
// ---------------------------------------------------------------------------

func (p *plugin) handleCommand(addr messenger.Address, cmd any) {
	recvAt := time.Now()
	log.Printf("plugin-zigbee2mqtt: [CMD] RECV entity=%s type=%T", addr.Key(), cmd)

	// Get entity info
	entityKey := domain.EntityKey{
		Plugin:   addr.Plugin,
		DeviceID: addr.DeviceID,
		ID:       addr.EntityID,
	}

	// Get entity from storage
	raw, err := p.store.Get(entityKey)
	if err != nil {
		log.Printf("plugin-zigbee2mqtt: command for unknown entity %s: %v", addr.Key(), err)
		return
	}

	var entity domain.Entity
	if err := json.Unmarshal(raw, &entity); err != nil {
		log.Printf("plugin-zigbee2mqtt: failed to parse entity %s: %v", addr.Key(), err)
		return
	}

	// Get topic info for command publishing
	topicInfo, err := p.getTopicInfo(entityKey)
	if err != nil {
		log.Printf("plugin-zigbee2mqtt: no topic info for %s: %v", addr.Key(), err)
		// Continue to log commands even if we can't publish (for testing)
	}

	// Encode command to Z2M JSON
	internal := json.RawMessage("{}")
	if topicInfo.Discovery != nil {
		internal = topicInfo.Discovery
	}

	payload, err := Encode(cmd, internal)
	if err != nil {
		log.Printf("plugin-zigbee2mqtt: failed to encode command %T: %v", cmd, err)
		return
	}

	// Publish to MQTT if connected
	cmdType := fmt.Sprintf("%T", cmd)
	log.Printf("plugin-zigbee2mqtt: [CMD] entity=%s type=%s payload=%s", addr.Key(), cmdType, string(payload))
	if p.mqtt != nil && p.mqtt.IsConnected() && topicInfo.CommandTopic != "" {
		payloadBytes := []byte(string(payload))
		publishStart := time.Now()
		token := p.mqtt.Publish(topicInfo.CommandTopic, 0, false, payloadBytes)
		acked := token.WaitTimeout(5 * time.Second)
		elapsed := time.Since(publishStart)
		if token.Error() != nil {
			log.Printf("plugin-zigbee2mqtt: [CMD] FAIL topic=%s elapsed=%s ack=%v err=%v", topicInfo.CommandTopic, elapsed.Round(time.Millisecond), acked, token.Error())
		} else {
			log.Printf("plugin-zigbee2mqtt: [CMD] OK   topic=%s elapsed=%s ack=%v", topicInfo.CommandTopic, elapsed.Round(time.Millisecond), acked)
		}
	} else {
		log.Printf("plugin-zigbee2mqtt: [CMD] SKIP entity=%s (MQTT not connected)", addr.Key())
	}

	// Update local entity state optimistically (optional)
	// This could be done here or wait for state message from device
	switch c := cmd.(type) {
	case domain.LightTurnOn:
		log.Printf("plugin-zigbee2mqtt: light %s turn_on", addr.Key())
		if light, ok := entity.State.(domain.Light); ok {
			light.Power = true
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightTurnOff:
		log.Printf("plugin-zigbee2mqtt: light %s turn_off", addr.Key())
		if light, ok := entity.State.(domain.Light); ok {
			light.Power = false
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightSetBrightness:
		log.Printf("plugin-zigbee2mqtt: light %s set_brightness brightness=%d", addr.Key(), c.Brightness)
		if light, ok := entity.State.(domain.Light); ok {
			light.Power = true
			light.Brightness = c.Brightness
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightSetColorTemp:
		log.Printf("plugin-zigbee2mqtt: light %s set_color_temp mireds=%d", addr.Key(), c.Mireds)
		if light, ok := entity.State.(domain.Light); ok {
			light.Temperature = c.Mireds
			if c.Brightness > 0 {
				light.Brightness = c.Brightness
			}
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightSetRGB:
		log.Printf("plugin-zigbee2mqtt: light %s set_rgb r=%d g=%d b=%d", addr.Key(), c.R, c.G, c.B)
		if light, ok := entity.State.(domain.Light); ok {
			light.Power = true
			light.RGB = []int{c.R, c.G, c.B}
			if c.Brightness > 0 {
				light.Brightness = c.Brightness
			}
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightSetRGBW:
		log.Printf("plugin-zigbee2mqtt: light %s set_rgbw r=%d g=%d b=%d w=%d", addr.Key(), c.R, c.G, c.B, c.W)
		if light, ok := entity.State.(domain.Light); ok {
			light.Power = true
			light.RGBW = []int{c.R, c.G, c.B, c.W}
			if c.Brightness > 0 {
				light.Brightness = c.Brightness
			}
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightSetRGBWW:
		log.Printf("plugin-zigbee2mqtt: light %s set_rgbww r=%d g=%d b=%d cw=%d ww=%d", addr.Key(), c.R, c.G, c.B, c.CW, c.WW)
		if light, ok := entity.State.(domain.Light); ok {
			light.Power = true
			light.RGBWW = []int{c.R, c.G, c.B, c.CW, c.WW}
			if c.Brightness > 0 {
				light.Brightness = c.Brightness
			}
			entity.State = light
			p.store.Save(entity)
		}
	case domain.LightSetHS:
		log.Printf("plugin-zigbee2mqtt: light %s set_hs hue=%.1f sat=%.1f", addr.Key(), c.Hue, c.Saturation)
	case domain.LightSetXY:
		log.Printf("plugin-zigbee2mqtt: light %s set_xy x=%.4f y=%.4f", addr.Key(), c.X, c.Y)
	case domain.LightSetWhite:
		log.Printf("plugin-zigbee2mqtt: light %s set_white white=%d", addr.Key(), c.White)
	case domain.LightSetEffect:
		log.Printf("plugin-zigbee2mqtt: light %s set_effect effect=%s", addr.Key(), c.Effect)
	case domain.SwitchTurnOn:
		log.Printf("plugin-zigbee2mqtt: switch %s turn_on", addr.Key())
		if sw, ok := entity.State.(domain.Switch); ok {
			sw.Power = true
			entity.State = sw
			p.store.Save(entity)
		}
	case domain.SwitchTurnOff:
		log.Printf("plugin-zigbee2mqtt: switch %s turn_off", addr.Key())
		if sw, ok := entity.State.(domain.Switch); ok {
			sw.Power = false
			entity.State = sw
			p.store.Save(entity)
		}
	case domain.SwitchToggle:
		log.Printf("plugin-zigbee2mqtt: switch %s toggle", addr.Key())
		if sw, ok := entity.State.(domain.Switch); ok {
			sw.Power = !sw.Power
			entity.State = sw
			p.store.Save(entity)
		}
	case domain.FanTurnOn:
		log.Printf("plugin-zigbee2mqtt: fan %s turn_on", addr.Key())
		if fan, ok := entity.State.(domain.Fan); ok {
			fan.Power = true
			entity.State = fan
			p.store.Save(entity)
		}
	case domain.FanTurnOff:
		log.Printf("plugin-zigbee2mqtt: fan %s turn_off", addr.Key())
		if fan, ok := entity.State.(domain.Fan); ok {
			fan.Power = false
			entity.State = fan
			p.store.Save(entity)
		}
	case domain.FanSetSpeed:
		log.Printf("plugin-zigbee2mqtt: fan %s set_speed percentage=%d", addr.Key(), c.Percentage)
		if fan, ok := entity.State.(domain.Fan); ok {
			fan.Percentage = c.Percentage
			entity.State = fan
			p.store.Save(entity)
		}
	case domain.CoverOpen:
		log.Printf("plugin-zigbee2mqtt: cover %s open", addr.Key())
		if cover, ok := entity.State.(domain.Cover); ok {
			cover.Position = 100
			entity.State = cover
			p.store.Save(entity)
		}
	case domain.CoverClose:
		log.Printf("plugin-zigbee2mqtt: cover %s close", addr.Key())
		if cover, ok := entity.State.(domain.Cover); ok {
			cover.Position = 0
			entity.State = cover
			p.store.Save(entity)
		}
	case domain.CoverSetPosition:
		log.Printf("plugin-zigbee2mqtt: cover %s set_position pos=%d", addr.Key(), c.Position)
		if cover, ok := entity.State.(domain.Cover); ok {
			cover.Position = c.Position
			entity.State = cover
			p.store.Save(entity)
		}
	case domain.LockLock:
		log.Printf("plugin-zigbee2mqtt: lock %s lock", addr.Key())
		if lock, ok := entity.State.(domain.Lock); ok {
			lock.Locked = true
			entity.State = lock
			p.store.Save(entity)
		}
	case domain.LockUnlock:
		log.Printf("plugin-zigbee2mqtt: lock %s unlock", addr.Key())
		if lock, ok := entity.State.(domain.Lock); ok {
			lock.Locked = false
			entity.State = lock
			p.store.Save(entity)
		}
	case domain.ButtonPress:
		log.Printf("plugin-zigbee2mqtt: button %s press", addr.Key())
	case domain.NumberSetValue:
		log.Printf("plugin-zigbee2mqtt: number %s set_value value=%v", addr.Key(), c.Value)
		if num, ok := entity.State.(domain.Number); ok {
			num.Value = c.Value
			entity.State = num
			p.store.Save(entity)
		}
	case domain.SelectOption:
		log.Printf("plugin-zigbee2mqtt: select %s set_option option=%s", addr.Key(), c.Option)
		if sel, ok := entity.State.(domain.Select); ok {
			sel.Option = c.Option
			entity.State = sel
			p.store.Save(entity)
		}
	case domain.TextSetValue:
		log.Printf("plugin-zigbee2mqtt: text %s set_value value=%s", addr.Key(), c.Value)
		if txt, ok := entity.State.(domain.Text); ok {
			txt.Value = c.Value
			entity.State = txt
			p.store.Save(entity)
		}
	case domain.ClimateSetMode:
		log.Printf("plugin-zigbee2mqtt: climate %s set_mode mode=%s", addr.Key(), c.HVACMode)
		if climate, ok := entity.State.(domain.Climate); ok {
			climate.HVACMode = c.HVACMode
			entity.State = climate
			p.store.Save(entity)
		}
	case domain.ClimateSetTemperature:
		log.Printf("plugin-zigbee2mqtt: climate %s set_temperature temp=%v", addr.Key(), c.Temperature)
		if climate, ok := entity.State.(domain.Climate); ok {
			climate.Temperature = c.Temperature
			entity.State = climate
			p.store.Save(entity)
		}
	default:
		log.Printf("plugin-zigbee2mqtt: unknown command %T for %s", cmd, addr.Key())
	}

	log.Printf("plugin-zigbee2mqtt: [CMD] DONE entity=%s type=%T total=%s", addr.Key(), cmd, time.Since(recvAt).Round(time.Millisecond))
}

// ---------------------------------------------------------------------------
// Demo device — a sample device registered at startup
// ---------------------------------------------------------------------------

func (p *plugin) seedDemo() error {
	entities := []domain.Entity{
		{
			ID: "demo_light", Plugin: pluginID, DeviceID: "demo_device",
			Type: "light", Name: "Demo Light",
			Commands: []string{"light_turn_on", "light_turn_off", "light_set_brightness", "light_set_color_temp"},
			State:    domain.Light{Power: false, Brightness: 128},
		},
		{
			ID: "demo_switch", Plugin: pluginID, DeviceID: "demo_device",
			Type: "switch", Name: "Demo Switch",
			Commands: []string{"switch_turn_on", "switch_turn_off", "switch_toggle"},
			State:    domain.Switch{Power: false},
		},
		{
			ID: "demo_sensor", Plugin: pluginID, DeviceID: "demo_device",
			Type: "sensor", Name: "Demo Temperature",
			State: domain.Sensor{Value: 21.0, Unit: "°C"},
		},
	}
	for _, e := range entities {
		if err := p.store.Save(e); err != nil {
			return fmt.Errorf("save %s: %w", e.ID, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Entrypoint
// ---------------------------------------------------------------------------
