# NA4WX Allstar Dashboard

A browser-based configuration tool for a [HamVoIP](http://hamvoip.org/) AllStarLink/Asterisk node — edit `rpt.conf`, `iax.conf`, `extensions.conf`, `usbradio.conf`/`simpleusb.conf`, and system/network settings without SSH or a text editor.

Runs as a single self-contained binary directly on the Pi. No Python/Node runtime to install, no database, no build step for the frontend.

## Get Started

On the Pi (as root, or with `sudo`):

```sh
git clone https://github.com/na4wx/na4wx_allstar_dashboard.git
cd na4wx_allstar_dashboard
sudo ./install.sh
```

`install.sh` needs to run as root — it installs system packages, writes to `/etc/hamvoip-gui/`, and installs a systemd unit. It checks for and installs everything else it needs (a modern Go toolchain, Piper/espeak-ng for text-to-speech, and optionally SkywarnPlus), builds the app natively on the Pi, and starts it listening on port 8088. Re-running the same command later pulls and installs updates. See [Deploying to a HamVoIP node](#deploying-to-a-hamvoip-node) below for the cross-compile alternative and first-run details.

## Features

The UI is five pages: **Home**, **Nodes**, **Status**, **System**, and **Raw Config**. On a narrow screen the nav collapses behind a hamburger button; every page's own layout is responsive independent of that.

### Home

- **Live status** per node — an on-air indicator (from `rpt stats`), who's connected right now, and every field app_rpt reports
- **Link / unlink** another node in one click, via the standard `*3` / `*1` touch-tone commands
- **Failsafes** for the two states that stop Asterisk starting: a node whose radio device has vanished from its config file, and a driver config file with no devices at all

### Nodes

The fast, no-CLI-calls list for finding, adding, and removing nodes — number, callsign, radio device, and a link to each node's full edit page. Home and Status both shell out to `asterisk -rx` per node to show live state; this page never does, so it stays quick regardless of node count or whether Asterisk is even running.

### Status

Detailed per-node stats (every field `rpt stats` reports) and connection history — connected nodes and link activity as tables, keeping the last 10 records of how connections changed, plus callsigns beside node numbers from AllStarLink's published node directory (downloaded daily).

### Adding and configuring a node

- **Setup wizard** (`/nodes/new`) asks only for node number, callsign, AllStarLink password, repeater mode, and radio interface, then derives and writes everything else across `rpt.conf`, `extensions.conf`, `iax.conf`, and the radio driver file
- **Node page** — tabbed into **Setup**, **Tones & Audio**, **Allstar Network**, **Live & Commands**, and **Automation**:
  - *Setup*: identity, radio hardware (pick an existing device or create one inline), timing (squelch tail, transmit safety cutoff, station ID interval — shown and edited in minutes, stored as the milliseconds app_rpt expects), and command/tone set
  - *Tones & Audio*: a friendly courtesy-tone and telemetry-sound editor; custom sound files with upload (a WAV is transcoded via `sox` to the format app_rpt expects), in-browser playback, and delete; and a **"Create from text"** generator that turns typed text into a sound file using [Piper](https://github.com/rhasspy/piper) (offline neural TTS, the default) with an automatic fallback to `espeak-ng`/`espeak` on hardware Piper doesn't support (e.g. 32-bit ARMv6 — Pi Zero/1) — both with an in-browser preview before saving
  - *Allstar Network*: AllStarLink registration and advanced IAX2 connection settings
  - *Live & Commands*: who's connected right now, connect/disconnect a node, DTMF relay (`asterisk -rx "rpt fun <node> <digits>"`), saved macros, and a command-set reference
  - *Automation*: scheduled connect/disconnect (cron-style, written straight into `rpt.conf`'s own scheduler), scheduled sound playback (tracked and fired by this app itself, since app_rpt has no native equivalent), and, if [SkywarnPlus](https://github.com/Mason10198/SkywarnPlus) is installed (see below), its weather-alert settings — feature toggles, county codes, node registration, Pushover notifications, and SkyDescribe
- **Command/tone set** — each node gets its own `functions<number>` / `macro<number>` / `telemetry<number>` / `morse<number>` sections, either copied from an existing node or bootstrapped from known-good defaults. This matters: a node whose `functions=` field is blank falls back to a bare `[functions]` section that doesn't exist on a stock HamVoIP install, so it silently accepts no DTMF commands at all
- **Dialplan entries** — `extensions.conf`'s `radio-secure`, `radio-secure-proxy`, and `radio-iaxrpt` contexts are written on create and removed on delete, with a bulk backfill button on Home for nodes that predate this app

### Weather alerts via SkywarnPlus

The Automation tab can configure [SkywarnPlus](https://github.com/Mason10198/SkywarnPlus) by [Mason10198](https://github.com/Mason10198) — a third-party tool that announces National Weather Service alerts over the repeater — if you choose to install it via `install.sh`'s interactive prompt (see [Deploying](#deploying-to-a-hamvoip-node) below; it's opt-in, not installed by default). This app never bundles or redistributes SkywarnPlus itself: `install.sh` downloads the real upstream release, and this app's UI only edits its `config.yaml` afterward (feature toggles, including AlertScript's own on/off switch, via SkywarnPlus's own `SkyControl.py`; county codes, node registration, Pushover push-notification settings, and SkyDescribe's text-to-speech settings via a small companion script, [`deploy/sky_configure.py`](deploy/sky_configure.py), that uses the same `ruamel.yaml` dependency SkywarnPlus already requires). AlertScript's own command/DTMF mappings and the courtesy-tone/ID swap during alerts aren't covered by this UI — see SkywarnPlus's own README and edit its `config.yaml` directly for those.

### System

Device name, static IP / DHCP, radio device management, SHARI USB audio preset, SA818/DRA818 module programming (frequency, CTCSS, squelch — over serial via `818-prog`), Asterisk restart and reboot, admin password, and a log tail.

### Raw Config

Generic section/key editor for `rpt.conf`, `iax.conf`, `extensions.conf`, `usbradio.conf`, `simpleusb.conf`, and `voter.conf` — a fallback for anything without a dedicated form.

Deliberately **not** covered by a dedicated form (edit via Raw Config instead): `voter.conf` (receiver voting) and GPIO — these vary enough between setups, or are used by few enough nodes, that a generic editor is the more honest choice than a form built on assumptions no one has verified against real hardware.

## Why this stack

Go compiles to a single static binary with everything embedded (templates, CSS, JS) — copy one file to the Pi, no dependency management on-device. The frontend is hand-written HTML/CSS/vanilla JS, no build pipeline, no CDN dependencies. The UI works fully offline, which matters on a device whose whole job is sometimes *providing* connectivity; the only thing this app's own process reaches the internet for is the optional node-directory download that puts callsigns next to node numbers, and everything works without it. (If you opt into installing SkywarnPlus, that's a separate process this app only configures — it makes its own periodic calls to the National Weather Service to do its job.)

Config files are read and written through [`internal/asteriskconf`](internal/asteriskconf/asteriskconf.go), a parser that preserves comments, blank lines, and key order — HamVoIP's shipped configs are heavily hand-annotated, and a generic INI library would silently discard all of that on save.

## Building

Requires Go 1.22+.

```sh
go test ./...   # run the test suite
make build      # build for your current machine
make pi         # cross-compile for Pi Zero/1/2 (armv6)
make pi64       # cross-compile for Pi 3/4 running 64-bit OS
```

## Deploying to a HamVoIP node

### Recommended: build on the Pi

Clone the repo on the node and run the top-level `install.sh`. It checks for the tools it needs (installing Go from go.dev if pacman's is too old — Arch Linux ARM's has been observed at go1.6, far below the 1.22 this needs; `sox`; and Piper or, where Piper has no build for the board's architecture, an `espeak-ng`/`espeak` fallback, for the "Create from text" sound generator), asks whether to also install SkywarnPlus (opt-in — see [Weather alerts via SkywarnPlus](#weather-alerts-via-skywarnplus) above), pulls any new commits, builds natively, and deploys:

```sh
git clone https://github.com/na4wx/na4wx_allstar_dashboard.git
cd na4wx_allstar_dashboard
sudo ./install.sh
```

Re-run the same command later to update. It always builds, including when there was nothing new to pull.

### Alternative: cross-compile on a dev machine

1. Cross-compile: `make pi` (or `make pi64` for a 64-bit image).
2. Copy the binary, the systemd unit, and the deploy script to the Pi:
   ```sh
   scp bin/hamvoip-gui-armv6 deploy/hamvoip-gui.service deploy/install.sh root@<pi-ip>:/root/
   ```
   Keep `hamvoip-gui.service` and `install.sh` in the same directory — the script installs the unit file from next to itself.
3. On the Pi: `cd /root && sudo ./install.sh hamvoip-gui-armv6`

   (That's `deploy/install.sh`, copied flat by the `scp` above — not the top-level `install.sh`, which builds from source instead of installing a prebuilt binary.)

Either route installs the binary to `/usr/local/bin/hamvoip-gui`, installs and enables a systemd unit, and starts it listening on port 8088.

### First run

Visit `http://<pi-ip>:8088/setup` to create the admin account — there is no default password. Until an account is created, every page redirects to `/setup`.

## Security notes

- The service runs as root (it edits root-owned config files and calls `systemctl`/`asterisk -rx`/`reboot`), so the admin account is the only thing standing between the network and full control of the node. Use a real password, and don't expose port 8088 to the open internet — put it behind a VPN, WireGuard, or at minimum a firewall rule restricting it to your LAN.
- There's no built-in TLS. If you need HTTPS, put a reverse proxy (Caddy, nginx) in front of it.
- Static IP changes are written to `dhcpcd.conf` but never auto-applied — they take effect on next reboot, specifically so a typo can't lock you out of the node mid-session.

## Command-line flags

```
-addr              listen address (default ":8088")
-asterisk-etc      path to Asterisk's config directory (default "/etc/asterisk")
-auth-file         path to store admin credentials (default "/etc/hamvoip-gui/auth.json")
-asterisk-bin      path to the asterisk binary, or bare name if it's on PATH (default "asterisk")
-asterisk-log      path to Asterisk's full log file, shown on the System page (default "/var/log/asterisk/full")
-sa818-tool        path to the 818-prog SA818/DRA818 programmer (default "818-prog")
-sa818-state-file  where to record the last settings sent to the SA818 module, since the
                   module itself can't be queried (default "/etc/hamvoip-gui/sa818-last.json")
-node-db-file      local copy of AllStarLink's node directory, used to show callsigns beside
                   node numbers (default "/var/lib/asterisk/astdb.txt")
-node-db-url       where to download that directory from, refreshed daily
                   (default "https://allmondb.allstarlink.org/allmondb.php")
-node-db-refresh   download the node directory daily; set false to only read an existing
                   on-disk copy and never make outbound requests (default true)
-sounds-custom-dir directory for the operator's own uploadable sound files, e.g. a station-ID
                   recording (default "/etc/asterisk/local")
-sounds-stock-dir  app_rpt's own built-in prompt library, offered read-only as pick-list
                   options (default "/var/lib/asterisk/sounds/rpt")
-sox-tool          path to the sox audio tool, or bare name if it's on PATH — transcodes an
                   uploaded or generated sound file to the format app_rpt expects (default "sox")
-sound-schedule-file
                   where the Automation tab's scheduled sound-playback entries are stored
                   (default "/etc/hamvoip-gui/sound-schedule.json")
-tts-tool          path to the Piper text-to-speech binary, or bare name if it's on PATH, used
                   by "Create from text" (default "piper")
-tts-voices-dir    directory holding downloaded Piper voice models (.onnx files); install.sh
                   downloads one by default (default "/etc/hamvoip-gui/piper-voices")
-skywarn-dir       directory holding an operator-installed copy of SkywarnPlus, if any — see
                   Weather alerts via SkywarnPlus (default "/usr/local/bin/SkywarnPlus")
```

All Asterisk control (the running/stopped indicator, the System page's restart button, and each node's live status and DTMF relay) goes through Asterisk's own CLI (`<bin> -rx "..."`) rather than `systemctl` — Asterisk is frequently supervised some other way (e.g. HamVoIP runs it under a `safe_asterisk` watchdog script, not a native systemd unit), so asking Asterisk itself is the only check that works regardless of how it's actually being run.

The node directory is the only feature that reaches the internet from this app's own process. It's cosmetic — it turns `49616` into `49616 WB4GBI` — and everything works without it. `/var/lib/asterisk/astdb.txt` is the same path AllStarLink's own `asl3-update-astdb` uses, so other dashboards on the box share one copy. HamVoIP also refreshes this file from its own cron job; both write the same data from the same source, and writes are atomic, so the overlap is harmless. Use `-node-db-refresh=false` if you'd rather this app never made outbound requests.

`-asterisk-bin` and `-asterisk-log` matter because HamVoIP installs Asterisk at a non-standard prefix (`/usr/local/hamvoip-asterisk/`) rather than `/usr/sbin` and `/var/log`. Find the real binary path with:

```sh
ps aux | grep asterisk
```

and the real log path straight from Asterisk itself (more reliable than guessing from the binary path):

```sh
asterisk -rx "logger show channels"
```

then pass both explicitly (also update `ExecStart` in `deploy/hamvoip-gui.service` to match, then `sudo systemctl daemon-reload && sudo systemctl restart hamvoip-gui`).

## Testing

```sh
go test ./...
go vet ./...
gofmt -l .
```

The `internal/asteriskconf` and `internal/config` packages have the heaviest test coverage since they're the code responsible for not corrupting your node's configuration — round-trip parsing, comment preservation, duplicate-key handling (`exten =>`, `register =>`), and section create/update/delete are all covered.

The parsers for app_rpt's CLI output (`rpt nodes`, `rpt lstats`, `rpt stats`) and for the AllStarLink node directory are tested against output captured verbatim from a real node rather than invented samples. That's deliberate: assuming a real-world format matched what looked reasonable has been this project's most repeated source of bugs.

## Acknowledgments

This app configures and shells out to several third-party tools rather than reimplementing what they already do well:

- [SkywarnPlus](https://github.com/Mason10198/SkywarnPlus) by [Mason10198](https://github.com/Mason10198) (GPL-3.0) — the optional weather-alert automation on the Automation tab; see [Weather alerts via SkywarnPlus](#weather-alerts-via-skywarnplus)
- [Piper](https://github.com/rhasspy/piper) — offline neural text-to-speech, the default engine behind "Create from text"
- [espeak-ng](https://github.com/espeak-ng/espeak-ng) — the text-to-speech fallback on hardware Piper doesn't support
- [sox](https://sox.sourceforge.net/) — audio transcoding for uploaded and generated sound files
- [AllStarLink](https://www.allstarlink.org/) / [app_rpt](https://github.com/AllStarLink/app_rpt) and [HamVoIP](http://hamvoip.org/) — the platform this app configures

## License

Copyright (C) 2026 Jordan Webb, NA4WX

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
[GNU General Public License](LICENSE) for more details.
