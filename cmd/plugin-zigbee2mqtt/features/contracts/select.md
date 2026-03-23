# Select — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/select.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - select:
      name: "Test Select"
      state_topic: "topic/state"
      command_topic: "topic/set"
      options:
        - "Option 1"
        - "Option 2"
        - "Option 3"
      qos: 0
      retain: false
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of current option     |
| `command_topic`               | Destination for set commands |
| `options`                     | Select.Options (list)        |
| `current option value`        | Select.Option                |

## Supported Commands

| SlideBolt action    | MQTT Topic        | Payload                    |
|---------------------|-------------------|----------------------------|
| `select_option`     | `command_topic`   | Selected option string     |

## Notes

- Z2M publishes the discovery payload to `homeassistant/select/<device_id>/config`
- Select provides a dropdown of predefined options
- The `options` array defines all valid choices
- Commands must send one of the exact strings from the `options` list
- The current selection is read from `state_topic` and maps to `Select.Option`
- Some devices may use `value_template` to extract the option from nested JSON
- Options are typically static and defined at discovery time
- The order of options in the discovery payload should be preserved for UI display
