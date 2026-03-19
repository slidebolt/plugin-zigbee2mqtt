# Climate — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/climate.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Full example configuration.yaml entry
mqtt:
  - climate:
      name: Study
      modes:
        - "off"
        - "cool"
        - "fan_only"
      swing_horizontal_modes:
        - "on"
        - "off"
      swing_modes:
        - "on"
        - "off"
      fan_modes:
        - "high"
        - "medium"
        - "low"
      preset_modes:
        - "eco"
        - "sleep"
        - "activity"
      power_command_topic: "study/ac/power/set"
      preset_mode_command_topic: "study/ac/preset_mode/set"
      mode_command_topic: "study/ac/mode/set"
      mode_command_template: "{{ value if value==\"off\" else \"on\" }}"
      temperature_command_topic: "study/ac/temperature/set"
      fan_mode_command_topic: "study/ac/fan/set"
      swing_horizontal_mode_command_topic: "study/ac/swingH/set"
      swing_mode_command_topic: "study/ac/swing/set"
      precision: 1.0
```

## SlideBolt Domain Mapping

| External field                        | SlideBolt field                    |
|---------------------------------------|------------------------------------|
| `name`                                | Entity.Name                        |
| `modes` (current)                     | Climate.HVACMode                   |
| `temperature` (current)               | Climate.Temperature                |
| `temperature_unit`                      | Climate.TemperatureUnit            |
| `mode_command_topic` payload          | ClimateSetMode.HVACMode            |
| `temperature_command_topic` payload   | ClimateSetTemperature.Temperature  |
| `fan_mode_command_topic`              | Fan command (if mapped)            |
| `preset_mode_command_topic`           | Select command (if mapped)         |
| `power_command_topic`                 | Power command (if device supports) |

## Supported Commands

| SlideBolt action              | MQTT Topic                         | Payload Example       |
|-------------------------------|------------------------------------|----------------------|
| `climate_set_mode`            | `mode_command_topic`               | `"cool"` or `"off"`    |
| `climate_set_temperature`     | `temperature_command_topic`        | `22` (integer/float)   |
| `fan_set_speed`               | `fan_mode_command_topic` (if used) | `"high"`, `"medium"`, `"low"` |
| `select_option` (preset)      | `preset_mode_command_topic`        | `"eco"`, `"sleep"`   |

## Notes

- Z2M publishes the discovery payload to `homeassistant/climate/<device_id>/config`
- Current state is read from the `state_topic` (not shown in config, but typically `zigbee2mqtt/<friendly_name>`)
- Temperature values should respect the `precision` setting (1.0 = whole numbers)
- The `mode_command_template` may transform values before publishing - this needs to be handled in encoding
- Not all Z2M climate devices support all features - check the discovery payload for available modes
- Some devices use `fan_mode_command_topic` for speed control, others may map to select for preset modes
