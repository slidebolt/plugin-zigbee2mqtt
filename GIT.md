# Git Workflow for plugin-zigbee2mqtt

This repository contains the Slidebolt Zigbee2MQTT Plugin, providing integration with Zigbee devices via the Zigbee2MQTT bridge. It produces a standalone binary.

## Dependencies
- **Internal:**
  - `sb-contract`: Core interfaces.
  - `sb-domain`: Shared domain models.
  - `sb-messenger-sdk`: Shared messaging interfaces.
  - `sb-runtime`: Core execution environment.
  - `sb-storage-sdk`: Shared storage interfaces.
  - `sb-testkit`: Testing utilities.
- **External:** 
  - `github.com/eclipse/paho.mqtt.golang`: MQTT client implementation.
  - `github.com/cucumber/godog`: BDD testing framework.

## Build Process
- **Type:** Go Application (Plugin).
- **Consumption:** Run as a background plugin service.
- **Artifacts:** Produces a binary named `plugin-zigbee2mqtt`.
- **Command:** `go build -o plugin-zigbee2mqtt ./cmd/plugin-zigbee2mqtt`
- **Validation:** 
  - Validated through unit tests: `go test -v ./...`
  - Validated through BDD tests: `go test -v ./cmd/plugin-zigbee2mqtt`
  - Validated by successful compilation of the binary.

## Pre-requisites & Publishing
As a central Zigbee bridge plugin, `plugin-zigbee2mqtt` must be updated whenever the core domain, messaging, storage, or testkit SDKs are changed.

**Before publishing:**
1. Determine current tag: `git tag | sort -V | tail -n 1`
2. Ensure all local tests pass: `go test -v ./...`
3. Ensure the binary builds: `go build -o plugin-zigbee2mqtt ./cmd/plugin-zigbee2mqtt`

**Publishing Order:**
1. Ensure all internal dependencies are tagged and pushed.
2. Update `plugin-zigbee2mqtt/go.mod` to reference the latest tags.
3. Determine next semantic version for `plugin-zigbee2mqtt` (e.g., `v1.0.5`).
4. Commit and push the changes to `main`.
5. Tag the repository: `git tag v1.0.5`.
6. Push the tag: `git push origin main v1.0.5`.

## Update Workflow & Verification
1. **Modify:** Update Zigbee translation logic in `internal/translate/` or bridge logic in `app/`.
2. **Verify Local:**
   - Run `go mod tidy`.
   - Run `go test ./...`.
   - Run `go test ./cmd/plugin-zigbee2mqtt` (BDD features).
   - Run `go build -o plugin-zigbee2mqtt ./cmd/plugin-zigbee2mqtt`.
3. **Commit:** Ensure the commit message clearly describes the Zigbee integration changes.
4. **Tag & Push:** (Follow the Publishing Order above).
