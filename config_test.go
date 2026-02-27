package main

import "testing"

func TestLoadZ2MConfigFromEnvCanonical(t *testing.T) {
	t.Setenv("ZIGBEE2MQTT_MQTT_URL", "tcp://broker:1883")
	t.Setenv("ZIGBEE2MQTT_DISCOVERY_TOPIC", "homeassistant/light/#")
	t.Setenv("ZIGBEE2MQTT_BASE_TOPIC", "zigbee2mqtt")
	t.Setenv("ZIGBEE2MQTT_USERNAME", "user")
	t.Setenv("ZIGBEE2MQTT_PASSWORD", "pass")

	cfg := loadZ2MConfigFromEnv()
	if cfg.MQTTURL != "tcp://broker:1883" {
		t.Fatalf("unexpected MQTT URL: %q", cfg.MQTTURL)
	}
	if cfg.DiscoveryTopic != "homeassistant/light/#" {
		t.Fatalf("unexpected discovery topic: %q", cfg.DiscoveryTopic)
	}
	if cfg.BaseTopic != "zigbee2mqtt" {
		t.Fatalf("unexpected base topic: %q", cfg.BaseTopic)
	}
	if cfg.Username != "user" || cfg.Password != "pass" {
		t.Fatalf("unexpected credentials: %q/%q", cfg.Username, cfg.Password)
	}
}

func TestLoadZ2MConfigFromEnvLegacyFallback(t *testing.T) {
	t.Setenv("MQTT_URL", "tcp://legacy:1883")
	t.Setenv("MQTT_DISCOVERY_TOPIC", "homeassistant/#")
	t.Setenv("Z2M_BASE_TOPIC", "z2m")

	cfg := loadZ2MConfigFromEnv()
	if cfg.MQTTURL != "tcp://legacy:1883" {
		t.Fatalf("expected legacy MQTT_URL fallback, got %q", cfg.MQTTURL)
	}
	if cfg.DiscoveryTopic != "homeassistant/#" {
		t.Fatalf("expected legacy discovery fallback, got %q", cfg.DiscoveryTopic)
	}
	if cfg.BaseTopic != "z2m" {
		t.Fatalf("expected Z2M_BASE_TOPIC fallback, got %q", cfg.BaseTopic)
	}
}
