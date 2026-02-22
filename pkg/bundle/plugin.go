package bundle

import (
	"os"

	"github.com/slidebolt/plugin-zigbee2mqtt/pkg/device"
	"github.com/slidebolt/plugin-zigbee2mqtt/pkg/logic"
	"github.com/slidebolt/plugin-sdk"
)

type MQTTPlugin struct {
	Bundle  sdk.Bundle
	Client  logic.MQTTClient
	Adapter *device.MQTTAdapter
}

func (p *MQTTPlugin) Init(b sdk.Bundle) error {
	p.Bundle = b
	b.UpdateMetadata("MQTT Bridge")

	b.OnConfigure(func() {
		p.start()
	})

	p.start()
	return nil
}

func (p *MQTTPlugin) Shutdown() {
	if p.Client != nil {
		p.Client.Disconnect()
	}
	if p.Adapter != nil {
		p.Adapter.Wait()
	}
}

func (p *MQTTPlugin) start() {
	raw := p.Bundle.Raw()

	// Try Raw Data first, fallback to ENV
	url, _ := raw["mqtt_url"].(string)
	if url == "" {
		url = os.Getenv("MQTT_URL")
	}

	if url == "" {
		p.Bundle.Log().Error("MQTT_URL missing in Config or ENV")
		return
	}

	// Disconnect any existing client before reconnecting
	if p.Client != nil {
		p.Client.Disconnect()
	}

	p.Client = logic.NewRealMQTTClient(url)
	if err := p.Client.Connect(); err != nil {
		p.Bundle.Log().Error("MQTT Connection Failed: %v", err)
		return
	}

	p.Adapter = device.NewMQTTAdapter(p.Bundle, p.Client)
	for _, dev := range p.Bundle.GetDevices() {
		ents, err := dev.GetEntities()
		if err != nil {
			continue
		}
		for _, ent := range ents {
			p.Adapter.WireExistingEntity(ent)
		}
	}

	// Subscribe to Discovery Topic (configurable)
	discoveryTopic, _ := raw["discovery_topic"].(string)
	if discoveryTopic == "" {
		discoveryTopic = os.Getenv("MQTT_DISCOVERY_TOPIC")
	}
	if discoveryTopic == "" {
		discoveryTopic = "homeassistant/#"
	}

	p.Client.Subscribe(discoveryTopic, func(topic string, payload []byte) {
		p.Adapter.HandleDiscovery(topic, payload)
	})

	p.Bundle.Log().Info("MQTT Plugin Connected to %s", url)
}

func NewPlugin() *MQTTPlugin { return &MQTTPlugin{} }
