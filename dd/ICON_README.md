# Add-on Icon and Logo

## Requirements

Home Assistant add-ons should include visual assets for better presentation in the UI:

### Icon (Required for Store Listing)
- **Filename**: `icon.png`
- **Location**: `dd/icon.png`
- **Size**: 256x256 pixels (recommended)
- **Format**: PNG with transparency
- **Purpose**: Displayed in the add-on store and details page

### Logo (Optional but Recommended)
- **Filename**: `logo.png`
- **Location**: `dd/logo.png`
- **Size**: 128x128 pixels minimum
- **Format**: PNG with transparency
- **Purpose**: Shown in various UI contexts

## Design Guidelines

1. **Simple and Clear**: Icon should be recognizable at small sizes
2. **Brand Aligned**: Use colors consistent with SmartDoor branding
3. **Transparent Background**: PNG with alpha channel
4. **No Text**: Icons should be symbolic, not text-based
5. **Square Format**: Maintain 1:1 aspect ratio

## Suggested Icon Concept

For a garage door/SmartDoor integration:
- Garage door outline
- MQTT/connectivity symbol overlay
- Home Assistant blue accent color (#18BCF2)

## How to Add

1. Create `icon.png` (256x256px)
2. Create `logo.png` (128x128px) - optional
3. Place in `dd/` directory
4. Commit and rebuild add-on

## Temporary Solution

Until custom icons are created, the add-on will use Home Assistant's default icon.
