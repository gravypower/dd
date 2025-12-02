# Home Assistant Add-on Updates Summary

This document details all updates made to bring the SmartDoor add-on configuration up to the latest Home Assistant best practices (verified December 2025).

## Research Sources

Based on official Home Assistant documentation:
- [Add-on Configuration | Home Assistant Developer Docs](https://developers.home-assistant.io/docs/add-ons/configuration/)
- [GitHub - home-assistant/docker-base](https://github.com/home-assistant/docker-base)
- Latest base image version: **Alpine 3.22** (confirmed as current stable)
- Latest tempio version: **2024.11.2** (already in use)

## Configuration Updates (`dd/config.yaml`)

### ✅ Added Required Fields (Best Practices)

| Field | Value | Purpose |
|-------|-------|---------|
| `startup` | `application` | Controls when add-on starts (after core services) |
| `boot` | `auto` | Automatically start on Home Assistant boot |

**Note:** Watchdog monitoring was removed in v0.3.3 as it was causing unnecessary restarts (add-on is an MQTT client, not a server).

### ✅ Updated Metadata

```yaml
name: SmartDoor MQTT Bridge  # More descriptive than "dd add-on"
version: "0.3.0"              # Bumped for position control feature
description: SmartDoor garage door integration with MQTT and position control support
url: https://github.com/gravypower/dd  # Updated repository URL
```

### ✅ Enhanced MQTT Configuration Schema

**Before:**
```yaml
options:
  mqtt: mqtt  # Simple string

schema:
  mqtt: str   # Basic validation
```

**After:**
```yaml
options:
  mqtt:
    broker: core-mosquitto
    port: 1883
    username: ""
    password: ""
  mqtt_prefix: dd-door

schema:
  mqtt:
    broker: str
    port: port
    username: str?      # Optional with '?'
    password: password?  # Optional with '?'
  mqtt_prefix: str
```

**Benefits:**
- Supports custom MQTT brokers (not just HA MQTT service)
- Proper schema validation with optional fields
- Configurable MQTT prefix for multi-instance setups
- Follows HA add-on schema best practices

## Service Script Updates (`dd/rootfs/etc/services.d/dd/run`)

### ✅ Enhanced MQTT Flexibility

Added support for custom MQTT brokers while maintaining backward compatibility:

```bash
# Check if user provided custom MQTT settings or use HA MQTT service
if bashio::config.exists 'mqtt.broker'; then
    bashio::log.info "Using custom MQTT broker configuration."
    MQTT_HOST="$(bashio::config 'mqtt.broker')"
    MQTT_PORT="$(bashio::config 'mqtt.port')"
    MQTT_USER="$(bashio::config 'mqtt.username')"
    MQTT_PASSWORD="$(bashio::config 'mqtt.password')"
else
    bashio::log.info "Using Home Assistant MQTT service."
    MQTT_HOST=$(bashio::services 'mqtt' 'host')
    # ... uses HA MQTT service
fi
```

### ✅ Added MQTT Prefix Support

```bash
MQTT_PREFIX="$(bashio::config 'mqtt_prefix')"
exec /usr/bin/dd/haus \
    ...
    -mqttPrefix "${MQTT_PREFIX}" \
    ...
```

## AppArmor Security Profile Updates (`dd/apparmor.txt`)

### ✅ Replaced Template Placeholders

**Before:**
- Profile name: `example`
- Service name: `my_program`
- Peer signal: `*_example`

**After:**
- Profile name: `dd_addon`
- Service name: `dd_service`
- Peer signal: `*_dd_addon`

### ✅ Added Required Permissions

Added network access and additional file permissions for proper operation:

```apparmor
# Network access for MQTT and SmartDoor device communication
network inet stream,
network inet6 stream,
network inet dgram,
network inet6 dgram,

# Additional system files
/etc/hosts r,
/etc/resolv.conf r,
/etc/ssl/certs/** r,
/dev/null rw,
/proc/*/stat r,
```

## Repository Metadata Updates (`repository.yaml`)

### ✅ Updated Repository Information

```yaml
name: SmartDoor MQTT Bridge Repository  # More descriptive
url: 'https://github.com/gravypower/dd'    # Correct URL
maintainer: SmartDoor Integration Team   # Removed placeholder
```

## Documentation Additions

### ✅ Created New Documentation Files

1. **`dd/CHANGELOG.md`** - Version history following Keep a Changelog format
2. **`dd/ICON_README.md`** - Guidelines for adding icon.png and logo.png
3. **`ADDON_UPDATES.md`** (this file) - Summary of all add-on updates

## Base Images Verification

### ✅ Confirmed Latest Versions (`dd/build.yaml`)

All base images verified as current (December 2025):
```yaml
build_from:
  aarch64: "ghcr.io/home-assistant/aarch64-base:3.22"  ✅ Alpine 3.22 (latest)
  amd64: "ghcr.io/home-assistant/amd64-base:3.22"      ✅ Alpine 3.22 (latest)
  armhf: "ghcr.io/home-assistant/armhf-base:3.22"      ✅ Alpine 3.22 (latest)
  armv7: "ghcr.io/home-assistant/armv7-base:3.22"      ✅ Alpine 3.22 (latest)
  i386: "ghcr.io/home-assistant/i386-base:3.22"        ✅ Alpine 3.22 (latest)

args:
  TEMPIO_VERSION: "2024.11.2"  ✅ Latest version
```

## Configuration Migration Guide

### For Existing Users

No breaking changes! The add-on configuration is backward compatible:

**Minimal config (uses HA MQTT service automatically):**
```yaml
code: "your-code"
password: "your-password"
host: "192.168.1.100"
mqtt_prefix: dd-door  # Optional, defaults to dd-door
debug: false
```

**With custom MQTT prefix:**
```yaml
code: "your-code"
password: "your-password"
host: "192.168.1.100"
mqtt_prefix: "custom-prefix"
debug: false
```

### For Custom MQTT Brokers

Only add the `mqtt` section if you want to use a custom broker instead of the Home Assistant MQTT integration.

**Note:** The `mqtt` section is not in the schema validation (to keep it optional), so you can add it manually:

```yaml
code: "your-code"
password: "your-password"
host: "192.168.1.100"
mqtt:
  broker: "192.168.1.50"  # External MQTT broker
  port: 1883
  username: "mqtt_user"   # Optional
  password: "mqtt_pass"   # Optional
mqtt_prefix: "custom-prefix"
debug: false
```

The add-on will automatically detect if the `mqtt.broker` field exists and use it instead of the HA MQTT service.

## Compliance Checklist

Based on [Home Assistant Add-on Configuration Best Practices](https://developers.home-assistant.io/docs/add-ons/configuration/):

- ✅ All required fields present (`name`, `version`, `slug`, `description`, `arch`)
- ✅ Proper `startup` configuration (`application`)
- ✅ Automatic boot configuration (`boot: auto`)
- ✅ Watchdog health monitoring enabled
- ✅ Services properly declared (`mqtt:need`)
- ✅ Schema validation with proper types
- ✅ Optional fields marked with `?`
- ✅ AppArmor profile properly configured
- ✅ Base images use latest stable Alpine (3.22)
- ✅ Repository metadata complete
- ✅ Changelog follows best practices
- ⚠️ Icon/logo pending (documented in ICON_README.md)

## Testing Recommendations

After deploying these updates:

1. **Configuration Validation:**
   - Install/update add-on in Home Assistant
   - Verify minimal config works without MQTT section (uses HA MQTT automatically)
   - Test with custom broker configuration if needed

2. **AppArmor Security:**
   - Review system logs for any AppArmor denials
   - Ensure network and file access permissions work

3. **Position Control:**
   - Test slider in Home Assistant UI
   - Verify position updates in real-time
   - Try preset positions (20%, 68%, etc.)

4. **Stability:**
   - Verify add-on runs continuously without restarts
   - Check logs for any termination signals or crashes

## Summary

All Home Assistant add-on configuration updates completed:

| Category | Status | Details |
|----------|--------|---------|
| **config.yaml** | ✅ Updated | Latest best practices, enhanced MQTT schema |
| **Service scripts** | ✅ Updated | Custom MQTT support, prefix configuration |
| **AppArmor profile** | ✅ Updated | Proper naming, network permissions |
| **Base images** | ✅ Verified | Latest Alpine 3.22 confirmed |
| **Repository metadata** | ✅ Updated | Proper naming and URLs |
| **Documentation** | ✅ Complete | Changelog, icon guide, updates summary |
| **Version bump** | ✅ Complete | 0.2.0 → 0.3.0 for position control |

The add-on now follows all current Home Assistant best practices and is ready for the v0.3.0 release!
