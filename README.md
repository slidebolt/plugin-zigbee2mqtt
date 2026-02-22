# Zigbee2MQTT Plugin for Slidebolt

The Zigbee2MQTT Plugin provides a generic bridge between MQTT-enabled devices (like Zigbee2MQTT, Tasmota, or custom ESP chips) and the Slidebolt Framework.

## Features

- **Generic Bridge**: Connects any MQTT device to Slidebolt via simple topic mapping.
- **Zigbee2MQTT Ready**: Pre-configured support for common Zigbee2MQTT lighting and sensor payloads.
- **Isolated Service**: Runs as a standalone sidecar service communicating via NATS.

## Architecture

This plugin follows the Slidebolt "Isolated Service" pattern:
- **`pkg/bundle`**: Implementation of the `sdk.Plugin` interface.
- **`pkg/logic`**: MQTT client logic and protocol handling.
- **`cmd/main.go`**: Service entry point.

## Development

### Prerequisites
- Go (v1.25.6+)
- Slidebolt `plugin-sdk` and `plugin-framework` repos sitting as siblings.

### Local Build
Initialize the Go workspace to link sibling dependencies:
```bash
go work init . ../plugin-sdk ../plugin-framework
go build -o bin/plugin-zigbee2mqtt ./cmd/main.go
```

### Testing
```bash
go test ./...
```

## Docker Deployment

### Build the Image
To build with local sibling modules:
```bash
make docker-build-local
```

To build from remote GitHub repositories:
```bash
make docker-build-prod
```

### Run via Docker Compose
Add the following to your `docker-compose.yml`:
```yaml
services:
  mqtt-plugin:
    image: slidebolt-plugin-zigbee2mqtt:latest
    environment:
      - NATS_URL=nats://core:4222
      - MQTT_BROKER=tcp://your-broker:1883
    restart: always
```

## License
Refer to the root project license.
