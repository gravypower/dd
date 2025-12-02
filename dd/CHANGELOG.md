# Changelog

All notable changes to this add-on will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [0.3.0] - 2025-12-02

### Added
- P **Position Control**: Full slider support for setting door position (0-100%)
  - New MQTT topics: `position` and `set_position`
  - Real-time position reporting with 5% granularity
  - Support for common presets (Pet Mode 20%, Delivery Mode 68%, Ventilation 5-10%)
- MQTT prefix configuration option (`mqtt_prefix`)
- Custom MQTT broker support (optional, defaults to HA MQTT service)
- Watchdog monitoring on MQTT TCP port
- Comprehensive unit tests for position mapping
- Complete architecture documentation

### Changed
- Updated add-on name to "SmartDoor MQTT Bridge"
- Improved add-on description
- Enhanced MQTT configuration with nested schema
- Set `startup` to `application` and `boot` to `auto` per HA best practices
- Updated AppArmor profile with correct service names and network permissions
- Improved error handling throughout codebase

### Fixed
- Critical race condition in MQTT command handler
- Aggressive Fatal() calls replaced with graceful error handling
- Thread-safe device FSM map access with RWMutex
- Better error context in crypto operations
- Documentation gaps filled with comprehensive guides

### Documentation
- Added `POSITION_CONTROL.md` with usage examples
- Added `IMPROVEMENTS.md` summarizing all enhancements
- Updated `README.md` with complete architecture documentation
- Created `ICON_README.md` for visual assets guidance

## [0.2.0] - Previous Release

### Added
- Initial MQTT integration
- Basic FSM state management
- Multi-architecture support

### Fixed
- MQTT connection resilience improvements
- Device state tracking

## [0.1.9] - Initial Release

### Added
- SmartDoor device integration
- Home Assistant MQTT discovery
- Basic door control (open/close/stop)
