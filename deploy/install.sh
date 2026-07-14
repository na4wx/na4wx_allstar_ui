#!/bin/sh
# Installs hamvoip-gui on a HamVoIP node. Run this ON THE PI, as root
# (or via sudo), from the directory containing the cross-compiled
# binary (see the Makefile's `pi`/`pi64` targets).
#
# Usage: sudo ./install.sh [path-to-binary]
# If no path is given, picks bin/hamvoip-gui-armv6 or bin/hamvoip-gui-arm64
# next to this script based on `uname -m`.

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

BINARY="$1"
if [ -z "$BINARY" ]; then
	case "$(uname -m)" in
		aarch64|arm64)
			BINARY="$REPO_ROOT/bin/hamvoip-gui-arm64"
			;;
		*)
			BINARY="$REPO_ROOT/bin/hamvoip-gui-armv6"
			;;
	esac
fi

if [ ! -f "$BINARY" ]; then
	echo "error: binary not found at $BINARY" >&2
	echo "Build it first with 'make pi' or 'make pi64' on your dev machine," >&2
	echo "copy it to this Pi, then re-run: sudo ./install.sh /path/to/binary" >&2
	exit 1
fi

if [ "$(id -u)" != "0" ]; then
	echo "error: this script must be run as root (sudo ./install.sh)" >&2
	exit 1
fi

echo "Installing $BINARY -> /usr/local/bin/hamvoip-gui"
install -m 0755 "$BINARY" /usr/local/bin/hamvoip-gui

echo "Installing systemd unit"
install -m 0644 "$SCRIPT_DIR/hamvoip-gui.service" /etc/systemd/system/hamvoip-gui.service

mkdir -p /etc/hamvoip-gui

systemctl daemon-reload
systemctl enable hamvoip-gui
systemctl restart hamvoip-gui

IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo
echo "Installed and started. Visit this URL to finish setup:"
echo "  http://${IP:-<this-pi-ip>}:8088/setup"
