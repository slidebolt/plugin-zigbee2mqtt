package main

import (
	"os"
	"strings"
)

type z2mConfig struct {
	MQTTURL        string
	DiscoveryTopic string
	BaseTopic      string
	Username       string
	Password       string
}

func loadZ2MConfigFromEnv() z2mConfig {
	cfg := z2mConfig{
		MQTTURL: strings.TrimSpace(firstNonEmpty(
			"ZIGBEE2MQTT_MQTT_URL",
			"Z2M_MQTT_BROKER_URL",
			"MQTT_URL",
		)),
		DiscoveryTopic: strings.TrimSpace(firstNonEmpty(
			"ZIGBEE2MQTT_DISCOVERY_TOPIC",
			"Z2M_DISCOVERY_TOPIC",
			"MQTT_DISCOVERY_TOPIC",
		)),
		BaseTopic: strings.TrimSpace(firstNonEmpty(
			"ZIGBEE2MQTT_BASE_TOPIC",
			"Z2M_BASE_TOPIC",
		)),
		Username: firstNonEmpty(
			"ZIGBEE2MQTT_USERNAME",
			"Z2M_USERNAME",
		),
		Password: firstNonEmpty(
			"ZIGBEE2MQTT_PASSWORD",
			"Z2M_PASSWORD",
		),
	}
	if cfg.DiscoveryTopic == "" {
		cfg.DiscoveryTopic = "slidebolt/discovery/#"
	}
	if cfg.BaseTopic == "" {
		cfg.BaseTopic = "zigbee2mqtt"
	}
	return cfg
}

func firstNonEmpty(keys ...string) string {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return os.Getenv(key)
		}
	}
	return ""
}
