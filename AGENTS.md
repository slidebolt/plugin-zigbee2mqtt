# plugin-zigbee2mqtt Instructions

`plugin-zigbee2mqtt` follows the reference runnable-module architecture.

- Keep `cmd/plugin-zigbee2mqtt/main.go` as a thin wrapper only.
- Put runtime lifecycle and device wiring in `app/`.
- Keep protocol/private helpers under `internal/...`.
- Prefer testing `app/` and `internal/...`.
- Temporary cmd compatibility shims must carry `BUGFIX` notes and be removed once legacy tests stop depending on `package main`.
