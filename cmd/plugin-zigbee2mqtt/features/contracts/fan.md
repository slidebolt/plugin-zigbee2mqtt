# Fan — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/fan.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example using percentage based speeds with preset modes configuration.yaml
mqtt:
  - fan:
      name: "Bedroom Fan"
      state_topic: "bedroom_fan/on/state"
      command_topic: "bedroom_fan/on/set"
      direction_state_topic: "bedroom_fan/direction/state"
      direction_command_topic: "bedroom_fan/direction/set"
      oscillation_state_topic: "bedroom_fan/oscillation/state"
      oscillation_command_topic: "bedroom_fan/oscillation/set"
      percentage_state_topic: "bedroom_fan/speed/percentage_state"
      percentage_command_topic: "bedroom_fan/speed/percentage"
      preset_mode_state_topic: "bedroom_fan/preset/preset_mode_state"
      preset_mode_command_topic: "bedroom_fan/preset/preset_mode"
      preset_modes:
        -  "auto"
        -  "smart"
        -  "whoosh"
        -  "eco"
        -  "breeze"
      qos: 0
      payload_on: "true"
      payload_off: "false"
      payload_oscillation_on: "true"
      payload_oscillation_off: "false"
      speed_range_min: 1
      speed_range_max: 10
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of on/off state       |
| `percentage_state_topic`    | Source of speed percentage   |
| `power` (on/off)              | Fan.Power                    |
| `percentage` (0-100)          | Fan.Percentage               |
| `preset_mode_state_topic`     | Current preset mode          |
| `preset_mode`                 | Fan.PresetMode               |
| `speed_range_min` / `speed_range_max` | Speed range mapping |

## Supported Commands

| SlideBolt action      | MQTT Topic                      | Payload                              |
|-----------------------|----------------------------------|--------------------------------------|
| `fan_turn_on`         | `command_topic`                  | `payload_on`                         |
| `fan_turn_off`        | `command_topic`                  | `payload_off`                        |
| `fan_set_speed`       | `percentage_command_topic`       | Percentage value (0-100)            |
| `select_option`       | `preset_mode_command_topic`      | Preset mode name (e.g., "auto")      |

## Notes

- Z2M publishes the discovery payload to `homeassistant/fan/<device_id>/config`
- Fans can support multiple control modes:
  - On/off only (basic fans)
  - Percentage speed (0-100%) - requires `percentage_command_topic`
  - Preset modes (auto, eco, sleep, etc.) - requires `preset_mode_command_topic`
- `speed_range_min` and `speed_range_max` define the native device speed range
- Direction and oscillation are separate features not currently mapped to SlideBolt domain
- Some fans support only preset modes without percentage control
- Speed values in SlideBolt are always 0-100, mapping to device-specific ranges as needed
