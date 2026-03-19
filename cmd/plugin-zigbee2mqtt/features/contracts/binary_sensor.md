# Binary Sensor — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/binary_sensor.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - binary_sensor:
      name: "Window Contact Sensor"
      state_topic: "bedroom/window/contact"
      payload_on: "ON"
      availability:
        - topic: "bedroom/window/availability"
          payload_available: "online"
          payload_not_available: "offline"
      qos: 0
      device_class: opening
      value_template: "{{ value_json.state }}"
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of state updates      |
| `payload_on` / `payload_off`| Maps to BinarySensor.On      |
| `device_class`                | BinarySensor.DeviceClass     |
| `availability.topic`          | Entity availability tracking |

## Supported Commands

Binary sensors are **read-only** entities and do not support commands.

| SlideBolt action | MQTT Topic | Payload |
|------------------|------------|---------|
| N/A              | N/A        | N/A     |

## Notes

- Z2M publishes the discovery payload to `homeassistant/binary_sensor/<device_id>/config`
- State messages are published to the configured `state_topic`
- `payload_on` and `payload_off` define the expected values (default: "ON"/"OFF")
- The `value_template` may extract nested JSON fields from the state message
- Common `device_class` values: `motion`, `door`, `window`, `smoke`, `moisture`, `opening`, `presence`, `light`, `sound`, `vibration`, `lock`, `plug`, `cold`, `heat`, `gas`, `power`, `problem`, `safety`, `update`, `running`, `moving`, `occupied`, `tamper`, `vibration`, `battery`, `battery_charging`, `carbon_monoxide`
- Availability topic can be used to track if the device is online/offline
