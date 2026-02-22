package logic

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"time"
)

type MQTTClient interface {
	Connect() error
	Subscribe(topic string, callback func(topic string, payload []byte)) error
	Publish(topic string, payload interface{}) error
	Disconnect()
}

type RealMQTTClient struct {
	client mqtt.Client
}

func NewRealMQTTClient(url string) *RealMQTTClient {
	opts := mqtt.NewClientOptions().AddBroker(url)
	opts.SetClientID(fmt.Sprintf("plugin-mqtt-%d", time.Now().UnixNano()))
	opts.SetAutoReconnect(true)
	return &RealMQTTClient{
		client: mqtt.NewClient(opts),
	}
}

func (c *RealMQTTClient) Connect() error {
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *RealMQTTClient) Subscribe(topic string, callback func(topic string, payload []byte)) error {
	token := c.client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		callback(msg.Topic(), msg.Payload())
	})
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *RealMQTTClient) Publish(topic string, payload interface{}) error {
	var data []byte
	switch v := payload.(type) {
	case string: data = []byte(v)
	case []byte: data = v
	default: return fmt.Errorf("unsupported payload type")
	}
	
	token := c.client.Publish(topic, 0, false, data)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *RealMQTTClient) Disconnect() {
	c.client.Disconnect(250)
}
