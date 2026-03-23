# Cover — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/cover.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - cover:
      name: "MQTT Cover"
      command_topic: "living-room-cover/set"
      state_topic: "living-room-cover/state"
      position_topic: "living-room-cover/position"
      set_position_topic: "living-room-cover/position/set"
      tilt_command_topic: "living-room-cover/position/set" # same as `set_position_topic`
      qos: 1
      retain: false
      payload_open:  "open"
      payload_close: "close"
      payload_stop:  "stop"
      state_opening: "open"
      state_closing: "close"
      state_stopped: "stop"
      position_open: 100
      position_closed: 0
      tilt_min: 0
      tilt_max: 6
      tilt_opened_value: 3
      tilt_closed_value: 0
      optimistic: false
      position_template: |-
        {% if not state_attr(entity_id, "current_position") %}
          {
            "position" : {{ value }},
            "tilt_position" : 0
          }
        {% else %}
          {% set old_position = state_attr(entity_id, "current_position") %}
          {% set old_tilt_percent = (state_attr(entity_id, "current_tilt_position")) %}

          {% set movement = value | int - old_position %}
          {% set old_tilt_position = (old_tilt_percent / 100 * (tilt_max - tilt_min)) %}
          {% set new_tilt_position = min(max((old_tilt_position + movement), tilt_min), tilt_max) %}

          {
            "position": {{ value }},
            "tilt_position": {{ new_tilt_position }}
          }
        {% endif %}
      tilt_command_template: >-
        {% set position = state_attr(entity_id, "current_position") %}
        {% set tilt = state_attr(entity_id, "current_tilt_position") %}
        {% set movement = (tilt_position - tilt) / 100 * tilt_max %}
        {{ position + movement }}
      payload_open: "on"
      payload_close:
      payload_stop: "on"
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of state updates      |
| `position_topic`              | Source of position updates   |
| `position` (0-100)            | Cover.Position               |
| `tilt_position`               | Cover.TiltPosition           |
| `position_open` / `position_closed` | Position range mapping |
| `optimistic`                  | Controls immediate state update |

## Supported Commands

| SlideBolt action | MQTT Topic               | Payload                    |
|------------------|--------------------------|---------------------------|
| `cover_open`     | `command_topic`          | `payload_open`            |
| `cover_close`    | `command_topic`          | `payload_close`           |
| `cover_set_position` | `set_position_topic` | Position value (0-100)  |

## Notes

- Z2M publishes the discovery payload to `homeassistant/cover/<device_id>/config`
- Cover supports both state (open/close/stop) and position (0-100%) control
- `position_template` can transform incoming position values - requires Jinja2 template parsing
- `optimistic: true` means the state updates immediately without waiting for confirmation
- Tilt control (blinds) may use separate tilt topics or combined position/tilt
- Position values are typically 0-100 where 0 = closed, 100 = open
- Some devices may not support `set_position_topic` and only support open/close/stop
