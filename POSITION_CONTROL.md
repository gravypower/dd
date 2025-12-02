# Position Control Feature

## Overview

Position control allows you to set the SmartDoor garage door to any position between 0% (closed) and 100% (open) using a slider in Home Assistant.

## Features Added

### 1. MQTT Topics

**Position Reporting:**
- Topic: `dd-door/{deviceID}/position`
- Payload: `0` to `100` (integer)
- Updates automatically when device position changes

**Set Position Command:**
- Topic: `dd-door/{deviceID}/set_position`
- Payload: `0` to `100` (integer)
- Allows setting exact door position via slider

### 2. Home Assistant Integration

The MQTT discovery configuration now includes:
```json
{
  "position_topic": "dd-door/{deviceID}/position",
  "set_position_topic": "dd-door/{deviceID}/set_position",
  "position_open": 100,
  "position_closed": 0
}
```

This enables the position slider in Home Assistant's cover entity UI.

### 3. Command Mapping

Position values are mapped to SmartDoor device commands using 5% increments:

| Position Range | Command | Description |
|----------------|---------|-------------|
| 0 | Close (4) | Fully closed |
| 1-5 | OpenPercent05 (32) | 5% open |
| 6-10 | OpenPercent10 (33) | 10% open |
| 11-15 | OpenPercent15 (34) | 15% open |
| ... | ... | ... |
| 91-95 | OpenPercent95 (50) | 95% open |
| 96-100 | Open (2) | Fully open |

### 4. Position Tracking

The daemon now:
- Publishes position updates whenever device status changes
- Reports intermediate positions during movement
- Updates Home Assistant's position slider in real-time
- Maintains position state even during opening/closing

## Usage

### In Home Assistant UI

1. **Slider Control:**
   - Drag the slider to set desired position (0-100%)
   - Door will move to that position automatically

2. **Quick Positions:**
   Create scripts for common heights:
   ```yaml
   script:
     garage_pet_mode:
       alias: "Pet Mode"
       sequence:
         - service: cover.set_cover_position
           target:
             entity_id: cover.garage_door
           data:
             position: 20

     garage_delivery_mode:
       alias: "Delivery Mode"
       sequence:
         - service: cover.set_cover_position
           target:
             entity_id: cover.garage_door
           data:
             position: 68
   ```

3. **Automations:**
   ```yaml
   automation:
     - alias: "Ventilation Mode at Night"
       trigger:
         - platform: time
           at: "22:00:00"
       condition:
         - condition: state
           entity_id: cover.garage_door
           state: "closed"
       action:
         - service: cover.set_cover_position
           target:
             entity_id: cover.garage_door
           data:
             position: 10  # Crack open for ventilation
   ```

### Via MQTT

**Set Position:**
```bash
mosquitto_pub -h mqtt-broker -t "dd-door/device123/set_position" -m "50"
```

**Monitor Position:**
```bash
mosquitto_sub -h mqtt-broker -t "dd-door/device123/position"
```

## Implementation Details

### Files Modified

1. **api/haus.go**
   - Added `PositionClosed` and `PositionOpen` constants
   - Added `PublishPosition()` method to `MQTTHandler`
   - Updated MQTT discovery config with position topics
   - Updated FSM callbacks to publish position on open/close

2. **api/devices.go**
   - Added `GetCommandForPosition(position int)` function
   - Maps 0-100 position to appropriate device command
   - Includes input validation and clamping

3. **bin/haus/main.go**
   - Added `handleSetPosition()` function
   - Updated MQTT subscription to include set_position topic
   - Added position publishing in status update loop
   - Now reports intermediate positions during movement

4. **api/devices_test.go**
   - Added `TestGetCommandForPosition()` with 30+ test cases
   - Added `TestGetCommandForPosition_AllPercentages()` for comprehensive coverage

### Error Handling

- Invalid position values (non-numeric) are logged and ignored
- Out-of-range positions are clamped to 0-100
- Command execution errors are logged but don't crash the daemon
- MQTT publish failures are logged and retried on next update

### Performance

- Position updates use QoS 0 (fire-and-forget) for minimal latency
- Position is published on every status poll (~2 second intervals)
- No additional device polling required

## Testing

Run the new tests:
```bash
go test ./api -v -run TestGetCommandForPosition
```

Expected output shows all 30+ test cases passing, including:
- Boundary conditions (negative, 0, 100, over 100)
- All percentage increments (5%, 10%, ..., 95%)
- In-between values that round to nearest 5%
- Full range scan (0-100)

## Common Use Cases

### Pet Door Mode (20%)
Perfect height for pets to enter/exit:
```yaml
position: 20
```

### Delivery/Parcel Mode (68%)
Allows package delivery without full access:
```yaml
position: 68
```

### Ventilation Mode (5-10%)
Small opening for air circulation:
```yaml
position: 5
```

### Half Open (50%)
General intermediate position:
```yaml
position: 50
```

## Limitations

- Device hardware supports 5% increments (positions between increments round up)
- Position reporting depends on device polling interval (~2 seconds)
- Very rapid position changes may show intermediate values briefly
- Position accuracy depends on device calibration

## Future Enhancements

Potential improvements:
1. Position presets in add-on configuration
2. Position history tracking
3. Auto-close timer based on position
4. Position-based triggers (e.g., "door > 50% open")
5. Motion-based auto-adjust

## Troubleshooting

**Position not updating in HA:**
- Check MQTT connection status
- Verify position_topic in MQTT discovery
- Check add-on logs for publish errors

**Door doesn't move to set position:**
- Verify device is online
- Check command execution logs
- Ensure position is in valid range (0-100)
- Test with manual open/close first

**Position shows wrong value:**
- Device may need calibration
- Check device status directly
- Verify position polling is working
