# Lock — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/lock.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - lock:
      name: Frontdoor
      state_topic: "home-assistant/frontdoor/state"
      code_format: "^\\d{4}$"
      command_topic: "home-assistant/frontdoor/set"
      command_template: '{ "action": "{{ value }}", "code":"{{ code }}" }'
      payload_lock: "LOCK"
      payload_unlock: "UNLOCK"
      state_locked: "LOCK"
      state_unlocked: "UNLOCK"
      state_locking: "LOCKING"
      state_unlocking: "UNLOCKING"
      state_jammed: "MOTOR_JAMMED"
      state_ok: "MOTOR_OK"
      optimistic: false
      qos: 1
      retain: true
      value_template: "{{ value.x }}"
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of lock state         |
| `state_locked` / `state_unlocked` | Maps to Lock.Locked        |
| `state_locking` / `state_unlocking` | Transitional states      |
| `state_jammed`                | Error state                  |
| `code_format`                 | PIN code pattern             |
| `command_template`            | May require PIN code         |

## Supported Commands

| SlideBolt action | MQTT Topic        | Payload                                          |
|------------------|-------------------|--------------------------------------------------|
| `lock_lock`      | `command_topic`   | `payload_lock` (optionally with PIN via template) |
| `lock_unlock`    | `command_topic`   | `payload_unlock` (optionally with PIN)          |

## Notes

- Z2M publishes the discovery payload to `homeassistant/lock/<device_id>/config`
- Lock states: locked, unlocked, locking, unlocking, jammed
- `code_format` indicates if a PIN code is required (regex pattern)
- `command_template` may transform the command payload - watch for `{{ code }}` placeholders
- `optimistic: false` means wait for state_topic confirmation before updating
- `retain: true` ensures the lock state persists across MQTT broker restarts
- Some locks support additional states: jammed (mechanical failure), motor_ok/motor_jammed
- The `value_template` extracts the state from potentially nested JSON
- PIN code handling requires special attention in the command encoder
