# HamVoIP Config GUI

A browser-based configuration tool for a [HamVoIP](http://hamvoip.org/) AllStarLink/Asterisk node — edit `rpt.conf`, `iax.conf`, `usbradio.conf`/`simpleusb.conf`, and system/network settings without SSH or a text editor.

Runs as a single self-contained binary directly on the Pi. No Python/Node runtime to install, no database, no build step for the frontend.

## Features

- **Nodes** — node identity, dial string, radio channel, duplex mode, timing, telemetry (`rpt.conf`)
- **Radio** — RX/TX levels, carrier/CTCSS source, PTT polarity (`usbradio.conf` / `simpleusb.conf`)
- **Connections** — DTMF function macros (`rpt.conf [functions]`), live link status, and a transparent DTMF relay (`asterisk -rx "rpt fun <node> <digits>"`) for connecting/disconnecting nodes
- **Node registration** — IAX2 registration and peer authentication to the AllStarLink network (`iax.conf`)
- **System** — hostname, static IP / DHCP, Asterisk service control, reboot, admin password, log tail
- **Raw Config** — generic section/key editor for any of the above files, as a fallback for anything not covered by a dedicated page

Deliberately **not** covered by a dedicated page (edit via Raw Config instead): `extensions.conf` dialplan details, `voter.conf` (receiver voting), and GPIO — these vary enough between setups, or are used by few enough nodes, that a generic editor is the more honest choice than a form built on assumptions no one has verified against real hardware.

## Why this stack

Go compiles to a single static binary with everything embedded (templates, CSS, JS) — copy one file to the Pi, no dependency management on-device. The frontend is hand-written HTML/CSS/vanilla JS, no build pipeline, no CDN dependencies (works with no internet access, which matters on a device whose whole job is sometimes *providing* connectivity).

Config files are read and written through [`internal/asteriskconf`](internal/asteriskconf/asteriskconf.go), a parser that preserves comments, blank lines, and key order — HamVoIP's shipped configs are heavily hand-annotated, and a generic INI library would silently discard all of that on save.

## Building

Requires Go 1.22+.

```sh
go test ./...        # run the test suite
make build            # build for your current machine
make pi                # cross-compile for Pi Zero/1/2 (armv6)
make pi64              # cross-compile for Pi 3/4 running 64-bit OS
```

## Deploying to a HamVoIP node

1. Cross-compile on your dev machine: `make pi` (or `make pi64` for a 64-bit image).
2. Copy the binary and `deploy/` directory to the Pi, e.g.:
   ```sh
   scp bin/hamvoip-gui-armv6 deploy/hamvoip-gui.service deploy/install.sh root@<pi-ip>:/root/hamvoip-gui-deploy/
   ```
3. On the Pi: `cd /root/hamvoip-gui-deploy && sudo ./install.sh hamvoip-gui-armv6`

This installs the binary to `/usr/local/bin/hamvoip-gui`, installs and enables a systemd unit, and starts it listening on port 8088.

### First run

Visit `http://<pi-ip>:8088/setup` to create the admin account — there is no default password. Until an account is created, every page redirects to `/setup`.

## Security notes

- The service runs as root (it edits root-owned config files and calls `systemctl`/`asterisk -rx`/`reboot`), so the admin account is the only thing standing between the network and full control of the node. Use a real password, and don't expose port 8088 to the open internet — put it behind a VPN, WireGuard, or at minimum a firewall rule restricting it to your LAN.
- There's no built-in TLS. If you need HTTPS, put a reverse proxy (Caddy, nginx) in front of it.
- Static IP changes are written to `dhcpcd.conf` but never auto-applied — they take effect on next reboot, specifically so a typo can't lock you out of the node mid-session.

## Command-line flags

```
-addr           listen address (default ":8088")
-asterisk-etc   path to Asterisk's config directory (default "/etc/asterisk")
-auth-file      path to store admin credentials (default "/etc/hamvoip-gui/auth.json")
```

## Testing

```sh
go test ./...
go vet ./...
gofmt -l .
```

The `internal/asteriskconf` and `internal/config` packages have the heaviest test coverage since they're the code responsible for not corrupting your node's configuration — round-trip parsing, comment preservation, duplicate-key handling (`exten =>`, `register =>`), and section create/update/delete are all covered.
