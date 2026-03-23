# Light — Protocol Contract

> **Plugin authors:** Replace the "External Protocol" section below with the
> raw protocol binding for your plugin. The feature file at
> `features/light.feature` tests the SlideBolt side; this document
> explains the external side.

## External Protocol

```yaml
# Example configuration.yaml entry
mqtt:
  - light:
      schema: template
      name: "Bulb-white"
      command_topic: "shellies/bulb/color/0/set"
      state_topic: "shellies/bulb/color/0/status"
      availability_topic: "shellies/bulb/online"
      command_on_template: >
        {"turn": "on", "mode": "white"
        {%- if brightness is defined -%}
        , "brightness": {{brightness | float | multiply(0.39215686) | round(0)}}
        {%- endif -%}
        {%- if color_temp is defined -%}
        , "temp": {{ [[(1000000 / color_temp | float) | round(0), 3000] | max, 6500] | min }}
        {%- endif -%}
        }
      command_off_template: '{"turn":"off", "mode": "white"}'
      state_template: "{% if value_json.ison and value_json.mode == 'white' %}on{% else %}off{% endif %}"
      brightness_template: "{{ value_json.brightness | float | multiply(2.55) | round(0) }}"
      color_temp_template: "{{ (1000000 / value_json.temp | float) | round(0) }}"
      payload_available: "true"
      payload_not_available: "false"
      max_mireds: 334
      min_mireds: 153
      qos: 1
      retain: false
      optimistic: false
```

## SlideBolt Domain Mapping

| External field                | SlideBolt field              |
|-------------------------------|------------------------------|
| `name`                        | Entity.Name                  |
| `state_topic`                 | Source of on/off state       |
| `brightness` (0-255 or 0-100) | Light.Brightness             |
| `brightness_scale`            | Scale factor for brightness  |
| `color_temp` (mireds)         | Light.Temperature            |
| `rgb` / `r` `g` `b`           | Light color (if supported)   |
| `effect`                      | Light.Effect                 |
| `max_mireds` / `min_mireds`   | Color temperature range      |

## Supported Commands

| SlideBolt action           | MQTT Topic                        | Payload / Notes                                         |
|---------------------------|-----------------------------------|--------------------------------------------------------|
| `light_turn_on`           | `command_topic`                   | JSON with `"state": "ON"`                              |
| `light_turn_off`          | `command_topic`                   | JSON with `"state": "OFF"`                             |
| `light_set_brightness`    | `command_topic`                   | Scale to `brightness_scale` (typically 0-254)           |
| `light_set_color_temp`    | `command_topic`                   | Mireds value (153-500 typical range)                    |
| `light_set_rgb`           | `command_topic`                   | RGB values 0-255 each                                 |
| `light_set_rgbw`          | `command_topic`                   | RGBW values 0-255 each                                  |
| `light_set_rgbww`         | `command_topic`                   | RGBCCT values 0-255 each                                |
| `light_set_hs`            | `command_topic`                   | Hue (0-360), Saturation (0-100)                       |
| `light_set_xy`            | `command_topic`                   | x,y chromaticity coordinates (0-1)                    |
| `light_set_white`         | `command_topic`                   | White channel value                                     |
| `light_set_effect`        | `command_topic`                   | Effect name string                                      |

## Notes

- Z2M publishes the discovery payload to `homeassistant/light/<device_id>/config`
- Light features vary significantly by device:
  - Simple on/off only
  - Dimmable (brightness only)
  - White spectrum (brightness + color temp)
  - RGB/RGBW/RGBWW (full color control)
- Color temperature is in **mireds** (reciprocal megakelvin), lower = warmer (redder), higher = cooler (bluer)
- Common mireds range: 153 (6500K cool white) to 500 (2000K warm white)
- `brightness_scale` is critical: Z2M uses 0-254, but some devices use 0-255 or 0-100
- `command_on_template` and `command_off_template` may contain Jinja2 templates that need transformation
- `effect_list` in discovery shows supported effects (if any)
- Some lights report state immediately (`optimistic: true`), others wait for confirmation
- Availability topic tracks if the light is reachable on the network
