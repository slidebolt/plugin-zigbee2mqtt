# Switch — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/switch.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - switch:
      unique_id: bedroom_switch
      name: "Bedroom Switch"
      state_topic: "home/bedroom/switch1"
      command_topic: "home/bedroom/switch1/set"
      availability:
        - topic: "home/bedroom/switch1/available"
      payload_on: "ON"
      payload_off: "OFF"
      state_on: "ON"
      state_off: "OFF"
      optimistic: false
      qos: 0
      retain: true
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `unique_id`                   | Entity.ID (extracted)        |
| `state_topic`                 | Source of on/off state       |
| `command_topic`               | Destination for commands     |
| `payload_on` / `payload_off`| Maps to Switch.Power         |
| `state_on` / `state_off`      | Expected state values        |
| `availability.topic`          | Entity availability tracking |
| `optimistic`                  | Controls immediate state update |

## Supported Commands

| SlideBolt action    | MQTT Topic        | Payload                  |
|---------------------|-------------------|--------------------------|
| `switch_turn_on`    | `command_topic`   | `payload_on`             |
| `switch_turn_off`   | `command_topic`   | `payload_off`            |
| `switch_toggle`     | `command_topic`   | May toggle locally       |

## Notes

- Z2M publishes the discovery payload to `homeassistant/switch/<device_id>/config`
- Switches are simple on/off controls
- `payload_on` and `payload_off` define the command values (default: "ON"/"OFF")
- `state_on` and `state_off` define the expected state values from state_topic
- `optimistic: true` updates state immediately without waiting for confirmation
- `retain: true` ensures the last command persists across MQTT broker restarts
- Availability topic tracks if the switch is online/offline
- Toggle command may be implemented by reading current state and sending opposite, or device may support a TOGGLE payload
