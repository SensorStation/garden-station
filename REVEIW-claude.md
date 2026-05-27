# Gardener — Code Review

*Reviewed by Claude Sonnet 4.6 on 2026-05-27*

---

## Project Overview

Gardener is a Go IoT application for automated plant watering on Raspberry Pi hardware. It reads a soil moisture sensor (VH400) and an environmental sensor (BME280), drives a water pump relay, responds to physical buttons, and exposes a web UI. Messaging between components uses MQTT via a local `otto` framework. A mock mode allows development without physical hardware.

---

## Architecture

| File | Role |
|---|---|
| `main.go` | Entry point: CLI flags, logger init, signal handling, graceful shutdown |
| `gardener.go` | Core `Gardener` struct: device initialization, pub/sub wiring, emulator |
| `app.go` | Embeds the `app/` directory and registers it with the HTTP server |
| `controller.go` | Declared but empty — only `package main` |
| `doc.go` | Package-level documentation describing the full intended architecture |
| `app/js/gardener.js` | Browser-side MQTT over WebSocket UI client |

The design embeds `*messenger.Messenger`, `*station.StationManager`, `*server.Server`, and `*station.DeviceManager` into a single `Gardener` struct. This is clean and keeps wiring minimal. The separation of hardware concerns (`devices` package) from messaging concerns (`otto/messenger`) is a good pattern for testability.

---

## Bugs and Correctness Issues

### 1. `controller.go` is empty (`controller.go:1`)
The file declares `package main` and nothing else. `doc.go` describes a full controller that should subscribe to sensor data, compare against wet/dry thresholds, and command the pump. This logic does not exist anywhere in the codebase.

### 2. `InitApp()` is never called (`app.go:10`)
`InitApp()` registers the embedded web UI with the HTTP server, but neither `Init()` nor `Start()` in `gardener.go` calls it. The web interface described in the README will not be served.

### 3. Inconsistent MQTT topic formats
Three different topic naming conventions appear in the same project:

- `gardener.go` publishes to bare `"d/soil"`, `"d/env"`, `"d/off"` (lines 90, 109, 138)
- `gardener.go:173` subscribes to plain `"soil"`, `"env"`, `"on"`, `"off"`, `"pump"`, `"display"` with no namespace
- `gardener.js` subscribes to `"ss/d/+/soil"`, `"ss/d/+/env"`, `"ss/c/+/pump"` with a full `ss/` prefix

Messages published by the Go code will never be received by the browser client, and vice versa.

### 4. `on` button and `off` button use different topic helpers (`gardener.go:77,90`)
```go
// on button:
g.Messenger.Pub(messenger.DataTopic("on"), "on")
// off button:
g.Messenger.Pub("d/off", []byte("off"))
```
One uses a helper function, the other hard-codes the prefix and passes a different payload type. Likely a copy-paste inconsistency.

### 5. `config.json` is not valid JSON (`config.json`)
Object values are missing the `:` separator after the key (e.g., `"soil" {` instead of `"soil": {`) and some fields use commas where colons are required (e.g., `"type", "gpio"`). This file cannot be parsed and appears to be unused by the current code.

### 6. `gardener` binary committed to the repository
A compiled Linux binary (`/gardener`) is tracked in git. It will become stale and bloat the repository history.

### 7. `gardener.log` committed to the repository
A runtime log file is tracked in git. Logs should be listed in `.gitignore`.

---

## Code Quality

### Debug `fmt.Println` statements in production handler (`gardener.go:190-202`)
`MsgHandler` contains several `fmt.Println("Got soil: ")` and `fmt.Printf("msg: %#v\n", msg)` calls. Structured logging via `slog` is used elsewhere; these should be replaced with `slog.Debug(...)` or removed.

### `_ = <-ticker.C` should be `<-ticker.C` (`gardener.go:220`)
```go
case _ = <-ticker.C:
```
The blank assignment is unnecessary. `case <-ticker.C:` is idiomatic Go.

### `panic` on all device init errors (`gardener.go:69,82,99,117,145,157`)
Every `New(...)` call panics on error. For a long-running daemon on dedicated hardware this is often acceptable, but it makes the emulator/mock path brittle on developer machines. Consider returning errors from `Init()` and letting `main()` decide whether to fatal.

### MQTT password visible in process listing (`main.go:33`)
`-mqtt-password` is a CLI flag, so the password will appear in `ps aux` output. Consider reading it from an environment variable or a file instead.

### `go.mod` uses `replace` directives pointing to local paths (`go.mod:10-11`)
```
replace github.com/rustyeddy/otto => ../otto
replace github.com/rustyeddy/devices => ../devices
```
These are development conveniences but will break any build that doesn't have the sibling directories present, including CI builds that only clone this repository. Consider publishing tagged releases of `otto` and `devices` and removing the `replace` directives when they stabilize, or documenting the required workspace layout clearly.

---

## Frontend

### Hardcoded IP address (`app/js/gardener.js:3`)
```js
const host = "ws://10.11.1.11:8080";
```
This is a private LAN address that will only work on one specific network. The host should be derived dynamically from `window.location.hostname` or injected by the server template at render time.

### `var msg` declared twice in same `switch` scope (`gardener.js:76,91`)
```js
var msg = JSON.parse(message);  // case "env"
...
var msg = JSON.parse(message)   // case "hello"
```
`var` declarations are function-scoped in JavaScript, so the second declaration silently overwrites the first. Use `const` or `let` inside each `case` block with braces.

### MQTT topic mismatch between JS client and Go server
As noted above, the browser subscribes to `ss/d/+/env` and publishes to `ss/c/station/pump`, while the Go server publishes to `d/env` and subscribes to `c/pump`. No messages will flow between them.

---

## CI / Build

The GitHub Actions workflow (`.github/workflows/go.yml`) is well structured:

- Builds for 7 platform/arch combinations including Pi Zero (ARMv6), Pi 3 (ARMv7), Pi 4/5 (ARM64), macOS, and Windows.
- Runs tests only on `linux-amd64` to avoid cross-compilation test issues.
- Uses `go-version-file: go.mod` to pin the Go version.
- Uploads build artifacts per platform.

One note: `go build ./...` in the workflow will fail if the `replace` directives in `go.mod` cannot be resolved. This would cause CI failures for anyone forking the repository without the sibling directories.

---

## Summary of Priority Issues

| Priority | Issue |
|---|---|
| High | `controller.go` is empty — core auto-watering logic is missing |
| High | MQTT topic mismatch between Go server and JS client — UI receives no data |
| High | `InitApp()` never called — web UI is not served |
| Medium | `config.json` is invalid JSON and unused |
| Medium | `replace` directives break external builds |
| Medium | Compiled binary and log file committed to git |
| Low | Hardcoded IP in `gardener.js` |
| Low | `fmt.Println` debug statements in `MsgHandler` |
| Low | `_ = <-ticker.C` style nit |
| Low | MQTT password exposed in process listing |
