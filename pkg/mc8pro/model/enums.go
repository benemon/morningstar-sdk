package model

// Message type values for [Message.Type] in preset and bank message
// arrays. Source: the editor's qG enum (editor.js module 6146),
// cross-validated against live captures (MsgTypeSetToggle and
// MsgTypeMidiClockTap observed on hardware) and the official 0x70
// SysEx API glossary (Empty/PC/CC).
//
// Expression presets use a DIFFERENT numbering — see the ExpMsgType*
// constants. (A third numbering exists for Morningstar's "Live Mode"
// product line and is out of scope for this SDK.)
const (
	MsgTypeEmpty              = 0
	MsgTypePC                 = 1
	MsgTypeCC                 = 2
	MsgTypeNoteOn             = 3
	MsgTypeNoteOff            = 4
	MsgTypeRealtime           = 5
	MsgTypeSysEx              = 6
	MsgTypeClockTap           = 7
	MsgTypePCScrollUp         = 8
	MsgTypePCScrollDown       = 9
	MsgTypeBankUp             = 10
	MsgTypeBankDown           = 11
	MsgTypeBankChangeMode     = 12
	MsgTypeSetBank            = 13
	MsgTypeTogglePage         = 14
	MsgTypeSetToggle          = 15
	MsgTypeMidiThru           = 16
	MsgTypeSelectExp          = 17
	MsgTypeLooperMode         = 18
	MsgTypeStrymonBankUp      = 19
	MsgTypeStrymonBankDown    = 20
	MsgTypeAxeFxTuner         = 21
	MsgTypeTogglePreset       = 22
	MsgTypeDelay              = 23
	MsgTypeMidiClockTap       = 24
	MsgTypeMidiSongPos        = 25
	MsgTypeCCWaveform         = 26
	MsgTypeEngagePreset       = 27
	MsgTypeKemperTuner        = 28
	MsgTypeMidiSongSelect     = 29
	MsgTypeSetDisplay         = 30
	MsgTypeCCSequencer        = 31
	MsgTypeCCScroll           = 32
	MsgTypePCScroll           = 33
	MsgTypePCMultiChannel     = 34
	MsgTypeKeystrokes         = 35
	MsgTypeUtility            = 36
	MsgTypeAxeFxIntegration   = 37
	MsgTypeNRPN               = 38
	MsgTypeML10X              = 39
	MsgTypeOmniportRelay      = 40
	MsgTypeMidiMMC            = 41
	MsgTypeTriggerMessages    = 42
	MsgTypeFocusMode          = 43
	MsgTypePresetRename       = 44
	MsgTypeML5R               = 45
	MsgTypeDeviceEngageBypass = 46
)

// MsgTypeNames maps [Message.Type] values in preset and bank messages
// to the display names the editor uses in its type selector.
var MsgTypeNames = map[int]string{
	MsgTypeEmpty:              "Empty",
	MsgTypePC:                 "PC (Program Change)",
	MsgTypeCC:                 "CC (Control Change)",
	MsgTypeNoteOn:             "Note On",
	MsgTypeNoteOff:            "Note Off",
	MsgTypeRealtime:           "Realtime",
	MsgTypeSysEx:              "SysEx",
	MsgTypeClockTap:           "MIDI Clock Tap Menu",
	MsgTypePCScrollUp:         "PC Scroll Up",
	MsgTypePCScrollDown:       "PC Scroll Down",
	MsgTypeBankUp:             "Bank Up",
	MsgTypeBankDown:           "Bank Down",
	MsgTypeBankChangeMode:     "Bank Change Mode",
	MsgTypeSetBank:            "Bank Jump",
	MsgTypeTogglePage:         "Toggle Page",
	MsgTypeSetToggle:          "Set Toggle",
	MsgTypeMidiThru:           "MIDI Thru",
	MsgTypeSelectExp:          "Select Exp Message",
	MsgTypeLooperMode:         "Looper Mode",
	MsgTypeStrymonBankUp:      "Strymon Bank Up",
	MsgTypeStrymonBankDown:    "Strymon Bank Down",
	MsgTypeAxeFxTuner:         "Axe FX Tuner",
	MsgTypeTogglePreset:       "Toggle Preset",
	MsgTypeDelay:              "Delay",
	MsgTypeMidiClockTap:       "MIDI Clock Tap",
	MsgTypeMidiSongPos:        "Song Position",
	MsgTypeCCWaveform:         "CC Waveform Generator",
	MsgTypeEngagePreset:       "Engage Preset",
	MsgTypeKemperTuner:        "Kemper Tuner",
	MsgTypeMidiSongSelect:     "Song Select",
	MsgTypeSetDisplay:         "Set Display",
	MsgTypeCCSequencer:        "Sequence Generator",
	MsgTypeCCScroll:           "CC Value Scroll",
	MsgTypePCScroll:           "PC Number Scroll",
	MsgTypePCMultiChannel:     "PC (Multi Channel)",
	MsgTypeKeystrokes:         "Keystrokes",
	MsgTypeUtility:            "Utility",
	MsgTypeAxeFxIntegration:   "Axe FX Integration",
	MsgTypeNRPN:               "NRPN",
	MsgTypeML10X:              "ML10X",
	MsgTypeOmniportRelay:      "Relay Switching",
	MsgTypeMidiMMC:            "MIDI MMC",
	MsgTypeTriggerMessages:    "Trigger Messages",
	MsgTypeFocusMode:          "Focus Mode",
	MsgTypePresetRename:       "Preset Rename",
	MsgTypeML5R:               "ML5R",
	MsgTypeDeviceEngageBypass: "Multi Engage/Bypass",
}

// Expression message type values for [Message.Type] in EXPRESSION
// preset message arrays (Bank.ExpPresetArray[n].MsgArray). Source:
// the editor's mQ enum (editor.js module 6146). These share the wire
// field with MsgType* but use a separate numbering — an expression
// message with Type 2 is "CC on Toe Down", NOT a plain CC.
const (
	ExpMsgTypeEmpty                = 0
	ExpMsgTypeCC                   = 1
	ExpMsgTypeCCToeDown            = 2
	ExpMsgTypeCCHeelDown           = 3
	ExpMsgTypeToggleChannelToeDown = 4
	ExpMsgTypeToggleCCToeDown      = 5
	ExpMsgTypeCCOnEngage           = 6
	ExpMsgTypeCCOnDisengage        = 7
	ExpMsgTypePCOnEngage           = 8
	ExpMsgTypePCOnDisengage        = 9
	ExpMsgTypePCToeDown            = 10
	ExpMsgTypePCHeelDown           = 11
	ExpMsgTypePitchBend            = 12
	ExpMsgTypeCCCustomResponse     = 13
	ExpMsgTypeWaveformEngineSpeed  = 14
	ExpMsgTypeSequencerEngineSpeed = 15
	ExpMsgTypeUtility              = 16
)

// ExpMsgTypeNames maps expression message types to display names.
var ExpMsgTypeNames = map[int]string{
	ExpMsgTypeEmpty:                "Empty",
	ExpMsgTypeCC:                   "Expression CC",
	ExpMsgTypeCCToeDown:            "CC on Toe Down",
	ExpMsgTypeCCHeelDown:           "CC on Heel Down",
	ExpMsgTypeToggleChannelToeDown: "Toggle Channel on Toe Down",
	ExpMsgTypeToggleCCToeDown:      "Toggle CC on Toe Down",
	ExpMsgTypeCCOnEngage:           "CC on Engage",
	ExpMsgTypeCCOnDisengage:        "CC on Disengage",
	ExpMsgTypePCOnEngage:           "PC on Engage",
	ExpMsgTypePCOnDisengage:        "PC on Disengage",
	ExpMsgTypePCToeDown:            "PC on Toe Down",
	ExpMsgTypePCHeelDown:           "PC on Heel Down",
	ExpMsgTypePitchBend:            "Pitch Bend",
	ExpMsgTypeCCCustomResponse:     "CC Custom Response",
	ExpMsgTypeWaveformEngineSpeed:  "Waveform Engine Speed",
	ExpMsgTypeSequencerEngineSpeed: "Sequencer Engine Speed",
	ExpMsgTypeUtility:              "Utility",
}

// Action (trigger) values for [Message.Action]. Source: the editor's
// action enum (editor.js:56330), cross-validated by the official
// 0x70 SysEx API glossary (0x00–0x0C) and the MC8 Pro manual's MIDI
// implementation chart (CC10–33 values 1–10).
const (
	ActionNothing                   = 0
	ActionPress                     = 1
	ActionRelease                   = 2
	ActionLongPress                 = 3
	ActionLongPressRelease          = 4
	ActionDoubleTap                 = 5
	ActionDoubleTapRelease          = 6
	ActionLongDoubleTap             = 7
	ActionLongDoubleTapRelease      = 8
	ActionReleaseAll                = 9
	ActionLongPressScroll           = 10
	ActionOnDisengage               = 11
	ActionOnFirstEngage             = 12
	ActionOnFirstEngageSendOnlyThis = 13
)

// ActionNames maps [Message.Action] values to display names.
var ActionNames = map[int]string{
	ActionNothing:                   "No Action",
	ActionPress:                     "Press",
	ActionRelease:                   "Release",
	ActionLongPress:                 "Long Press",
	ActionLongPressRelease:          "Long Press Release",
	ActionDoubleTap:                 "Double Tap",
	ActionDoubleTapRelease:          "Double Tap Release",
	ActionLongDoubleTap:             "Long Double Tap",
	ActionLongDoubleTapRelease:      "Long Double Tap Release",
	ActionReleaseAll:                "Release All",
	ActionLongPressScroll:           "Long Press Scroll",
	ActionOnDisengage:               "On Disengage",
	ActionOnFirstEngage:             "On First Engage",
	ActionOnFirstEngageSendOnlyThis: "On First Engage Send Only This",
}

// Toggle position values for [Message.Toggle]: which preset toggle
// position(s) a message fires in. Source: the official 0x70 SysEx
// API glossary ("toggle types"), confirmed against the editor's
// message UI (onMessageToggleMenuSelect at editor.js:81291-81298
// binds Both Positions=2, Position 1=0, Position 2=1, Shift=3).
const (
	TogglePos1     = 0
	TogglePos2     = 1
	TogglePosBoth  = 2 // the default for new messages
	TogglePosShift = 3
)

// TogglePosNames maps [Message.Toggle] values to display names.
var TogglePosNames = map[int]string{
	TogglePos1:     "Position 1",
	TogglePos2:     "Position 2",
	TogglePosBoth:  "Both Positions",
	TogglePosShift: "Shift",
}
