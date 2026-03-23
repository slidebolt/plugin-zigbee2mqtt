# Button — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/button.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - button:
      unique_id: bedroom_switch_reboot_btn
      name: "Restart Bedroom Switch"
      command_topic: "home/bedroom/switch1/commands"
      payload_press: "restart"
      availability:
        - topic: "home/bedroom/switch1/available"
      qos: 0
      retain: false
      entity_category: "config"
      device_class: "restart"
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `unique_id`                   | Entity.ID (extracted)        |
| `command_topic`               | Command destination topic    |
| `payload_press`               | Button press payload         |
| `entity_category`               | Entity category (config/diagnostic) |
| `device_class`                | Button classification        |
| `availability.topic`          | Entity availability tracking |

## Supported Commands

| SlideBolt action | MQTT Topic        | Payload              |
|------------------|-------------------|----------------------|
| `button_press`   | `command_topic`   | `payload_press` value |

## Notes

- Z2M publishes the discovery payload to `homeassistant/button/<device_id>/config`
- Buttons are stateless - they only support the `press` command
- Common `device_class` values: `identify`, `restart`, `update`
- The `payload_press` defines what value to send when the button is pressed (default: "PRESS")
- `entity_category` can be "config" (configuration controls) or "diagnostic" (diagnostic info)
- Buttons typically do not have a `state_topic` as they are write-only
