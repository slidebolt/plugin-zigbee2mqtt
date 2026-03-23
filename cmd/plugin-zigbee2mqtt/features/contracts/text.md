# Text — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/text.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - text:
      name: "Remote LCD screen"
      icon: mdi:ab-testing
      mode: "text"
      state_topic: "txt/state"
      command_topic: "txt/cmd"
      min: 2
      max: 20
      qos: 0
      retain: false
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of current text       |
| `command_topic`               | Destination for text updates |
| `value` (text string)         | Text.Value                   |
| `min`                         | Text.Min (min length)        |
| `max`                         | Text.Max (max length)        |
| `pattern`                     | Text.Pattern (regex)         |
| `mode`                        | Text.Mode (text/password)    |

## Supported Commands

| SlideBolt action    | MQTT Topic        | Payload                  |
|---------------------|-------------------|--------------------------|
| `text_set_value`    | `command_topic`   | Text string value        |

## Notes

- Z2M publishes the discovery payload to `homeassistant/text/<device_id>/config`
- Text entities allow setting arbitrary text values
- `mode` can be "text" (visible) or "password" (masked)
- `min` and `max` define the allowed string length range
- `pattern` is a regex pattern that valid input must match
- Commands send the raw text string to the `command_topic`
- State is read from `state_topic` and may use `value_template` for extraction
- Useful for device displays, configuration strings, or any text-based control
- The `icon` field suggests a UI icon but doesn't affect functionality
