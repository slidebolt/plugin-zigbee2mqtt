# TASK: plugin-zigbee2mqtt

## Status: Discovery Works — Command and Event Paths are Stubs

## Issues

### 1. OnCommand Does Nothing (High)
`OnCommand` returns the entity unchanged with no error. Sending any command to a
Zigbee2MQTT device silently "succeeds" but publishes nothing to MQTT. The device
receives no instruction.

- [ ] Implement MQTT command publishing in `OnCommand`
- [ ] Look up the entity's `CommandTopic` and `PayloadOn`/`PayloadOff` from the persisted `discovered` map (stored in entity `Config.Data`)
- [ ] Map SDK domain commands (light on/off, switch on/off, etc.) to the correct MQTT topic and payload

### 2. OnEvent Does Not Update Entity State (High)
`OnEvent` is a pass-through. When the runner delivers an inbound event (e.g. a zigbee device
reporting its state), the entity's reported state is never updated.

- [ ] Implement `OnEvent` to parse the event payload and update `entity.Data.Reported` / call the appropriate `sdk-entities` store helper

### 3. Silent Domain Misclassification (Low)
`mapDomain` defaults any unrecognized `entity_type` to `"sensor"` without logging. Novel or
unexpected Zigbee device types will be silently classified as sensors, which may cause incorrect
command routing later.

- [ ] Add a log warning when an unrecognized entity type is encountered and defaulted
