# Kalkan Aura LCD — device identification and display protocol

Findings from a static analysis of the vendor software, for the purpose of adding
support to [Bifrost](https://github.com/satty-br/Bifrost-screen). No hardware was
available; everything below is read out of the shipped binaries, so the parts marked
**unconfirmed** need one person with the cooler to verify.

## TL;DR

The Kalkan Aura LCD is a **rebadged GAMDIAS panel** driven by **GAMDIAS ZeusCast**.
It is a **USB HID** device — `VID 0x1B80`, `PID 0xB550` — that takes an
**HTTP-shaped text protocol over 64-byte HID reports**, with JSON bodies and JPEG
frames. There is no proprietary binary framing to reverse: the wire format is
readable ASCII.

This is well within what Bifrost already does for the Mancer Mystic G1 (native
SetupAPI/HidD_*, no CGO), and the renderer already produces the frames.

## What was analysed

| File | What it is |
|---|---|
| `Kalkan KK Aura Software 3.2.1.6.exe` | Inno Setup 6.5.2, 306 MB. Installs `KK.exe` — a .NET 4.8 WPF app, 12.5 MB |
| `Kalkan KK AURA Display Digital V2.7.exe` | Inno Setup 6.4.0.1, 54 MB. Installs `XKWHardwareServices` (the *digital*, non-LCD product) |

Both come from `Kalkan KK Aura Software + Driver.zip` on Kalkan's download server.

`KK.exe` bundles `HidLibrary.dll`, `HidSharp.dll`, `LibreHardwareMonitorLib.dll`,
`Magick.NET`, `OpenCvSharp`, `LibVLCSharp`, `ffmpeg.exe` and `yt-dlp.exe`. It is not
obfuscated: type names, user strings and IL constants are all readable.

### Method (reproducible)

1. `innoextract` (git master) does not read Inno Setup 6.4/6.5 — its setup loader now
   uses **revision 2** with **64-bit offsets**. Patching `loader/offsets.cpp` to read
   `u64 total / u64 exeOffset / u32 exeUncompressedSize / u32 exeCrc / u64 headerOffset /
   u64 dataOffset / u32 pad` for `revision == 2` makes 6.4.0.1 extract cleanly.
2. 6.5.2 additionally inserts **49 bytes** between the 64-byte version string and the
   first compressed block. After skipping them, the setup header is a plain LZMA1 stream
   in 4096-byte CRC32-prefixed chunks, and the data section is a single `zlb\x1a` chunk
   (LZMA1, `lc=3 lp=0 pb=2`, 8 MB dictionary) that decompresses to 658 MB.
3. PE files were then carved out of that blob by walking section tables, and the .NET
   metadata heaps (`#~`, `#US`, `#Strings`) parsed directly.

## Device identification

**Vendor ID `0x1B80` for every LCD and digital display in this family.** Product IDs are
carried through the code as 4-hex-digit strings (`"B550"`), and the app matches devices by
HID device path — `\\?\hid#vid_1b80&pid_b550&mi_xx...` — so the panel is one interface of
a **composite** device.

The product table (`LcdProductTable` / `LcdProductDefinition`) maps each PID to a marketing
name, an OEM name and a native resolution. The relevant rows:

| PID | GAMDIAS name | OEM name in the Kalkan build | Resolution | FPS |
|---|---|---|---|---|
| **`0xB550`** | **AURA Lite II LCD Display** | **KALKAN LCD Display** | **320 × 240** | **30** |
| `0xB54E` | CHIONE LCD 4 Display | AURA LITE LCD | 320 × 240 | 30 |
| `0xB547` | HeliosP2 | AURA LCD Display | 480 × 480 | 30 |
| `0xB551` | AI LCD Display | AURA KK LCD Display | 480 × 480 | 30 |

Other PIDs in the same family, not yet mapped: `B522 B526 B533 B534 B538 B53A B53B B53C
B53D B540 B541 B542 B548 B54B B54C B54D B54F`.

> **Unconfirmed:** that the retail *Kalkan Aura LCD 240 (KLK00102)* and *360 (KLK00103)*
> are `0xB550`. The evidence is strong — `0xB550` is the only PID whose OEM string is
> literally `KALKAN LCD Display`, its update endpoint is
> `gamdias.com/Software/ZeusCast/Oem/KK/…`, and 320×240 matches the advertised 2.4" panel —
> but `0xB54E` is the same panel at the same resolution, so only a `Get-PnpDevice` on a real
> unit settles it.

## Transport

- **USB HID.** The app logs `HidTransport64X Open` and reports
  `InputReportByteLength` / `OutputReportByteLength` / `FeatureReportByteLength`.
- **64-byte reports.** Confirmed as a literal `0x40` in the device-construction IL for the
  digital siblings; the `64X` in the transport class name says the same for the LCD.
- The device is opened by enumerating HID paths and string-matching
  `VID_` + `&PID_` + `&MI_` + `&Col`, and by reading
  `SYSTEM\CurrentControlSet\Enum\HID\` — i.e. the usual SetupAPI walk that Bifrost's
  Mancer code already performs.

## Wire protocol

Requests are **HTTP-like plain text**, split across 64-byte output reports:

```
POST <field> <value>\r\n
SeqNumber=<n>\r\n
ContentType=json\r\n
ContentLength=<len>\r\n
\r\n
<JSON body>
```

State pushes use `STATE` instead of `POST`. The device answers `200\r\n` on success and
`400\r\n` on failure, read back through input reports.

### Command surface

Every command seen in `ProtocolCommands`:

| Request line | Handler | Purpose |
|---|---|---|
| `POST conn\r\n\r\n` | `REQUEST_Conn_POST` | open/handshake (no body) |
| `POST power 1` | `REQUEST_Power_POST` | panel on/off |
| `POST brightness 1` | `REQUEST_Brightness_POST` | backlight |
| `POST rotate 1` | `REQUEST_Rotate_POST` | orientation |
| `POST mode 1` | `REQUEST_Mode_POST` | display mode |
| `POST logo 1` | `REQUEST_Logo_POST` | boot logo |
| `POST presetThemeId 1` | `REQUEST_PresetThemeId_POST` | built-in theme |
| `POST osdState 1` | `REQUEST_OSDState_POST` | OSD overlay on/off |
| `POST realtimeDisplay 1` | `REQUEST_RealTimeDisplay_POST` | live-data mode |
| `POST sysinfoDisplay 1` | `REQUEST_SysInfoDisplay_POST` | firmware-rendered sysinfo |
| `POST timeout 1` | `REQUEST_Timeout_POST` | sleep timeout |
| `POST alarm 1` | `REQUEST_Alarm_POST` | temperature alarm |
| `POST log 1` | `REQUEST_Log_POST` | device log |
| `POST reboot 1` | `REQUEST_Reboot_POST` | reboot |
| `POST recovery 1` | `REQUEST_Recovery_POST` | factory reset |
| `POST transport 1` | `REQUEST_Transport_POST` | begin an image/video transfer |
| `POST transported 1` | — | end of transfer |
| `STATE heartbeat 1` | `REQUEST_Heartbeat_STATE` | keepalive |
| `STATE timestamp 1` | `REQUEST_Timestamp_STATE` | clock sync |

### Pushing a frame

`Process_REQUEST_Transport_POST_Step1/2/3` plus `SEND_TransportRealTimeDATA` show a
three-step transfer:

1. `POST transport` with a JSON header describing the payload. JSON keys observed:
   `file`, `format`, `width`, `height`, `data`, `blockMaxSize`.
2. The device answers the *FileHeader* — the app logs
   `(F1)Received FileHeader Respond Success` / `… fail` — and the payload is then sent in
   blocks of `blockMaxSize`.
3. `POST transported` closes the transfer; `(N)Received transport` confirms.

Frames are handled by `JpegFrameHandler` and `BitmapFrameHandler`, with
`FrameCallbackFormat` selecting between them, so the panel takes **JPEG** (and a raw
bitmap alternative) rather than a proprietary pixel format. `TransportMax={0}x{1}`
caps the accepted dimensions.

> **Unconfirmed, needs hardware:** the HID report ID, whether output reports are
> written with `HidD_SetOutputReport` or `WriteFile`, the exact JSON schema per command,
> the `blockMaxSize` the firmware reports, and the interface (`MI_`) index the panel lives on.
> All of these fall out of a single 30-second USB capture, or of a first connection attempt.

## The digital sibling (easier, and separate)

The *AURA DIGITAL* line uses a different stack: `TempComm.dll`, a native x64 DLL that also
speaks HID (`HidD_GetAttributes`, `HidD_GetHidGuid`) and exports a clean, self-describing
API — `DeviceOpen`, `DeviceClose`, `CheckOnlineDevice`, `GetOnlineDeviceQty`,
`SetCpuDynamicInfo`, `SetGpuDynamicInfo`, `SetCpuName`, `SetCpuWatt`, `SetCpuVoltage`,
`SetMemDynamicInfo`, `SetDiskDynamicInfo`, `SetNetSpeed`, `SetFanRPM`, `SetWaterFanRPM`,
`SetGpuFanRPM`, `SetOtherFanRPM`, `SetDisplayMode`, `SetDisplayName`, `SetDeviceTime`,
`SetColor`, `SetColorEx`, `SetOsInfo`.

That is much closer to the Mancer integration Bifrost already ships: a handful of HID
writes, no framebuffer. If the goal is "get *a* Kalkan cooler working with the least
risk", the digital model is the cheaper first target.

## What this means for Bifrost

Everything needed is already in the codebase:

- HID enumeration and I/O on Windows via SetupAPI / `HidD_*`, no CGO — the Mancer driver.
- Frame rendering — `internal/render` already draws full screens; 320 × 240 is just another
  geometry.
- JPEG encoding — Go's standard library.

The work is a new `internal/kalkan` package: enumerate `VID_1B80`, match the PID against a
product table, `POST conn`, then a 30 fps loop of `POST transport` → JPEG blocks →
`POST transported`, plus `STATE heartbeat`. Roughly the size of the Mancer package plus the
frame encoder.

**It cannot be finished without hardware.** The honest plan is to publish this document,
write the driver against it, ship it marked as untested, and ask for a volunteer with the
cooler to confirm the PID and the first handshake.

## Note on method

This documents a hardware interface for the purpose of interoperability, from the vendor's
own shipped binaries. No vendor code is copied into Bifrost; the implementation is written
from the protocol description above.

## Status in this repository

`internal/kalkan` implements the device table, the text protocol, the 64-byte report
chunking and the JPEG frame transfer, against the description above. `Display` adapts it to
the `lcd.Display` interface, so the panel is driven by exactly the same machinery as the
3.5" USB screens: same screens, same rotation, same preview, same status card.

It is **wired into the app and enabled by default**, but it costs nothing when no such
cooler is present: the app scans for `VID 0x1B80` over HID (cached, every 5s) and only
creates a synthetic device — id `kalkan`, never written to `config.json` — when a known
panel answers. Unplug it and the device disappears again.

The protocol layer is unit-tested against a fake transport, and the auto-detection against
an injected scanner. **No byte of it has ever reached a real panel.**

`tools/kalkanprobe` builds a standalone `kalkan-probe.exe` that enumerates the HID
devices, tries the handshake and can push a test frame. That is the instrument for
whoever has the hardware.

```
kalkan-probe                 list the panels it finds
kalkan-probe -todos          list every HID device on the PC
kalkan-probe -conectar       open the panel and send POST conn
kalkan-probe -imagem         push a colour-bar test frame
kalkan-probe -brilho 40      set brightness
kalkan-probe -log saida.txt  also write everything to a file
```
