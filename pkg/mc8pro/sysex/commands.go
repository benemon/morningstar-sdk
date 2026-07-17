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
	CmdReqEngagePreset              = 29 // remote-trigger a preset press
	CmdReqEngageExp                 = 30 // remote-trigger an expression engage
	CmdSwapPreset                   = 24 // swap a main preset slot (device-side)
	CmdSwapExpPreset                = 25 // swap an expression preset slot (device-side)
	CmdReqControllerSettingsAll     = 35
	CmdReqControllerGeneralConfig   = 36
	CmdReqWaveformEngine            = 37
	CmdReqSequencerEngine           = 38
	CmdReqScrollSlots               = 39
	CmdReqMidiChannelNames          = 40
	CmdReqBankArrangement           = 41
	CmdReqOmniportData              = 42
	CmdReqBankPresetNames           = 43
	CmdReqControllerFirmwareVersion = 44
	CmdReqEventProcessor            = 45
	CmdReqControllerUUID            = 46
	CmdToggleLooperMode             = 47
	CmdReqPresetNames               = 64
	CmdReqExpressionCalibration     = 65
	CmdReqResistorLadderCalibration = 66
	CmdReqMidiClockSlots            = 80

	// Handshake.
	CmdPing         = 125 // sendPing()
	CmdRetryRequest = 126 // sendRetryRequest()
	CmdAck          = 127 // sendAcknowedgeSysex()
)

// Cmd1 family values. Used as the first byte of the command selector.
const (
	Cmd1General  = 0x00 // most editor commands (including all REQUEST_*)
	Cmd1Upload   = 0x04 // startTransmission/endTransmission envelope
	Cmd1Write    = 0x06 // live preset/bank writes
	Cmd1Backup   = 0x07 // backup/restore traffic
	Cmd1External = 0x70 // official external-application API (documented)
)

// Cmd2 (op2) values for the official external-application API
// (Cmd1 = 0x70). Source: reference/sysex-api-external-applications.pdf
// — Morningstar's DOCUMENTED third-party API, sharing the standard
// frame structure. The op3..op6 function arguments occupy the normal
// Args slots; op7 (byte 12) is 0 for every documented function, and
// the transaction ID (byte 13) may be left 0, so Build produces
// spec-compliant frames as-is.
//
// Most write functions take a save flag: ExtSave commits to flash,
// any other value applies a TEMPORARY override that reverts when the
// bank changes.
const (
	ExtControllerFunc    = 0x00 // op3: 0=bank up, 1=bank down, 2=toggle page
	ExtSetPresetShort    = 0x01 // op3=preset, op4=save; payload = padded ASCII
	ExtSetPresetToggle   = 0x02 // op3=preset, op4=save; payload = padded ASCII
	ExtSetPresetLong     = 0x03 // op3=preset, op4=save; payload = padded ASCII
	ExtSetPresetMessage  = 0x04 // op3=preset, op4=slot 0-15, op5=type (PC/CC), op6=save
	ExtSetPresetOther    = 0x05 // op3=preset, op4=slot?, op6=save; payload = flags+colors
	ExtSetBankName       = 0x10 // op4=save; payload = padded ASCII (current bank)
	ExtDisplayMessage    = 0x11 // op4=duration in 100ms units; payload ≤ 20 ASCII chars
	ExtGetPresetShort    = 0x21 // op3=preset; reply payload = name
	ExtGetPresetToggle   = 0x22 // op3=preset; reply payload = name
	ExtGetPresetLong     = 0x23 // op3=preset; reply payload = name
	ExtGetBankName       = 0x30 // reply payload = current bank name
	ExtGetToggleStates   = 0x31 // reply payload = one byte per preset, 0x7F = toggled
	ExtGetControllerInfo = 0x32 // reply payload = model, fw[4], msgs/preset, name sizes
	ExtError             = 0x7F // device error reply; ack code in op3

	// ExtSave is the op-argument value that commits an external-API
	// write to flash. Any other value = temporary override.
	ExtSave = 0x7F

	// ExtColorKeep leaves a color field unchanged in an
	// ExtSetPresetOther frame. (The PDF prints this as "F7", which
	// cannot be a SysEx data byte — 0xF7 terminates the frame — so
	// the intended 7-bit value is 0x7F.)
	ExtColorKeep = 0x7F
)

// Ack codes carried in op3 of an ExtError reply.
const (
	ExtAckSuccess          = 0x00
	ExtAckWrongModelID     = 0x01
	ExtAckWrongChecksum    = 0x02
	ExtAckWrongPayloadSize = 0x03
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

// Cmd2 values for the upload envelope (Cmd1 = 0x04). Used for
// controller-settings writes. Each write is wrapped in
// startTransmission / endTransmission.
const (
	CmdUploadStart           = 0x00 // editor.js startTransmission()
	CmdUploadEnd             = 0x01 // editor.js endTransmission()
	CmdUploadControllerCfg   = 0x02 // sendSysex(4, 2, data)
	CmdUploadMidiChannels    = 0x03 // sysexBuilder(4, 3) — row-framed
	CmdUploadBankArrange     = 0x04 // sendSysex4(4, 4, 0, 0, data)
	CmdUploadWaveform        = 0x05 // sendSysex4(4, 5, 0, 0, data)
	CmdUploadSequencer       = 0x06 // sendSysex4(4, 6, 0, 0, data)
	CmdUploadScrollCounters  = 0x07 // sendSysex4(4, 7, 0, 0, data)
	CmdUploadOmniports       = 0x08 // sendSysex(4, 8, data)
	CmdUploadPresetRearrange = 0x09 // sendSysex4(4, 9, 0, 0, data)
	CmdUploadEventProcessor  = 0x0A // sendSysex4(4, 10, 0, 0, data)
	CmdUploadResistorLadder  = 0x0B // sendSysex(4, 11, data)
	CmdUploadMidiClockSlots  = 0x0C // sendSysex(4, 12, data)
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

// Restore protocol commands (Cmd1 = 0x07). The restore is the
// inverse of the backup dump: the editor sends frames one at a
// time, and the device ACKs each with (07, 00, arg=33) before
// requesting the next. Discovered from editor.js:55888-55893
// and the hasNewMessage handler at editor.js:55909-55957.
const (
	// CmdRestoreStartSingle tells the device to begin a single-bank
	// restore. The device responds with (07, 00, arg=33) when ready
	// for the first frame. editor.js:55889.
	CmdRestoreStartSingle = 48 // 0x30 — sendSysexFunction(7, 0, 48, 0)

	// CmdRestoreStartAll tells the device to begin an all-banks
	// restore. Same handshake as single. editor.js:55892.
	CmdRestoreStartAll = 48 // same cmd, arg[1]=1 distinguishes

	// CmdRestoreComplete signals that all frames have been sent.
	// The device responds with (07, 00, arg=17) on success or
	// (07, 00, arg=3) on failure. editor.js:55884.
	CmdRestoreComplete = 49 // 0x31 — sendSysexFunction(7, 0, 49, 0)

	// CmdRestoreReadyForNext is the device's ACK: "send the next
	// frame." Received as (07, 00, arg=33). editor.js:55951.
	CmdRestoreReadyForNext = 33 // 0x21 — arg byte in (07, 00) response

	// CmdRestoreDone is the device's final ACK: "upload complete."
	// Received as (07, 00, arg=17). editor.js:55948.
	CmdRestoreDone = 17 // 0x11 — arg byte in (07, 00) response

	// CmdRestoreFailed is the device's error: "upload failed."
	// Received as (07, 00, arg=3). editor.js:55940.
	CmdRestoreFailed = 3 // 0x03 — arg byte in (07, 00) response

	// Cmd2 values for restore data frames (Cmd1 = 0x07).
	// These mirror the backup Cmd2 values but with different
	// numbers for the upload direction.
	CmdRestoreBankMeta  = 0x10 // sendSysex(7, 16, data)     — editor.js:55671
	CmdRestorePreset    = 0x11 // sendSysex4(7, 17, n, 0, d) — editor.js:55674
	CmdRestoreExpPreset = 0x12 // sendSysex4(7, 18, n, 0, d) — editor.js:55677
)

// NoArgs is a convenience value for commands that take no arguments.
var NoArgs = [4]byte{0, 0, 0, 0}
