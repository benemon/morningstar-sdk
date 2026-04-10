package sysex

// Command constants for the Morningstar MC8 Pro protocol. These come
// from the `bt` enum at editor.js:15352 and from observation of the
// live wire format. See ../../CLAUDE.md for the full reference.
//
// All commands below are used as (Cmd1, Cmd2) pairs. For most requests
// Cmd1 = 0; distinct Cmd1 values (4, 6, 7) identify entirely different
// command families (data upload, live edit writes, backup/restore).
//
// Within the Cmd1 = 0 family, these are the named values. Requests all
// use ARG1..ARG4 = 0 unless otherwise noted.
// The editor's onClickDisconnectDevice handler sends (0, 1) but that
// alone does NOT exit edit mode on the device. The real exit is
// editor.js:90264 which sends (0, 28) via disconnectEditorMode().
// (0, 1) appears to be a legacy or no-op command that happens to be
// sent from a different disconnect path. Always use CmdSessionClose
// (= 28) to cleanly exit edit mode.
const (
	// Session control.
	CmdSessionOpen  = 27 // editor.js:90425  onClickConnectDevice → startTransmission flow
	CmdSessionClose = 28 // editor.js:90264  disconnectEditorMode() — the REAL exit-edit-mode command
	CmdSessionZero  = 0  // alt session-open used by the MC3 code path
	CmdLegacyDisco  = 1  // editor.js:95018  sent in onClickDisconnectDevice but insufficient on its own

	// REQUEST_* — read-side commands. The device replies with one or
	// more frames carrying the requested data.
	CmdReqEngagePreset               = 29
	CmdReqEngageExp                  = 30
	CmdReqControllerSettingsAll      = 35
	CmdReqControllerGeneralConfig    = 36
	CmdReqWaveformEngine             = 37
	CmdReqSequencerEngine            = 38
	CmdReqScrollSlots                = 39
	CmdReqMidiChannelNames           = 40
	CmdReqBankArrangement            = 41
	CmdReqOmniportData               = 42
	CmdReqBankPresetNames            = 43
	CmdReqControllerFirmwareVersion  = 44
	CmdReqEventProcessor             = 45
	CmdReqControllerUUID             = 46
	CmdToggleLooperMode              = 47
	CmdReqPresetNames                = 64
	CmdReqExpressionCalibration      = 65
	CmdReqResistorLadderCalibration  = 66
	CmdReqMidiClockSlots             = 80

	// Handshake.
	CmdPing         = 125 // sendPing()
	CmdRetryRequest = 126 // sendRetryRequest()
	CmdAck          = 127 // sendAcknowedgeSysex()
)

// Cmd1 family values. Used as the first byte of the command selector.
const (
	Cmd1General = 0x00 // most editor commands (including all REQUEST_*)
	Cmd1Upload  = 0x04 // startTransmission/endTransmission envelope
	Cmd1Write   = 0x06 // live preset/bank writes
	Cmd1Backup  = 0x07 // backup/restore traffic
)

// Cmd2 values for the write family (Cmd1 = 0x06). These are the
// editor's live-edit write commands, confirmed from editor.js:85910
// (sendFullPresetData) and editor.js:90760 (sendFullBankData).
//
// Writes are fire-and-forget: the editor sends a bare frame with no
// startTransmission/endTransmission envelope and does NOT wait for an
// acknowledgment from the device. The args carry bank/preset/isExp
// addressing; the payload carries the encoded data.
const (
	CmdWritePreset = 0x11 // sendSysex6(6, 17, bank, preset, isExp, 0, data)
	CmdWriteBank   = 0x12 // sendSysex6(6, 18, bank, 0, 0, 0, data)
)

// Cmd2 values for the upload envelope (Cmd1 = 0x04). Used only for
// specific operations like bank-rearrange, NOT for regular preset or
// bank writes.
const (
	CmdUploadStart = 0x00 // editor.js startTransmission()
	CmdUploadEnd   = 0x01 // editor.js endTransmission()
)

// Cmd2 values for the backup family (Cmd1 = 0x07). These serve dual
// purpose: requesting a backup (editor→device) and receiving the
// backup data stream (device→editor).
const (
	CmdBackupHeader   = 0x00 // Used both as request trigger and response header
	CmdBackupPreset   = 0x01 // 1032-byte per-preset frame (one per preset slot)
	CmdBackupBankMeta = 0x02 // 647-byte bank-level metadata frame
)

// CmdBackupRequest arg values for requesting bank/device dumps
// via cmd (07, 00, arg). Discovered from editor.js:55546 requestData().
const (
	CmdBackupRequestSingleBank = 50 // 0x32 — dump current bank (MC8 Pro: 37 frames)
	CmdBackupRequestAllBanks   = 51 // 0x33 — dump all banks + controller settings
)

// NoArgs is a convenience value for commands that take no arguments.
var NoArgs = [4]byte{0, 0, 0, 0}
