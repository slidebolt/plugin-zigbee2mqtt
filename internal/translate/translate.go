package translate

// translate.go — Zigbee2MQTT Protocol Translation Layer
//
// This file translates between Z2M MQTT JSON format and SlideBolt domain types.
//
// Z2M Protocol Notes:
//   - Discovery: published to homeassistant/<type>/<device_id>/config
//   - State messages: published to zigbee2mqtt/<friendly_name>
//   - Command messages: published to zigbee2mqtt/<friendly_name>/set
//   - Brightness scale: 0-254 (Zigbee standard)
//   - Color temperature: mireds (153-500 typical range)
//   - State values: "ON"/"OFF" for on/off controls
//
//   Decode: Z2M JSON state → canonical domain state (lenient)
//     - Called on every inbound state message from the device
//     - Handle value_template transformations
//     - Clamp out-of-range values rather than reject
//     - Return (nil, false) to silently skip unrecognisable payloads
//
//   Encode: SlideBolt command → Z2M JSON payload (strict)
//     - Called when a command arrives for an entity
//     - Return an error for anything invalid — never send bad data to a device
//     - internal contains the discovery payload with topics, scales, templates

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	domain "github.com/slidebolt/sb-domain"
)

// DiscoveryPayload represents the Z2M discovery configuration
// Uses flexible types to handle various Z2M payload formats
type DiscoveryPayload struct {
	// Common fields
	Name              string          `json:"name"`
	StateTopic        string          `json:"state_topic"`
	CommandTopic      string          `json:"command_topic"`
	AvailabilityTopic string          `json:"availability_topic"`
	Device            json.RawMessage `json:"dev,omitempty"` // Device info for debugging

	// Light specific
	Brightness      bool     `json:"brightness"`
	BrightnessScale int      `json:"brightness_scale"`
	ColorTemp       bool     `json:"color_temp"`
	MinMireds       int      `json:"min_mireds"`
	MaxMireds       int      `json:"max_mireds"`
	EffectList      []string `json:"effect_list"`

	// Switch/Cover/Lock/Fan - use RawMessage to handle string/bool/number
	PayloadOn     json.RawMessage `json:"payload_on"`
	PayloadOff    json.RawMessage `json:"payload_off"`
	PayloadOpen   json.RawMessage `json:"payload_open"`
	PayloadClose  json.RawMessage `json:"payload_close"`
	PayloadStop   json.RawMessage `json:"payload_stop"`
	PayloadLock   json.RawMessage `json:"payload_lock"`
	PayloadUnlock json.RawMessage `json:"payload_unlock"`

	// Cover
	PositionTopic    string `json:"position_topic"`
	SetPositionTopic string `json:"set_position_topic"`
	PositionOpen     int    `json:"position_open"`
	PositionClosed   int    `json:"position_closed"`

	// Sensor/Binary Sensor
	ValueTemplate     string `json:"value_template"`
	UnitOfMeasurement string `json:"unit_of_measurement"`
	DeviceClass       string `json:"device_class"`

	// Climate
	Modes            []string `json:"modes"`
	FanModes         []string `json:"fan_modes"`
	PresetModes      []string `json:"preset_modes"`
	ModeCommandTopic string   `json:"mode_command_topic"`
	TempCommandTopic string   `json:"temperature_command_topic"`
	FanCommandTopic  string   `json:"fan_mode_command_topic"`
	MinTemp          float64  `json:"min_temp"`
	MaxTemp          float64  `json:"max_temp"`
	TempStep         float64  `json:"temp_step"`
	Precision        float64  `json:"precision"`
	TemperatureUnit  string   `json:"temperature_unit"`

	// Number
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`

	// Select
	Options []string `json:"options"`

	// Text
	Pattern string `json:"pattern"`
	Mode    string `json:"mode"`
	TextMin int    `json:"min"`
	TextMax int    `json:"max"`

	// Fan
	PercentageCommandTopic string `json:"percentage_command_topic"`
	SpeedRangeMin          int    `json:"speed_range_min"`
	SpeedRangeMax          int    `json:"speed_range_max"`

	// Button
	PayloadPress json.RawMessage `json:"payload_press"`
}

type discoveryDeviceInfo struct {
	Name         string   `json:"name"`
	FriendlyName string   `json:"friendly_name"`
	Identifiers  []string `json:"identifiers"`
}

func (d *DiscoveryPayload) UnmarshalJSON(data []byte) error {
	type rawDiscoveryPayload DiscoveryPayload

	var aux struct {
		rawDiscoveryPayload
		NameAlias                   string          `json:"name"`
		StateTopicShort             string          `json:"stat_t"`
		CommandTopicShort           string          `json:"cmd_t"`
		AvailabilityTopicShort      string          `json:"avty_t"`
		BrightnessScaleShort        int             `json:"bri_scl"`
		ColorTempShort              bool            `json:"clr_temp"`
		MinMiredsShort              int             `json:"min_mirs"`
		MaxMiredsShort              int             `json:"max_mirs"`
		EffectListShort             []string        `json:"fx_list"`
		PayloadOnShort              json.RawMessage `json:"pl_on"`
		PayloadOffShort             json.RawMessage `json:"pl_off"`
		PayloadOpenShort            json.RawMessage `json:"pl_open"`
		PayloadCloseShort           json.RawMessage `json:"pl_cls"`
		PayloadStopShort            json.RawMessage `json:"pl_stop"`
		PayloadLockShort            json.RawMessage `json:"pl_lock"`
		PayloadUnlockShort          json.RawMessage `json:"pl_unlk"`
		PositionTopicShort          string          `json:"pos_t"`
		SetPositionTopicShort       string          `json:"set_pos_t"`
		PositionOpenShort           int             `json:"pos_open"`
		PositionClosedShort         int             `json:"pos_clsd"`
		ValueTemplateShort          string          `json:"val_tpl"`
		UnitOfMeasurementShort      string          `json:"unit_of_meas"`
		DeviceClassShort            string          `json:"dev_cla"`
		ModesShort                  []string        `json:"modes"`
		FanModesShort               []string        `json:"fan_modes"`
		PresetModesShort            []string        `json:"pr_modes"`
		ModeCommandTopicShort       string          `json:"mode_cmd_t"`
		TempCommandTopicShort       string          `json:"temp_cmd_t"`
		FanCommandTopicShort        string          `json:"fan_mode_cmd_t"`
		MinTempShort                float64         `json:"min_temp"`
		MaxTempShort                float64         `json:"max_temp"`
		TempStepShort               float64         `json:"temp_step"`
		PrecisionShort              float64         `json:"temp_step"`
		TemperatureUnitShort        string          `json:"temp_unit"`
		OptionsShort                []string        `json:"ops"`
		PatternShort                string          `json:"pattern"`
		ModeShort                   string          `json:"mode"`
		TextMinShort                int             `json:"min"`
		TextMaxShort                int             `json:"max"`
		PercentageCommandTopicShort string          `json:"pct_cmd_t"`
		SpeedRangeMinShort          int             `json:"spd_rng_min"`
		SpeedRangeMaxShort          int             `json:"spd_rng_max"`
		PayloadPressShort           json.RawMessage `json:"pl_prs"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*d = DiscoveryPayload(aux.rawDiscoveryPayload)

	applyString := func(dst *string, value string) {
		if *dst == "" && value != "" {
			*dst = value
		}
	}
	applyInt := func(dst *int, value int) {
		if *dst == 0 && value != 0 {
			*dst = value
		}
	}
	applyFloat := func(dst *float64, value float64) {
		if *dst == 0 && value != 0 {
			*dst = value
		}
	}
	applyStrings := func(dst *[]string, value []string) {
		if len(*dst) == 0 && len(value) > 0 {
			*dst = value
		}
	}
	applyRaw := func(dst *json.RawMessage, value json.RawMessage) {
		if len(*dst) == 0 && len(value) > 0 {
			*dst = value
		}
	}

	applyString(&d.Name, aux.NameAlias)
	applyString(&d.StateTopic, aux.StateTopicShort)
	applyString(&d.CommandTopic, aux.CommandTopicShort)
	applyString(&d.AvailabilityTopic, aux.AvailabilityTopicShort)
	applyInt(&d.BrightnessScale, aux.BrightnessScaleShort)
	if !d.ColorTemp && aux.ColorTempShort {
		d.ColorTemp = true
	}
	applyInt(&d.MinMireds, aux.MinMiredsShort)
	applyInt(&d.MaxMireds, aux.MaxMiredsShort)
	applyStrings(&d.EffectList, aux.EffectListShort)
	applyRaw(&d.PayloadOn, aux.PayloadOnShort)
	applyRaw(&d.PayloadOff, aux.PayloadOffShort)
	applyRaw(&d.PayloadOpen, aux.PayloadOpenShort)
	applyRaw(&d.PayloadClose, aux.PayloadCloseShort)
	applyRaw(&d.PayloadStop, aux.PayloadStopShort)
	applyRaw(&d.PayloadLock, aux.PayloadLockShort)
	applyRaw(&d.PayloadUnlock, aux.PayloadUnlockShort)
	applyString(&d.PositionTopic, aux.PositionTopicShort)
	applyString(&d.SetPositionTopic, aux.SetPositionTopicShort)
	applyInt(&d.PositionOpen, aux.PositionOpenShort)
	applyInt(&d.PositionClosed, aux.PositionClosedShort)
	applyString(&d.ValueTemplate, aux.ValueTemplateShort)
	applyString(&d.UnitOfMeasurement, aux.UnitOfMeasurementShort)
	applyString(&d.DeviceClass, aux.DeviceClassShort)
	applyStrings(&d.Modes, aux.ModesShort)
	applyStrings(&d.FanModes, aux.FanModesShort)
	applyStrings(&d.PresetModes, aux.PresetModesShort)
	applyString(&d.ModeCommandTopic, aux.ModeCommandTopicShort)
	applyString(&d.TempCommandTopic, aux.TempCommandTopicShort)
	applyString(&d.FanCommandTopic, aux.FanCommandTopicShort)
	applyFloat(&d.MinTemp, aux.MinTempShort)
	applyFloat(&d.MaxTemp, aux.MaxTempShort)
	applyFloat(&d.TempStep, aux.TempStepShort)
	applyFloat(&d.Precision, aux.PrecisionShort)
	applyString(&d.TemperatureUnit, aux.TemperatureUnitShort)
	applyStrings(&d.Options, aux.OptionsShort)
	applyString(&d.Pattern, aux.PatternShort)
	applyString(&d.Mode, aux.ModeShort)
	applyInt(&d.TextMin, aux.TextMinShort)
	applyInt(&d.TextMax, aux.TextMaxShort)
	applyString(&d.PercentageCommandTopic, aux.PercentageCommandTopicShort)
	applyInt(&d.SpeedRangeMin, aux.SpeedRangeMinShort)
	applyInt(&d.SpeedRangeMax, aux.SpeedRangeMaxShort)
	applyRaw(&d.PayloadPress, aux.PayloadPressShort)

	return nil
}

func (d DiscoveryPayload) DeviceName() string {
	if len(d.Device) == 0 {
		return ""
	}

	var dev discoveryDeviceInfo
	if err := json.Unmarshal(d.Device, &dev); err != nil {
		return ""
	}
	if dev.Name != "" {
		return dev.Name
	}
	return dev.FriendlyName
}

// GetPayloadString extracts a string value from a RawMessage payload field
func (d DiscoveryPayload) GetPayloadString(field json.RawMessage) string {
	if len(field) == 0 {
		return ""
	}

	// Try as string first
	var strVal string
	if err := json.Unmarshal(field, &strVal); err == nil {
		return strVal
	}

	// Try as bool
	var boolVal bool
	if err := json.Unmarshal(field, &boolVal); err == nil {
		if boolVal {
			return "ON"
		}
		return "OFF"
	}

	// Try as number
	var numVal float64
	if err := json.Unmarshal(field, &numVal); err == nil {
		return fmt.Sprintf("%v", numVal)
	}

	return string(field)
}

// Z2MLightState represents the JSON structure from Z2M state messages
type Z2MLightState struct {
	State      string `json:"state"`
	Brightness int    `json:"brightness"`
	ColorTemp  int    `json:"color_temp"`
	Color      struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		R int     `json:"r"`
		G int     `json:"g"`
		B int     `json:"b"`
	} `json:"color"`
	ColorMode   string `json:"color_mode"`
	Linkquality int    `json:"linkquality"`
}

// Z2MSwitchState represents switch state from Z2M
type Z2MSwitchState struct {
	State       string `json:"state"`
	Linkquality int    `json:"linkquality"`
}

// Z2MCoverState represents cover state from Z2M
type Z2MCoverState struct {
	State       string `json:"state"`
	Position    int    `json:"position"`
	Linkquality int    `json:"linkquality"`
}

// Z2MFanState represents fan state from Z2M
type Z2MFanState struct {
	State       string `json:"state"`
	FanSpeed    int    `json:"fan_speed"`
	Percentage  int    `json:"percentage"`
	Linkquality int    `json:"linkquality"`
}

// Z2MLockState represents lock state from Z2M
type Z2MLockState struct {
	State       string `json:"state"`
	LockState   string `json:"lock_state"`
	Linkquality int    `json:"linkquality"`
}

// Z2MClimateState represents climate state from Z2M
type Z2MClimateState struct {
	LocalTemperature       float64 `json:"local_temperature"`
	CurrentHeatingSetpoint float64 `json:"current_heating_setpoint"`
	SystemMode             string  `json:"system_mode"`
	RunningState           string  `json:"running_state"`
	FanMode                string  `json:"fan_mode"`
	Preset                 string  `json:"preset"`
	Linkquality            int     `json:"linkquality"`
}

// Z2MSensorState represents sensor state from Z2M
type Z2MSensorState struct {
	Temperature *float64 `json:"temperature"`
	Humidity    *float64 `json:"humidity"`
	Pressure    *float64 `json:"pressure"`
	Illuminance *float64 `json:"illuminance"`
	Battery     *int     `json:"battery"`
	Voltage     *float64 `json:"voltage"`
	Current     *float64 `json:"current"`
	Power       *float64 `json:"power"`
	Energy      *float64 `json:"energy"`
	Contact     *bool    `json:"contact"`
	Occupancy   *bool    `json:"occupancy"`
	WaterLeak   *bool    `json:"water_leak"`
	Smoke       *bool    `json:"smoke"`
	CO2         *float64 `json:"co2"`
	CO          *float64 `json:"co"`
	Linkquality int      `json:"linkquality"`
}

// Decode converts a raw Z2M JSON payload into a canonical domain state.
// Returns (state, true) on success, (nil, false) to silently skip.
func Decode(entityType string, raw json.RawMessage) (any, bool) {
	return DecodeWithMeta(entityType, raw, "", "", "")
}

// DecodeWithMeta is like Decode but uses discovery metadata for accurate sensor decoding.
// valueField is the specific JSON field to extract (e.g. "linkquality", "power").
// unit and deviceClass are taken from the HA discovery payload when available.
func DecodeWithMeta(entityType string, raw json.RawMessage, valueField, unit, deviceClass string) (any, bool) {
	switch entityType {
	case "light":
		return decodeLight(raw)
	case "switch":
		return decodeSwitch(raw)
	case "cover":
		return decodeCover(raw)
	case "lock":
		return decodeLock(raw)
	case "fan":
		return decodeFan(raw)
	case "sensor":
		if valueField != "" {
			return decodeSensorField(raw, valueField, unit, deviceClass)
		}
		return decodeSensor(raw)
	case "binary_sensor":
		if valueField != "" {
			return decodeBinarySensorField(raw, valueField)
		}
		return decodeBinarySensor(raw)
	case "climate":
		return decodeClimate(raw)
	case "button":
		return decodeButton(raw)
	case "number":
		return decodeNumber(raw)
	case "select":
		return decodeSelect(raw)
	case "text":
		return decodeText(raw)
	default:
		return nil, false
	}
}

// Encode converts a SlideBolt domain command into a Z2M JSON payload.
// internal is the raw discovery payload previously stored with WriteFile(Internal).
// Returns an error if the command is invalid or unsupported.
func Encode(cmd any, internal json.RawMessage) (json.RawMessage, error) {
	switch c := cmd.(type) {
	case domain.LightTurnOn:
		return encodeLightTurnOn(c, internal)
	case domain.LightTurnOff:
		return encodeLightTurnOff(c, internal)
	case domain.LightSetBrightness:
		return encodeLightSetBrightness(c, internal)
	case domain.LightSetColorTemp:
		return encodeLightSetColorTemp(c, internal)
	case domain.LightSetRGB:
		return encodeLightSetRGB(c, internal)
	case domain.LightSetRGBW:
		return encodeLightSetRGBW(c, internal)
	case domain.LightSetRGBWW:
		return encodeLightSetRGBWW(c, internal)
	case domain.LightSetHS:
		return encodeLightSetHS(c, internal)
	case domain.LightSetXY:
		return encodeLightSetXY(c, internal)
	case domain.LightSetWhite:
		return encodeLightSetWhite(c, internal)
	case domain.LightSetEffect:
		return encodeLightSetEffect(c, internal)
	case domain.SwitchTurnOn:
		return encodeSwitchTurnOn(c, internal)
	case domain.SwitchTurnOff:
		return encodeSwitchTurnOff(c, internal)
	case domain.SwitchToggle:
		return encodeSwitchToggle(c, internal)
	case domain.FanTurnOn:
		return encodeFanTurnOn(c, internal)
	case domain.FanTurnOff:
		return encodeFanTurnOff(c, internal)
	case domain.FanSetSpeed:
		return encodeFanSetSpeed(c, internal)
	case domain.CoverOpen:
		return encodeCoverOpen(c, internal)
	case domain.CoverClose:
		return encodeCoverClose(c, internal)
	case domain.CoverSetPosition:
		return encodeCoverSetPosition(c, internal)
	case domain.LockLock:
		return encodeLockLock(c, internal)
	case domain.LockUnlock:
		return encodeLockUnlock(c, internal)
	case domain.ButtonPress:
		return encodeButtonPress(c, internal)
	case domain.NumberSetValue:
		return encodeNumberSetValue(c, internal)
	case domain.SelectOption:
		return encodeSelectOption(c, internal)
	case domain.TextSetValue:
		return encodeTextSetValue(c, internal)
	case domain.ClimateSetMode:
		return encodeClimateSetMode(c, internal)
	case domain.ClimateSetTemperature:
		return encodeClimateSetTemperature(c, internal)
	default:
		return nil, fmt.Errorf("translate: unsupported command type %T", cmd)
	}
}

// ---------------------------------------------------------------------------
// Decode: per-type (Z2M JSON state → domain state)
// ---------------------------------------------------------------------------

func decodeLight(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MLightState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Try to unmarshal as direct domain.Light (fallback)
		var s domain.Light
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return clampLight(s), true
	}

	// Convert Z2M format to domain.Light
	s := domain.Light{
		Power: z2m.State == "ON",
	}

	// Handle brightness (Z2M uses 0-254, domain uses 0-254)
	// Use default 254 only when explicitly ON and brightness field not present
	// We detect this by checking if brightness is 0 but linkquality or other fields exist
	// indicating this is a full state message
	if z2m.Brightness != 0 || z2m.Linkquality > 0 || z2m.ColorTemp > 0 || z2m.Color.X > 0 {
		// Brightness field is present (even if 0), use it
		s.Brightness = z2m.Brightness
	} else if z2m.State == "ON" {
		// Default brightness when ON but no brightness in payload
		s.Brightness = 254
	}

	// Handle color temperature in mireds
	if z2m.ColorTemp > 0 {
		s.Temperature = z2m.ColorTemp
	}

	// Handle color modes
	if z2m.ColorMode == "xy" && z2m.Color.X > 0 {
		s.XY = []float64{z2m.Color.X, z2m.Color.Y}
	}

	// Handle RGB color
	// Check if R, G, B fields are present (non-zero or explicitly set)
	if z2m.Color.R != 0 || z2m.Color.G != 0 || z2m.Color.B != 0 ||
		(z2m.Color.R == 0 && z2m.Color.G == 0 && z2m.Color.B == 0 && z2m.State == "ON") {
		s.RGB = []int{z2m.Color.R, z2m.Color.G, z2m.Color.B}
	}

	return clampLight(s), true
}

func clampLight(s domain.Light) domain.Light {
	if s.Brightness < 0 {
		s.Brightness = 0
	}
	if s.Brightness > 254 {
		s.Brightness = 254
	}
	if s.Temperature < 0 {
		s.Temperature = 0
	}
	if s.Temperature > 1000 {
		s.Temperature = 1000
	}
	return s
}

func decodeSwitch(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MSwitchState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.Switch
		var s domain.Switch
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return s, true
	}

	return domain.Switch{
		Power: z2m.State == "ON",
	}, true
}

func decodeCover(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MCoverState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.Cover
		var s domain.Cover
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return clampCover(s), true
	}

	s := domain.Cover{
		Position: z2m.Position,
	}

	return clampCover(s), true
}

func clampCover(s domain.Cover) domain.Cover {
	if s.Position < 0 {
		s.Position = 0
	}
	if s.Position > 100 {
		s.Position = 100
	}
	return s
}

func decodeLock(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MLockState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.Lock
		var s domain.Lock
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return s, true
	}

	locked := false
	if z2m.LockState == "LOCKED" || z2m.State == "LOCK" {
		locked = true
	}

	return domain.Lock{
		Locked: locked,
	}, true
}

func decodeFan(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MFanState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.Fan
		var s domain.Fan
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return clampFan(s), true
	}

	// Check if this is actually a Z2M fan payload
	isZ2M := z2m.State != "" || z2m.Linkquality > 0

	if !isZ2M {
		// Try to parse as direct domain.Fan
		var s domain.Fan
		if err := json.Unmarshal(raw, &s); err == nil {
			return clampFan(s), true
		}
	}

	s := domain.Fan{
		Power:      z2m.State == "ON",
		Percentage: z2m.Percentage,
	}

	// If percentage not provided but fan_speed is, convert it
	if s.Percentage == 0 && z2m.FanSpeed > 0 {
		// Assume fan_speed 1-3 maps to 33%, 66%, 100%
		s.Percentage = int(float64(z2m.FanSpeed) / 3.0 * 100)
	}

	return clampFan(s), true
}

func clampFan(s domain.Fan) domain.Fan {
	if s.Percentage < 0 {
		s.Percentage = 0
	}
	if s.Percentage > 100 {
		s.Percentage = 100
	}
	return s
}

func decodeSensor(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MSensorState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.Sensor
		var s domain.Sensor
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return s, true
	}

	// Determine sensor type and unit based on which field is present
	var value float64
	var unit string
	var deviceClass string

	switch {
	case z2m.Temperature != nil:
		value = *z2m.Temperature
		unit = "°C"
		deviceClass = "temperature"
	case z2m.Humidity != nil:
		value = *z2m.Humidity
		unit = "%"
		deviceClass = "humidity"
	case z2m.Pressure != nil:
		value = *z2m.Pressure
		unit = "hPa"
		deviceClass = "pressure"
	case z2m.Illuminance != nil:
		value = *z2m.Illuminance
		unit = "lx"
		deviceClass = "illuminance"
	case z2m.Battery != nil:
		value = float64(*z2m.Battery)
		unit = "%"
		deviceClass = "battery"
	case z2m.Voltage != nil:
		value = *z2m.Voltage
		unit = "V"
		deviceClass = "voltage"
	case z2m.Current != nil:
		value = *z2m.Current
		unit = "A"
		deviceClass = "current"
	case z2m.Power != nil:
		value = *z2m.Power
		unit = "W"
		deviceClass = "power"
	case z2m.Energy != nil:
		value = *z2m.Energy
		unit = "kWh"
		deviceClass = "energy"
	case z2m.CO2 != nil:
		value = *z2m.CO2
		unit = "ppm"
		deviceClass = "carbon_dioxide"
	case z2m.CO != nil:
		value = *z2m.CO
		unit = "ppm"
		deviceClass = "carbon_monoxide"
	default:
		// No recognized sensor type
		return nil, false
	}

	return domain.Sensor{
		Value:       value,
		Unit:        unit,
		DeviceClass: deviceClass,
	}, true
}

// decodeSensorField extracts a specific named field from a Z2M payload.
// This is used when the HA discovery value_template identifies the exact field,
// e.g. "{{ value_json.linkquality }}" → field="linkquality".
func decodeSensorField(raw json.RawMessage, field, unit, deviceClass string) (any, bool) {
	if len(raw) == 0 || field == "" {
		return nil, false
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}

	fieldRaw, ok := m[field]
	if !ok {
		return nil, false
	}

	// Try numeric value
	var fval float64
	if err := json.Unmarshal(fieldRaw, &fval); err == nil {
		return domain.Sensor{
			Value:       fval,
			Unit:        unit,
			DeviceClass: deviceClass,
		}, true
	}

	// Try string value (some sensors report string states)
	var sval string
	if err := json.Unmarshal(fieldRaw, &sval); err == nil {
		return domain.Sensor{
			Value:       sval,
			Unit:        unit,
			DeviceClass: deviceClass,
		}, true
	}

	return nil, false
}

// decodeBinarySensorField extracts a specific boolean field from a Z2M payload.
func decodeBinarySensorField(raw json.RawMessage, field string) (any, bool) {
	if len(raw) == 0 || field == "" {
		return nil, false
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}

	fieldRaw, ok := m[field]
	if !ok {
		return nil, false
	}

	// Boolean field
	var bval bool
	if err := json.Unmarshal(fieldRaw, &bval); err == nil {
		return domain.BinarySensor{On: bval}, true
	}

	// String ON/OFF
	var sval string
	if err := json.Unmarshal(fieldRaw, &sval); err == nil {
		return domain.BinarySensor{On: strings.EqualFold(sval, "on") || strings.EqualFold(sval, "true")}, true
	}

	return nil, false
}

func decodeBinarySensor(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MSensorState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.BinarySensor
		var s domain.BinarySensor
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return s, true
	}

	// Check if this is actually a Z2M sensor payload by looking for recognized fields
	isZ2M := z2m.Contact != nil || z2m.Occupancy != nil || z2m.WaterLeak != nil ||
		z2m.Smoke != nil || z2m.Battery != nil || z2m.Linkquality > 0

	if !isZ2M {
		// Try to parse as direct domain.BinarySensor
		var s domain.BinarySensor
		if err := json.Unmarshal(raw, &s); err == nil {
			return s, true
		}
	}

	// Determine binary sensor type and value from Z2M format
	var on bool
	var deviceClass string

	switch {
	case z2m.Contact != nil:
		on = !*z2m.Contact // Contact sensor: true = no contact, false = contact
		deviceClass = "door"
	case z2m.Occupancy != nil:
		on = *z2m.Occupancy
		deviceClass = "occupancy"
	case z2m.WaterLeak != nil:
		on = *z2m.WaterLeak
		deviceClass = "moisture"
	case z2m.Smoke != nil:
		on = *z2m.Smoke
		deviceClass = "smoke"
	default:
		// Try to parse as generic on/off state
		var genericState struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(raw, &genericState); err == nil {
			on = genericState.State == "ON"
		} else {
			return nil, false
		}
	}

	return domain.BinarySensor{
		On:          on,
		DeviceClass: deviceClass,
	}, true
}

func decodeClimate(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var z2m Z2MClimateState
	if err := json.Unmarshal(raw, &z2m); err != nil {
		// Fallback to direct domain.Climate
		var s domain.Climate
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, false
		}
		return s, true
	}

	s := domain.Climate{
		HVACMode:    mapZ2MModeToHVAC(z2m.SystemMode),
		Temperature: int(z2m.CurrentHeatingSetpoint),
	}

	return s, true
}

// mapZ2MModeToHVAC maps Z2M system_mode to SlideBolt HVACMode
func mapZ2MModeToHVAC(z2mMode string) string {
	switch strings.ToLower(z2mMode) {
	case "off":
		return "off"
	case "heat", "heating":
		return "heat"
	case "cool", "cooling":
		return "cool"
	case "fan_only":
		return "fan_only"
	case "dry":
		return "dry"
	case "auto":
		return "auto"
	default:
		return z2mMode
	}
}

func decodeButton(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	// Buttons are write-only, but we can parse state for press counts
	var state struct {
		Action      string `json:"action"`
		ActionCount int    `json:"action_count"`
		Linkquality int    `json:"linkquality"`
	}

	if err := json.Unmarshal(raw, &state); err != nil {
		// Fallback to direct domain.Button
		var s domain.Button
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return domain.Button{Presses: 0}, true
		}
		return s, true
	}

	// Check if this is actually a Z2M button payload
	isZ2M := state.Action != "" || state.ActionCount > 0 || state.Linkquality > 0

	if !isZ2M {
		// Try to parse as direct domain.Button
		var s domain.Button
		if err := json.Unmarshal(raw, &s); err == nil {
			return s, true
		}
	}

	return domain.Button{
		Presses: state.ActionCount,
	}, true
}

func decodeNumber(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var s domain.Number
	if err := json.Unmarshal(raw, &s); err != nil {
		// Try to extract value from nested field
		var state struct {
			Value float64 `json:"value"`
		}
		if err2 := json.Unmarshal(raw, &state); err2 != nil {
			return nil, false
		}
		s.Value = state.Value
	}

	return s, true
}

func decodeSelect(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var s domain.Select
	if err := json.Unmarshal(raw, &s); err != nil {
		// Try to extract option from nested field
		var state struct {
			State string `json:"state"`
		}
		if err2 := json.Unmarshal(raw, &state); err2 != nil {
			return nil, false
		}
		s.Option = state.State
	}

	return s, true
}

func decodeText(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var s domain.Text
	if err := json.Unmarshal(raw, &s); err != nil {
		// Try to extract text from nested field
		var state struct {
			Text string `json:"text"`
		}
		if err2 := json.Unmarshal(raw, &state); err2 != nil {
			return nil, false
		}
		s.Value = state.Text
	}

	return s, true
}

// ---------------------------------------------------------------------------
// Encode: per-command (domain command → Z2M JSON payload)
// ---------------------------------------------------------------------------

func encodeLightTurnOn(_ domain.LightTurnOn, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"state": "ON",
	})
}

func encodeLightTurnOff(_ domain.LightTurnOff, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"state": "OFF",
	})
}

func encodeLightSetBrightness(c domain.LightSetBrightness, internal json.RawMessage) (json.RawMessage, error) {
	if c.Brightness < 0 || c.Brightness > 254 {
		return nil, fmt.Errorf("translate: brightness %d out of range [0,254]", c.Brightness)
	}

	return json.Marshal(map[string]any{
		"state":      "ON",
		"brightness": c.Brightness,
	})
}

func encodeLightSetColorTemp(c domain.LightSetColorTemp, internal json.RawMessage) (json.RawMessage, error) {
	if c.Mireds < 153 || c.Mireds > 500 {
		return nil, fmt.Errorf("translate: mireds %d out of range [153,500]", c.Mireds)
	}

	return json.Marshal(map[string]any{
		"state":      "ON",
		"color_temp": c.Mireds,
	})
}

func encodeLightSetRGB(c domain.LightSetRGB, internal json.RawMessage) (json.RawMessage, error) {
	for name, v := range map[string]int{"r": c.R, "g": c.G, "b": c.B} {
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("translate: %s value %d out of range [0,255]", name, v)
		}
	}

	return json.Marshal(map[string]any{
		"state": "ON",
		"color": map[string]any{
			"r": c.R,
			"g": c.G,
			"b": c.B,
		},
	})
}

func encodeLightSetRGBW(c domain.LightSetRGBW, internal json.RawMessage) (json.RawMessage, error) {
	for name, v := range map[string]int{"r": c.R, "g": c.G, "b": c.B, "w": c.W} {
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("translate: %s value %d out of range [0,255]", name, v)
		}
	}

	return json.Marshal(map[string]any{
		"state": "ON",
		"color": map[string]any{
			"r": c.R,
			"g": c.G,
			"b": c.B,
		},
		"white_value": c.W,
	})
}

func encodeLightSetRGBWW(c domain.LightSetRGBWW, internal json.RawMessage) (json.RawMessage, error) {
	for name, v := range map[string]int{"r": c.R, "g": c.G, "b": c.B, "cw": c.CW, "ww": c.WW} {
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("translate: %s value %d out of range [0,255]", name, v)
		}
	}

	return json.Marshal(map[string]any{
		"state": "ON",
		"color": map[string]any{
			"r": c.R,
			"g": c.G,
			"b": c.B,
		},
		"color_temp": c.CW, // Approximate mapping
	})
}

func encodeLightSetHS(c domain.LightSetHS, internal json.RawMessage) (json.RawMessage, error) {
	if c.Hue < 0 || c.Hue > 360 {
		return nil, fmt.Errorf("translate: hue %.2f out of range [0,360]", c.Hue)
	}
	if c.Saturation < 0 || c.Saturation > 100 {
		return nil, fmt.Errorf("translate: saturation %.2f out of range [0,100]", c.Saturation)
	}

	return json.Marshal(map[string]any{
		"state": "ON",
		"color": map[string]any{
			"hue":        c.Hue,
			"saturation": c.Saturation,
		},
	})
}

func encodeLightSetXY(c domain.LightSetXY, internal json.RawMessage) (json.RawMessage, error) {
	if c.X < 0 || c.X > 1 {
		return nil, fmt.Errorf("translate: x %.4f out of range [0,1]", c.X)
	}
	if c.Y < 0 || c.Y > 1 {
		return nil, fmt.Errorf("translate: y %.4f out of range [0,1]", c.Y)
	}

	return json.Marshal(map[string]any{
		"state": "ON",
		"color": map[string]any{
			"x": c.X,
			"y": c.Y,
		},
	})
}

func encodeLightSetWhite(c domain.LightSetWhite, internal json.RawMessage) (json.RawMessage, error) {
	if c.White < 0 || c.White > 254 {
		return nil, fmt.Errorf("translate: white %d out of range [0,254]", c.White)
	}

	return json.Marshal(map[string]any{
		"state":      "ON",
		"brightness": c.White,
		"color_mode": "white",
	})
}

func encodeLightSetEffect(c domain.LightSetEffect, internal json.RawMessage) (json.RawMessage, error) {
	if c.Effect == "" {
		return nil, fmt.Errorf("translate: effect must not be empty")
	}

	return json.Marshal(map[string]any{
		"state":  "ON",
		"effect": c.Effect,
	})
}

func encodeSwitchTurnOn(_ domain.SwitchTurnOn, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"state": "ON"})
}

func encodeSwitchTurnOff(_ domain.SwitchTurnOff, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"state": "OFF"})
}

func encodeSwitchToggle(_ domain.SwitchToggle, internal json.RawMessage) (json.RawMessage, error) {
	// Z2M doesn't have native toggle, we'll handle this at a higher level
	return json.Marshal(map[string]any{"state": "TOGGLE"})
}

func encodeFanTurnOn(_ domain.FanTurnOn, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"state":      "ON",
		"percentage": 100,
	})
}

func encodeFanTurnOff(_ domain.FanTurnOff, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"state":      "OFF",
		"percentage": 0,
	})
}

func encodeFanSetSpeed(c domain.FanSetSpeed, internal json.RawMessage) (json.RawMessage, error) {
	if c.Percentage < 0 || c.Percentage > 100 {
		return nil, fmt.Errorf("translate: fan percentage %d out of range 0-100", c.Percentage)
	}

	state := "ON"
	if c.Percentage == 0 {
		state = "OFF"
	}

	return json.Marshal(map[string]any{
		"state":      state,
		"percentage": c.Percentage,
	})
}

func encodeCoverOpen(_ domain.CoverOpen, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"state": "OPEN"})
}

func encodeCoverClose(_ domain.CoverClose, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"state": "CLOSE"})
}

func encodeCoverSetPosition(c domain.CoverSetPosition, internal json.RawMessage) (json.RawMessage, error) {
	if c.Position < 0 || c.Position > 100 {
		return nil, fmt.Errorf("translate: cover position %d out of range 0-100", c.Position)
	}

	return json.Marshal(map[string]any{
		"state":    "ON",
		"position": c.Position,
	})
}

func encodeLockLock(_ domain.LockLock, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"state": "LOCK"})
}

func encodeLockUnlock(_ domain.LockUnlock, internal json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"state": "UNLOCK"})
}

func encodeButtonPress(_ domain.ButtonPress, internal json.RawMessage) (json.RawMessage, error) {
	// Extract payload_press from discovery if available
	var discovery DiscoveryPayload
	payload := ""
	if len(internal) > 0 {
		if err := json.Unmarshal(internal, &discovery); err == nil {
			payload = discovery.GetPayloadString(discovery.PayloadPress)
		}
	}
	if payload == "" {
		payload = "PRESS"
	}

	return json.Marshal(map[string]any{"action": payload})
}

func encodeNumberSetValue(c domain.NumberSetValue, internal json.RawMessage) (json.RawMessage, error) {
	// Extract min/max from discovery for validation
	var discovery DiscoveryPayload
	if len(internal) > 0 {
		if err := json.Unmarshal(internal, &discovery); err == nil {
			if discovery.Min != 0 || discovery.Max != 0 {
				if c.Value < discovery.Min || c.Value > discovery.Max {
					return nil, fmt.Errorf("translate: value %.2f out of range [%.2f,%.2f]",
						c.Value, discovery.Min, discovery.Max)
				}
			}
		}
	}

	return json.Marshal(map[string]any{"value": c.Value})
}

func encodeSelectOption(c domain.SelectOption, internal json.RawMessage) (json.RawMessage, error) {
	if c.Option == "" {
		return nil, fmt.Errorf("translate: select option must not be empty")
	}

	// Validate option is in allowed list
	var discovery DiscoveryPayload
	if len(internal) > 0 {
		if err := json.Unmarshal(internal, &discovery); err == nil {
			valid := false
			for _, opt := range discovery.Options {
				if opt == c.Option {
					valid = true
					break
				}
			}
			if !valid && len(discovery.Options) > 0 {
				return nil, fmt.Errorf("translate: option %q not in allowed list %v", c.Option, discovery.Options)
			}
		}
	}

	return json.Marshal(map[string]any{"state": c.Option})
}

func encodeTextSetValue(c domain.TextSetValue, internal json.RawMessage) (json.RawMessage, error) {
	if c.Value == "" {
		return nil, fmt.Errorf("translate: text value must not be empty")
	}

	// Extract min/max length and pattern from discovery
	var discovery DiscoveryPayload
	if len(internal) > 0 {
		if err := json.Unmarshal(internal, &discovery); err == nil {
			// Check min/max length
			if discovery.TextMin > 0 && len(c.Value) < discovery.TextMin {
				return nil, fmt.Errorf("translate: text length %d less than minimum %d",
					len(c.Value), discovery.TextMin)
			}
			if discovery.TextMax > 0 && len(c.Value) > discovery.TextMax {
				return nil, fmt.Errorf("translate: text length %d exceeds maximum %d",
					len(c.Value), discovery.TextMax)
			}
		}
	}

	return json.Marshal(map[string]any{"text": c.Value})
}

func encodeClimateSetMode(c domain.ClimateSetMode, internal json.RawMessage) (json.RawMessage, error) {
	if c.HVACMode == "" {
		return nil, fmt.Errorf("translate: climate hvac_mode must not be empty")
	}

	// Map SlideBolt HVACMode back to Z2M system_mode
	z2mMode := mapHVACModeToZ2M(c.HVACMode)

	return json.Marshal(map[string]any{"system_mode": z2mMode})
}

// mapHVACModeToZ2M maps SlideBolt HVACMode to Z2M system_mode
func mapHVACModeToZ2M(hvacMode string) string {
	switch strings.ToLower(hvacMode) {
	case "off":
		return "off"
	case "heat":
		return "heat"
	case "cool":
		return "cool"
	case "fan_only":
		return "fan_only"
	case "dry":
		return "dry"
	case "auto":
		return "auto"
	default:
		return hvacMode
	}
}

func encodeClimateSetTemperature(c domain.ClimateSetTemperature, internal json.RawMessage) (json.RawMessage, error) {
	var discovery DiscoveryPayload
	if len(internal) > 0 {
		if err := json.Unmarshal(internal, &discovery); err == nil {
			// Validate against min/max temp
			if discovery.MinTemp != 0 || discovery.MaxTemp != 0 {
				temp := float64(c.Temperature)
				if temp < discovery.MinTemp || temp > discovery.MaxTemp {
					return nil, fmt.Errorf("translate: temperature %.1f out of range [%.1f,%.1f]",
						temp, discovery.MinTemp, discovery.MaxTemp)
				}
			}
			// Round to precision
			if discovery.Precision > 0 {
				temp := c.Temperature
				temp = math.Round(temp/discovery.Precision) * discovery.Precision
				c.Temperature = temp
			}
		}
	}

	return json.Marshal(map[string]any{
		"current_heating_setpoint": c.Temperature,
	})
}
