package main

import (
	"strings"

	"github.com/slidebolt/sdk-entities/light"
	entityswitch "github.com/slidebolt/sdk-entities/switch"
)

func supportedActionsForDiscovered(ent discoveredEntity) []string {
	switch mapDomain(ent.EntityType) {
	case light.Type:
		actions := []string{light.ActionTurnOn, light.ActionTurnOff}
		if ent.SupportsBrightness {
			actions = append(actions, light.ActionSetBrightness)
		}
		if ent.SupportsRGB {
			actions = append(actions, light.ActionSetRGB)
		}
		if ent.SupportsTemperature {
			actions = append(actions, light.ActionSetTemperature)
		}
		return actions
	case entityswitch.Type:
		return entityswitch.SupportedActions()
	default:
		return nil
	}
}

func deriveLightCapabilities(data *haDiscoveryPayload) (supportsBrightness, supportsRGB, supportsTemperature bool) {
	if data == nil {
		return false, false, false
	}
	if data.Brightness {
		supportsBrightness = true
	}

	for _, mode := range data.SupportedColorModes {
		m := strings.ToLower(strings.TrimSpace(mode))
		switch m {
		case "brightness":
			supportsBrightness = true
		case "color_temp":
			supportsTemperature = true
			supportsBrightness = true
		case "rgb", "rgbw", "rgbww", "xy", "hs":
			supportsRGB = true
			supportsBrightness = true
		}
	}
	return supportsBrightness, supportsRGB, supportsTemperature
}
