package main

import (
	"fmt"
	"github.com/slidebolt/plugin-framework"
	 "github.com/slidebolt/plugin-zigbee2mqtt/pkg/bundle"
	"github.com/slidebolt/plugin-sdk"
)

func main() {
	fmt.Println("Starting MQTT Plugin Sidecar...")
	
	// Initialize framework registry/connectivity
	framework.Init()

	b, err := sdk.RegisterBundle("plugin-mqtt")
	if err != nil {
		fmt.Printf("Failed to register bundle: %v\n", err)
		return
	}

	p := bundle.NewPlugin()
	if err := p.Init(b); err != nil {
		fmt.Printf("Failed to init plugin: %v\n", err)
		return
	}

	fmt.Println("MQTT Plugin is running.")
	// Keep alive
	select {}
}