# SmartDoor Home Assistant Integration

This project provides a Home Assistant add-on for integrating SmartDoor smart garage door systems with Home Assistant via MQTT.

## Architecture Overview

### Components

The project consists of three main executables:

1. **`register`** (`bin/register`) - One-time credential registration with SmartDoor cloud servers
2. **`action`** (`bin/action`) - CLI utility for sending direct commands to devices (for testing)
3. **`haus`** (`bin/haus`) - Main daemon that bridges SmartDoor devices with Home Assistant via MQTT

### System Architecture

```
┌─────────────────────┐
│  Home Assistant     │
│  (MQTT Client)      │
└──────────┬──────────┘
           │ MQTT Protocol
           │ (Commands & Status)
┌──────────▼──────────┐
│   haus Daemon       │
│   - MQTT Bridge     │
│   - FSM Manager     │
│   - Status Polling  │
└──────────┬──────────┘
           │ Encrypted API
           │ (AES-CBC, HMAC)
┌──────────▼──────────┐
│  SmartDoor Device   │
│  (Local Network)    │
│  Ports: 8989, 8991  │
└─────────────────────┘
```

### Package Structure

- **Root Package** (`github.com/samthor/dd`)
  - `conn.go` - Device connection & encrypted communication
  - `crypto.go` - AES-CBC encryption/decryption, HMAC signing
  - `types.go` - Core data structures (Conn, Credential, Message, RPC)
  - `cert.go` - Embedded SSL certificates for SmartDoor CA

- **API Package** (`github.com/samthor/dd/api`)
  - `haus.go` - MQTT integration & finite state machine logic
  - `devices.go` - Device status structures and fetching
  - `command.go` - Command execution wrapper
  - `availableCommands.go` - Complete command mapping (40+ commands)
  - `info.go` - Basic device information retrieval

- **Helper Package** (`github.com/samthor/dd/helper`)
  - `creds.go` - Credential loading from JSON files
  - `messages.go` - Background message polling loop

- **Executables** (`bin/`)
  - `register/main.go` - Credential registration
  - `action/main.go` - Direct command execution
  - `haus/main.go` - Main Home Assistant integration daemon

## Device Communication

### API Endpoints

SmartDoor devices expose two HTTPS endpoints:

#### Public/Unencrypted API (Port 8991)
- Basic device information endpoint
- Example: `/sdk/info` returns device name, version, basestation ID

#### Encrypted API (Port 8989)
- All authenticated operations
- AES-CBC encryption with MD5-derived IV
- HMAC-SHA256 request signing
- Session-based authentication

### Communication Protocol

1. **Connection** (`/app/connect`)
   - Send credentials (base station ID, phone ID, phone secret)
   - Receive session ID and session secret
   - Establish next access timestamp

2. **Signed Requests**
   - Each request signed with both session and phone signatures
   - Timestamp coordination using `nextAccess` mechanism
   - Process ID tracking for async RPC responses

3. **Message Polling** (`/app/res/messages`)
   - Background polling for device status updates
   - Encrypted message payloads
   - Process ID matching for RPC responses

4. **Command Execution** (`/app/res/action`)
   - Send device commands (open, close, stop, etc.)
   - Percentage-based positioning (5%-95%)
   - Light, camera, and lockout controls

### Encryption Details

- **Algorithm**: AES-CBC
- **Key Derivation**: MD5 hash of phone secret (16 bytes for AES-128)
- **IV Derivation**: MD5 hash of timestamp
- **Padding**: PKCS5
- **Signature**: HMAC-SHA256(timestamp:data)

## MQTT Integration

### Home Assistant Discovery

The daemon publishes MQTT discovery configuration for automatic device setup:

```
Topic: homeassistant/cover/{deviceID}/config
Payload: {
  "name": "Device Name",
  "command_topic": "dd-door/{deviceID}/command",
  "state_topic": "dd-door/{deviceID}/state",
  "availability_topic": "dd-door/{deviceID}/availability",
  "device_class": "garage",
  ...
}
```

### MQTT Topics

- **Command Topic**: `dd-door/{deviceID}/command`
  - Payloads: `go_open`, `go_close`, `STOP`

- **State Topic**: `dd-door/{deviceID}/state`
  - Payloads: `opening`, `closing`, `open`, `closed`, `stopping`

- **Availability Topic**: `dd-door/{deviceID}/availability`
  - Payloads: `online`, `offline`

### Finite State Machine

Each device is managed by a state machine with the following states:

```
States:
  initial → online → {opening, closing, open, closed, stopping, stopped}
                  ↓
               offline

Events:
  go_online, go_offline, go_open, go_close, go_opened, go_closed, go_stop, go_stopped

Transitions:
  - go_online: initial/offline → online
  - go_open: online/closed/stopped → opening
  - go_opened: * → open
  - go_close: online/open/stopped → closing
  - go_closed: * → closed
  - go_stop: online/opening/closing → stopping
  - go_offline: * → offline
```

## Available Commands

### Basic Operations
- **Open** (2) - Fully open door
- **Close** (4) - Fully close door
- **Stop** (3) - Stop door movement

### Partial Opens
- **PartOpen1** (5) - Pet mode (~20%)
- **PartOpen2** (6) - Parcel mode (~68%)
- **PartOpen3** (7) - Custom height

### Percentage-Based Positioning
- Commands 32-50 for 5% to 95% positioning

### Auxiliary Controls
- **Light On/Off** (16, 17)
- **Aux On/Off** (18, 19)
- **Phone Lockout** (257, 258)
- **Remote Control Lockout** (20, 21)
- **Camera Alarms** (352-355)
- **Cycle Testing** (321, 322)

## Home Assistant Add-on

The `dd` directory contains the Home Assistant add-on configuration:

- Multi-architecture Docker support (aarch64, amd64, armhf, armv7, i386)
- S6-overlay for process management
- AppArmor security profile
- Automatic credential registration on first run

### Configuration Options

```yaml
code: "registration_code"      # From SmartDoor app
password: "registration_password"
host: "192.168.1.x"            # Local device IP
mqtt:
  broker: "core-mosquitto"
  port: 1883
  user: ""
  password: ""
debug: false
```

## Security Considerations

- Credentials stored in `/config/dd-credentials.json` (plaintext)
- SSL/TLS validation uses embedded SmartDoor CA certificates
- All device communication encrypted with AES-CBC
- HMAC-SHA256 signatures prevent request tampering
- Session-based authentication with server-provided secrets

## Thread Safety

- Global `DeviceFSMs` map protected by `sync.RWMutex`
- Thread-safe helper functions: `GetDeviceFSM()`, `SetDeviceFSM()`, `GetAllDeviceFSMs()`
- MQTT publish operations protected by mutex
- FSM callbacks acquire locks before modifying state

## Error Handling

- API functions return errors instead of calling `Fatal()`
- Graceful degradation when MQTT connection is temporarily lost
- Auto-reconnect for MQTT with persistent sessions
- Retry logic for configuration publishing
- Contextual error messages for crypto failures

## Development

### Building

```bash
go build -o register ./bin/register
go build -o action ./bin/action
go build -o haus ./bin/haus
```

### Testing

```bash
go test ./...                    # All tests
go test ./api -v                 # API package tests
go test -run TestEncryptDecrypt  # Specific test
```

### Adding New Commands

1. Add command constant to `api/availableCommands.go`
2. Update `AvailableCommandsMap` with string mapping
3. Document the command code range in comments

## Add-ons

- [**dd**: Home Assistant Add-on](./dd)
