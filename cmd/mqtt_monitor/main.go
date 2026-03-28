package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	broker := os.Getenv("TEST_MQTT_BROKER")
	if broker == "" {
		log.Fatal("TEST_MQTT_BROKER is required")
	}

	fmt.Printf("Connecting to MQTT broker: %s\n", broker)

	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("monitor-" + fmt.Sprintf("%d", time.Now().Unix())).
		SetConnectTimeout(5 * time.Second).
		SetAutoReconnect(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(10 * time.Second)

	if token.Error() != nil {
		log.Fatalf("Failed to connect: %v", token.Error())
	}

	fmt.Println("✓ Connected to MQTT broker")
	fmt.Println("")
	fmt.Println("MONITORING ALL zigbee2mqtt/# TOPICS")
	fmt.Println("====================================")
	fmt.Println("")
	fmt.Println("Send your MQTT messages now!")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("")

	// Subscribe to all zigbee2mqtt topics
	token = client.Subscribe("zigbee2mqtt/#", 0, func(c mqtt.Client, msg mqtt.Message) {
		var prettyJSON map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &prettyJSON); err == nil {
			jsonBytes, _ := json.MarshalIndent(prettyJSON, "", "  ")
			fmt.Printf("\n[%s] %s\n%s\n", time.Now().Format("15:04:05"), msg.Topic(), string(jsonBytes))
		} else {
			fmt.Printf("\n[%s] %s\n%s\n", time.Now().Format("15:04:05"), msg.Topic(), string(msg.Payload()))
		}
	})

	token.WaitTimeout(5 * time.Second)
	if token.Error() != nil {
		log.Fatalf("Failed to subscribe: %v", token.Error())
	}

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	client.Disconnect(250)
}
