package main

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type mqttClient interface {
	Connect() error
	Subscribe(topic string, callback func(topic string, payload []byte)) error
	Publish(topic string, payload interface{}) error
	Disconnect()
}

type realMQTTClient struct {
	client mqtt.Client
}

func newRealMQTTClient(cfg z2mConfig) *realMQTTClient {
	opts := mqtt.NewClientOptions().AddBroker(cfg.MQTTURL)
	opts.SetClientID(fmt.Sprintf("plugin-zigbee2mqtt-%d", time.Now().UnixNano()))
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true) // Start fresh to get all retained messages immediately
	opts.SetMessageChannelDepth(1000) // Increase buffer for initial burst of retained messages
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}
	return &realMQTTClient{client: mqtt.NewClient(opts)}
}

func (c *realMQTTClient) Connect() error {
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	time.Sleep(1 * time.Second) // Give broker time to prepare initial burst
	return nil
}

func (c *realMQTTClient) Subscribe(topic string, callback func(topic string, payload []byte)) error {
	token := c.client.Subscribe(topic, 0, func(_ mqtt.Client, msg mqtt.Message) {
		callback(msg.Topic(), msg.Payload())
	})
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *realMQTTClient) Publish(topic string, payload interface{}) error {
	var data []byte
	switch v := payload.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("unsupported payload type")
	}

	token := c.client.Publish(topic, 0, false, data)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *realMQTTClient) Disconnect() {
	c.client.Disconnect(250)
}
