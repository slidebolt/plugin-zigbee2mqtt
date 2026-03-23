# Number — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/number.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - number:
      name: "Threshold"
      state_topic: "my-device/threshold/state"
      command_topic: "my-device/threshold/set"
      min: 0
      max: 100
      step: 1
      unit_of_measurement: "%"
      qos: 1
      retain: false
      optimistic: false
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of current value      |
| `command_topic`               | Destination for set commands |
| `min`                         | Number.Min                   |
| `max`                         | Number.Max                   |
| `step`                        | Number.Step                  |
| `unit_of_measurement`         | Number.Unit                  |
| `mode`                        | Input mode (box/slider)      |

## Supported Commands

| SlideBolt action    | MQTT Topic        | Payload                         |
|---------------------|-------------------|---------------------------------|
| `number_set_value`  | `command_topic`   | Numeric value (respects min/max) |

## Notes

- Z2M publishes the discovery payload to `homeassistant/number/<device_id>/config`
- Number entities allow setting a numeric value within a defined range
- The value must be validated against `min` and `max` before sending
- `step` defines the increment/decrement granularity (e.g., 0.5 for half steps)
- `mode` can be "box" (text input) or "slider" (visual slider)
- Values may be integers or floats depending on `step` and precision needs
- The `command_topic` receives the raw numeric value (as string or number JSON)
- Some devices may use `value_template` to transform incoming state values
