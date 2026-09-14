<p align="center"><img src="docs/icone.png" width="96" alt=""></p>

# Bifrost

A lightweight panel for the **Turing / UsbMonitor 3.5"** USB screen, replacing the official app:
shows the **music playing**, the **game open on Steam**, **PC usage** and a **clock**,
with a control panel so you can choose what shows up and how.

A single binary, written in Go. No need to install Python or anything else.
Runs on **Windows**, **Linux** and **macOS** — see [Platforms](#platforms) for what differs between them.

## Screenshots

<table>
<tr>
<td align="center" width="25%"><img src="docs/preview_jogo.png" width="200" alt="Game screen, open on Steam"><br><sub><b>Game</b> (Steam)</sub></td>
<td align="center" width="25%"><img src="docs/preview_musica.png" width="200" alt="Music playing screen"><br><sub><b>Music</b></sub></td>
<td align="center" width="25%"><img src="docs/preview_sistema.png" width="200" alt="System usage screen"><br><sub><b>System</b></sub></td>
<td align="center" width="25%"><img src="docs/preview_relogio.png" width="200" alt="Clock screen"><br><sub><b>Clock</b></sub></td>
</tr>
</table>

<p align="center"><img src="docs/preview_paisagem.png" width="440" alt="Game screen in landscape mode"></p>
<p align="center"><sub>Portrait, flipped portrait, landscape or flipped landscape — orientation is adjustable from the panel.</sub></p>

<p align="center"><img src="docs/painel.png" width="720" alt="Bifrost control panel"></p>

## What it does

- **Music**: title, artist, album, cover art and progress bar for whatever player shows up
  in Windows' media controls (Spotify, YouTube in the browser, Deezer, Media Player…). No login required.
- **Game (Steam)**: name, cover art, total playtime, last 2 weeks and current session time,
  read directly from the Steam install on your PC, no API key needed.
- **Live match tracking** (optional): while you're in an active match, the Game screen shows
  live stats instead of the usual Steam summary — **CS2/CS:GO and Dota 2** via Valve's official
  **Game State Integration**, **League of Legends** via Riot's local **Live Client Data API**,
  **Valorant** via the Riot Client's unofficial local API, **F1 22+** via the official **UDP
  telemetry** protocol, **Euro Truck Simulator 2 / American Truck Simulator** via the official
  **SCS Telemetry** plugin's shared memory, **World of Warcraft** via the game's own combat
  log file, and **Assetto Corsa / Assetto Corsa Competizione (ACC)** via Kunos' official shared
  memory. With no live match, it falls back to your Steam totals plus real-time **FPS** (read
  from RivaTuner Statistics Server, if installed). See [Live match tracking](#live-match-tracking) below.
- **System**: CPU, GPU, memory, disk, network, and how long the PC has been on.
- **Clock**: time and date, in 12- or 24-hour format, with or without seconds.
- **Mancer Mystic G1 watercooler display** (optional): sends the live CPU temperature to the small 2-digit HID
  display built into the Mancer Mystic G1 water block, auto-detected over USB (VID `0xAA88` / PID `0x8666`) —
  no port to pick, just a toggle. See [Mancer Mystic G1 display](#mancer-mystic-g1-display) below.
- **Multiple screens**: connect more than one USB screen and configure each independently —
  different orientation, brightness, and switching mode per screen (e.g. one fixed on System,
  another rotating through Music/Game). New screens are auto-detected; see
  [Multiple screens](#multiple-screens) for details and limitations.
- **Control panel**, to choose:
  - which screens show up and in what order;
  - how they switch: automatic (shows whatever is happening), rotation, or a fixed screen;
  - what each screen displays;
  - colors, background, orientation (portrait or landscape) and brightness, with a live preview.
- **Tray icon** near the clock: opens the panel, switches screens, pauses, reconnects.
- **Resolves conflicts with the official app**: if UsbMonitor is holding the port,
  the panel shows what it is and offers a button to close it.
- Reconnects on its own if the USB cable is unplugged, and only sends the part of the image that changed.

## Platforms

| Feature | Windows | Linux | macOS |
|---|---|---|---|
| Screen (USB serial) | ✅ | ✅ | ✅ |
| Panel (web UI) | ✅ embedded window | ✅ opens in the browser | ✅ opens in the browser |
| Tray icon | ✅ | ✅ | ✅ |
| System stats (CPU/GPU/RAM/disk/temps) | ✅ CPU/GPU temps need a monitoring tool already installed (see below) | ✅ CPU/GPU temps depend on `lm-sensors`/`nvidia-smi` being available | ✅ CPU % is an approximation (no CGO); no temperatures |
| Now playing | ✅ Windows Media Controls (any player) | ✅ MPRIS (Spotify, VLC, browsers, etc.) | ⚠️ Music.app and Spotify only, via AppleScript (no cover art) |
| Steam — local detection | ✅ registry + local files | ❌ use **Steam Web API** instead (Steam tab) | ❌ use **Steam Web API** instead (Steam tab) |
| Steam — Web API | ✅ | ✅ | ✅ |

Windows doesn't expose CPU/GPU temperatures through a public API, so Bifrost tries several sources
already running on your PC, cheapest first, and uses whichever answers: **HWiNFO**, **PawnIO** (used by
LibreHardwareMonitor 0.9.5+, HWiNFO, Fan Control — Bifrost only *reads* from it if one of those already
installed the driver; it never installs anything itself), **LibreHardwareMonitor**'s web server or WMI,
**MSI Afterburner** and **AIDA64**. If none of them are available, temperature just doesn't show (run
`bifrost --diagnostico` to see exactly which sources answered).
| Autostart with the system | ✅ registry | ✅ XDG autostart (`~/.config/autostart`) | ✅ LaunchAgent (`~/Library/LaunchAgents`) |
| Conflict detection (official app) | ✅ | n/a (Windows-only official app) | n/a (Windows-only official app) |
| Mancer Mystic G1 watercooler display | ✅ SetupAPI + HidD_*/HidP_* (no CGO) | ✅ `/dev/hidraw*` (no CGO, may need a udev rule) | ❌ would require IOKit/CGO |

On Linux/macOS there's no embedded window, so the panel opens in your default browser instead
(still only reachable from `127.0.0.1`).

## Multiple screens

Add as many screens as you have in the **Connection** tab. Each one gets its own port, model,
orientation, brightness, and switching mode — so you can, for example, pin one screen to always
show System stats and let another rotate through Music/Game.

A screen's **Port** can be left as **Automatic**, in which case Bifrost claims any detected,
unclaimed screen for it, or pinned to a specific port if you want a stable, predictable mapping.

> **Known limitation**: the cheap Turing/UsbMonitor clones typically report the same hardware
> VID/PID/serial number for every unit of the same model, so Bifrost can't tell two identical
> physical screens apart by hardware identity alone — only by which port they're plugged into.
> If you have two or more identical screens, pin each one to a specific port (once you've
> figured out which port is which, e.g. by flashing the brightness) for a mapping that survives
> reboots; leaving them all on "Automatic" still works, but which physical screen ends up as
> "which" device can shuffle between runs.


## Languages

Bifrost's panel and screen also speak **English**, **Português**, **Español**, **日本語 (Japanese)** and **中文 (Mandarin)**.
By default it auto-detects the language configured in Windows; you can also pick one manually in the **General** tab of the panel.

> Note: the physical LCD screen uses an embedded Latin font (Roboto), which doesn't include Japanese/Chinese glyphs.
> When Japanese or Mandarin is selected, the **physical screen** falls back to English, while the **web panel** and the
> **tray icon** are fully translated.

## How to use

1. Download the binary for your OS from **Releases** (or build it yourself, see below) and put it in a folder.
2. Open it. The panel opens (an embedded window on Windows, your browser on Linux/macOS)
   and the rainbow icon appears in the tray.
3. If the panel warns that **another program is using the screen** (Windows only, official app),
   check the official app in the list and click **Close selected and connect**. Windows will ask
   for administrator permission, because the official app runs as administrator. After that,
   disable the official app's autostart in Task Manager, under the *Startup apps* tab.
4. Done. Open a game through Steam and it shows up on the screen within seconds.

Closing the panel window **does not** turn off the screen. Bifrost keeps running from the tray
icon. To quit for good: right-click the icon → **Quit** (or Ctrl+C in the terminal if the tray
icon isn't available on your desktop environment).

### Steam

By default, Bifrost reads the game **directly from the Steam app installed on your PC**. It uses:

- the Windows registry, which reports the open game;
- library manifests, for the game's name;
- `localconfig.vdf`, for playtime and account;
- Steam's image cache, for the cover art.

No API key needed, doesn't depend on profile privacy, works offline, and detects
the game in about 2 seconds.

If you play on **another PC**, switch the source to **Steam Web API** in the Steam tab of the panel:

1. Click **Generate my key** and create a Steam Web API key (for "Domain Name" you can use `localhost`).
2. Click **Find my ID** to get your SteamID64 (17 digits).
3. In Steam's privacy settings, set **Game details** to **Public**.
4. Click **Test**.

### Live match tracking

<p align="center"><img src="docs/preview_jogo_ao_vivo.png" width="200" alt="Game screen showing a live CS2 match"></p>
<p align="center"><sub>While a match is on, the Game screen swaps the usual Steam summary for live status: map/round, score, K/D/A, health, armor, money, bomb state, and CPU/GPU/FPS.</sub></p>

The **Steam** tab has a **Live matches** toggle (on by default). While it's on:

- **CS2 / CS:GO and Dota 2**: Bifrost writes a small `.cfg` file into the game's own `cfg` folder
  (auto-detected from your Steam libraries), using Valve's official **Game State Integration**
  protocol. The game then POSTs a JSON update to a local port (`127.0.0.1:47018`) on every kill,
  round change, bomb plant, etc. — nothing is injected into the game, and no memory is read.
  If the game was already open when you turned this on, restart it once so it picks up the new
  `.cfg` file. Docs: [CS2/CS:GO GSI](https://developer.valvesoftware.com/wiki/Counter-Strike:_Global_Offensive_Game_State_Integration),
  [Dota 2 GSI](https://developer.valvesoftware.com/wiki/Dota_2_Workshop_Tools/Game_State_Integration).
  **Dota 2 also needs `-gamestateintegration` added to its Steam launch options**
  (Library → right-click Dota 2 → Properties → Launch Options) — without it, Dota 2 won't read
  the `.cfg` file at all, no matter how many times it's restarted. CS2 doesn't need this.
- **League of Legends**: read from Riot's own local **Live Client Data API**
  (`https://127.0.0.1:2999/liveclientdata/allgamedata`), which the League client exposes by itself
  while a match is in progress — no setup needed.
- **Valorant**: reads the same unofficial local Riot Client API chain that community overlays use
  (there's no official Riot API for Valorant like there is for LoL). Only shows map, mode and the
  chosen agent — no live kills/health/money, since the API doesn't expose those.
- **F1 22 and newer**: reads the official **UDP Telemetry** broadcast — turn it on in the game's
  own menu (Settings → Telemetry → UDP on, IP `127.0.0.1`, port `20777`, format matching the
  game's year) and Bifrost picks it up automatically. Shows speed, gear, RPM, position, current
  lap and track/session.
- **Euro Truck Simulator 2 / American Truck Simulator**: reads the shared memory exposed by the
  free, community-made [SCS Telemetry plugin](https://github.com/RenCloud/scs-sdk-plugin) — copy
  its DLL into the game's `plugins` folder once. Shows speed, gear, RPM and fuel. Windows only.
- **World of Warcraft**: reads `WoWCombatLog.txt`, the file the game itself writes to disk once
  combat logging is turned on with `/combatlog` (or automatically by addons like Details!) — the
  same file Details!, Recount and the Warcraft Logs uploader use. Only shows data during an
  encounter (boss) fight: zone, encounter name/difficulty and fight duration — the log doesn't
  expose your own damage/healing without knowing your character name. Bifrost auto-detects the
  default Battle.net install path (retail/Classic/Classic Era).
- **Assetto Corsa / Assetto Corsa Competizione (ACC)**: reads the official **shared memory**
  (`Local\acpmf_physics`/`acpmf_graphics`/`acpmf_static`) that both games expose on their own —
  no plugin or setup needed, just have the game open with the car on track. Shows speed, gear,
  RPM, track, session type, lap and position. Windows only.
- **FPS**: while no live match is detected, the Game screen also shows real-time FPS if
  [RivaTuner Statistics Server](https://www.guru3d.com/download/rtss-rivatuner-statistics-server-download/)
  is installed and running (read from its shared memory, the same source every FPS overlay uses).

While a match is live, the Game screen replaces the Steam summary with the match's actual status:
map/hero/track, round/lap/game time, score or position, and per-game extras (health/armor/money and
bomb state for CS2; gold/XP per minute and CS for Dota 2/LoL; speed/gear/RPM for F1 and ETS2/ATS;
zone/encounter for WoW) — plus CPU, GPU and FPS at the bottom, so you can keep an eye on
performance without tabbing out.

When there's no live match, the Game screen just shows your Steam totals as before.

### Mancer Mystic G1 display

If you have a **Mancer Mystic G1** water block, its small 2-digit HID display (normally just showing "88")
can be fed the live CPU temperature. **It's auto-detected over USB** by its VID/PID (`0xAA88` / `0x8666`) —
plug it in and it shows up on its own in the **Connection** tab, no setup needed. If you'd rather not use it,
click **Remove** on its card (you can add it back later from the **Add device** section). It updates twice
a second and shows "88" again if Bifrost stops or the connection drops.

This is an independent feature (not one of the rotating screens above) based on the reverse-engineered
protocol from [dsmlucas/mancer-g1-cpu-temp-display](https://github.com/dsmlucas/mancer-g1-cpu-temp-display)
(MIT licensed): a single HID output report byte, 0–99, equal to the temperature in Celsius.

- **Windows**: implemented with the native SetupAPI/HidD_*/HidP_* Win32 APIs (no CGO), the same way
  Device Manager enumerates HID devices under the hood.
- **Linux**: implemented via `/dev/hidraw*` (no CGO). If you get a permission error, add a udev rule
  granting your user access to the device, similar to the reference project's `99-mancer-watercooler.rules`.
- **macOS**: not supported — would require IOKit's HID Manager, which needs CGO.

## Where data is stored

`%APPDATA%\Bifrost\` on Windows, `~/.config/Bifrost/` on Linux, `~/Library/Application Support/Bifrost/`
on macOS. Holds `config.json`, `bifrost.log` and the cover art cache.

**Portable mode**: if a `dados` folder exists next to the binary, everything is saved there instead.

**Diagnostics**: `bifrost --diagnostico` (`bifrost.exe` on Windows) generates a `diagnostico.txt` with
serial ports, conflicting programs, the music playing, and system readings. Useful when opening an issue.

## Building

Requires [Go 1.24+](https://go.dev/dl/).

On Windows, for a quick local build (`dist\bifrost.exe`):

```bat
build.bat 1.0.0
```

For Windows + Linux + macOS binaries (amd64 and arm64), from any OS with Go installed,
no CGO or cross-toolchain needed:

```sh
./build.sh 1.0.0
```

This writes `dist/bifrost-windows-amd64.exe`, `dist/bifrost-linux-{amd64,arm64}` and
`dist/bifrost-darwin-{amd64,arm64}`. To run the tests: `go test ./...`

## Structure

```
cmd/bifrost/        program entry point, tray icon, panel window, diagnostics
internal/acctelemetry/  Assetto Corsa/ACC live data (Kunos' official shared memory)
internal/app/       decides the current screen, keeps the connection, sends the frames
internal/ets2telemetry/  Euro Truck Simulator 2/American Truck Simulator live data (SCS Telemetry plugin's shared memory)
internal/f1telemetry/    F1 22+ live data (official UDP telemetry)
internal/gsi/       Game State Integration server (CS2/CS:GO and Dota 2 live match data)
internal/i18n/      translation catalog and OS UI-language detection
internal/lcd/       screen protocol (revision A) and the serial port (Windows/Linux/macOS)
internal/lolapi/    League of Legends live match data (Riot's local Live Client Data API)
internal/render/    screen drawing (portrait and landscape)
internal/valorantapi/    Valorant live match data (Riot Client's unofficial local API)
internal/wowcombatlog/   World of Warcraft live encounter data (the game's own combat log file)
internal/media/     music playing — Windows Media Control (WinRT), MPRIS/D-Bus on Linux, AppleScript (Music.app/Spotify) on macOS
internal/rtss/      real-time FPS, read from RivaTuner Statistics Server's shared memory
internal/steam/     Steam: local reading (Windows registry + .vdf files) and Web API (all platforms)
internal/sysinfo/   CPU, GPU, memory, network and disk per platform
internal/web/       control panel (local server + UI)
internal/winutil/   processes, autostart, single instance, per platform
internal/update/    checks GitHub Releases for new versions and installs them
internal/config/    config.json
tools/genres/       generates the Windows .exe's icon, manifest and version
```

The panel is only served on `127.0.0.1` (not visible on the network) and rejects requests coming from other sites.

## Updating

Bifrost checks GitHub Releases for a newer version on startup and every 6 hours
(toggle in the **General** tab). When one is found, the panel shows a banner
with an **Update and restart** button — it downloads the right binary for your
OS/arch, verifies its SHA-256 against the release's `checksums.txt` when
available, replaces the running binary, and restarts. No data is sent anywhere
other than the public, unauthenticated GitHub API/CDN.

## Troubleshooting

| Problem | What to do |
|---|---|
| "Another program is using the screen" | Use the panel's button to close UsbMonitor, or close it from Task Manager. |
| "Screen not found" | Check the cable. In the **Connection** tab, see if a port is marked as "is the screen". |
| Odd colors or orientation | Adjust **Orientation** in the Appearance tab. |
| Music doesn't show up | The player needs to show up in Windows' media mini-player (the media keys). |
| Game doesn't show up | Click **Test** in the Steam tab. Games opened outside of Steam aren't detected. On the Web API, check the "Game details" privacy setting. |
| The screen gets hot | Lower the brightness. These screens get hot above ~50%. |

## For hardware manufacturers

If you make a USB screen, watercooler display, or similar peripheral and would
like it properly integrated into Bifrost (official protocol documentation,
sample hardware, or just a conversation about what's needed), reach out at
**ricardo@satty.com.br**.

## Contributing

Bug reports, feature requests and PRs are welcome — see
[CONTRIBUTING.md](CONTRIBUTING.md) for how to set up a dev environment and the
project's conventions. Found a security vulnerability? Please don't open a
public issue — see [SECURITY.md](SECURITY.md) instead.

## License

GPL-3.0-or-later. The screen protocol was ported from
[turing-smart-screen-python](https://github.com/mathoudebine/turing-smart-screen-python).
Full credits in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

GPL-3.0 already guarantees you the right to use Bifrost commercially — that can't be taken away
by this project. If you build a product or service on top of it, we'd appreciate a credit/link
back to this repository and its author; that's a courtesy request, not a license condition.
