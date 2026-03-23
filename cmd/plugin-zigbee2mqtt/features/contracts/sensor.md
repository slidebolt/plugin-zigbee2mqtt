# Sensor — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/sensor.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  sensor:
    - name: "Timer 1"
      state_topic: "tele/sonoff/sensor"
      value_template: "{{ value_json.Timer1.Arm }}"
      json_attributes_topic: "tele/sonoff/sensor"
      json_attributes_template: "{{ value_json.Timer1 | tojson }}"
      unit_of_measurement: ""
      device_class: ""

    - name: "Temperature"
      state_topic: "zigbee2mqtt/living_room_temp"
      value_template: "{{ value_json.temperature }}"
      unit_of_measurement: "°C"
      device_class: "temperature"
      qos: 0
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of value updates      |
| `value_template`              | Extraction from JSON payload |
| extracted value               | Sensor.Value                 |
| `unit_of_measurement`           | Sensor.Unit                  |
| `device_class`                | Sensor.DeviceClass           |
| `json_attributes_topic`         | Additional attributes        |

## Supported Commands

Sensors are **read-only** entities and do not support commands.

| SlideBolt action | MQTT Topic | Payload |
|------------------|------------|---------|
| N/A              | N/A        | N/A     |

## Notes

- Z2M publishes the discovery payload to `homeassistant/sensor/<device_id>/config`
- State messages contain the current value, typically as JSON
- `value_template` is crucial - sensors often report multiple values in one JSON payload
- Common `device_class` values: `temperature`, `humidity`, `pressure`, `illuminance`, `energy`, `power`, `voltage`, `current`, `battery`, `timestamp`, `monetary`, `distance`, `volume`, `weight`, `duration`, `data_rate`, `concentration`, `aqi`, `carbon_dioxide`, `carbon_monoxide`, `gas`, `nitrogen_dioxide`, `nitrogen_monoxide`, `ozone`, `pm1`, `pm10`, `pm25`, `sulphur_dioxide`, `volatile_organic_compounds`
- `json_attributes_topic` can carry additional metadata beyond the main value
- Values may be numeric or string depending on the sensor type
- For numeric sensors, pay attention to `unit_of_measurement` for proper display
- Some sensors report only when value changes, others periodically
