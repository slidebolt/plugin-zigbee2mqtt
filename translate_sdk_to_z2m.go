package main

import (
	"fmt"

	"github.com/slidebolt/sdk-entities/light"
	entityswitch "github.com/slidebolt/sdk-entities/switch"
)

func buildLightCommandPayload(lc light.Command, discovered discoveredEntity) (string, error) {
	switch lc.Type {
	case light.ActionTurnOn:
		return discovered.PayloadOn, nil
	case light.ActionTurnOff:
		return discovered.PayloadOff, nil
	case light.ActionSetRGB:
		if !discovered.SupportsRGB {
			return "", fmt.Errorf("action %s is not supported by this zigbee light", lc.Type)
		}
		if lc.RGB == nil || len(*lc.RGB) != 3 {
			return "", fmt.Errorf("invalid rgb payload")
		}
		rgb := *lc.RGB
		return fmt.Sprintf(`{"state":"ON","color":{"r":%v,"g":%v,"b":%v}}`, rgb[0], rgb[1], rgb[2]), nil
	case light.ActionSetBrightness:
		if !discovered.SupportsBrightness {
			return "", fmt.Errorf("action %s is not supported by this zigbee light", lc.Type)
		}
		if lc.Brightness == nil {
			return "", fmt.Errorf("invalid brightness payload")
		}
		if *lc.Brightness < 0 || *lc.Brightness > 100 {
			return "", fmt.Errorf("brightness must be between 0 and 100")
		}
		scaled := int(float64(*lc.Brightness) / 100.0 * 254.0)
		return fmt.Sprintf(`{"state":"ON","brightness":%d}`, scaled), nil
	case light.ActionSetTemperature:
		if !discovered.SupportsTemperature {
			return "", fmt.Errorf("action %s is not supported by this zigbee light", lc.Type)
		}
		if lc.Temperature == nil {
			return "", fmt.Errorf("invalid temperature payload")
		}
		mireds := 1000000 / *lc.Temperature
		return fmt.Sprintf(`{"state":"ON","color_temp":%d}`, mireds), nil
	default:
		return "", fmt.Errorf("unsupported light command type: %s", lc.Type)
	}
}

func buildSwitchCommandPayload(sc entityswitch.Command, discovered discoveredEntity) (string, error) {
	switch sc.Type {
	case entityswitch.ActionTurnOn:
		return discovered.PayloadOn, nil
	case entityswitch.ActionTurnOff:
		return discovered.PayloadOff, nil
	default:
		return "", fmt.Errorf("unsupported switch command type: %s", sc.Type)
	}
}
