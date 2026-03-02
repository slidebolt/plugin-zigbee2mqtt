### `plugin-zigbee2mqtt` repository

#### Project Overview

This repository contains the `plugin-zigbee2mqtt`, a plugin that integrates the Slidebolt system with [Zigbee2MQTT](https://www.zigbee2mqtt.io/). It allows Slidebolt to discover, monitor, and control Zigbee devices that are managed by a Zigbee2MQTT instance.

#### Architecture

The `plugin-zigbee2mqtt` works by connecting to an MQTT broker and leveraging the Home Assistant discovery protocol, which is a feature of Zigbee2MQTT.

-   **MQTT Connection**: The plugin establishes a connection to an MQTT broker using the credentials and address provided in its configuration.

-   **Home Assistant Discovery**: It subscribes to the Home Assistant discovery topic (e.g., `homeassistant/#`). When a Zigbee device is paired with Zigbee2MQTT, it publishes a discovery message on this topic. The plugin captures and parses this message to learn about the new device and its capabilities (e.g., if it's a light, switch, sensor, etc.).

-   **Device and Entity Creation**: Based on the discovery information, the plugin creates a corresponding device and its associated entities within the Slidebolt system.

-   **State Synchronization and Control**:
    -   The plugin subscribes to the specific state topic for each discovered entity to receive real-time updates from the Zigbee device. These updates are then emitted as events in the Slidebolt system.
    -   When a command is issued to an entity from within Slidebolt, the plugin publishes a message to the appropriate MQTT command topic to control the physical Zigbee device.

#### Key Files

| File | Description |
| :--- | :--- |
| `main.go` | The main entry point that initializes and runs the plugin. |
| `plugin.go` | The core plugin logic, responsible for managing the MQTT connection, processing discovery messages, and handling the lifecycle of devices and entities. |
| `mqtt_client.go` | A wrapper around the `paho.mqtt.golang` library that simplifies MQTT operations. |
| `discovery.go` | Contains the data structures and logic for parsing the Home Assistant MQTT discovery payloads published by Zigbee2MQTT. |
| `config.go` | Handles loading the MQTT broker configuration from environment variables. |
| `.env.example`| Specifies the environment variables required to connect to the MQTT broker, including the URL, discovery topic, and credentials. |

#### Available Commands

The plugin can send commands to control Zigbee devices by publishing to their MQTT command topics. The supported commands depend on the device type but typically include:

-   **Switches**: `turn_on`, `turn_off`
-   **Lights**: `turn_on`, `turn_off`, `set_brightness`, `set_rgb`, `set_temperature`, etc.
