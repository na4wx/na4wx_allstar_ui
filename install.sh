#!/bin/bash
# Run this ON THE PI, as root, from inside the cloned repo (Arch Linux
# ARM — HamVoIP's OS). It makes sure the tools needed to build this
# project are installed, pulls the latest code if there is any, then
# builds natively and redeploys via deploy/install.sh.
#
# It always builds and deploys, including when the pull found nothing
# new. That matters for a first-time install: the user has just cloned,
# so there is nothing to fetch, and skipping the build in that case
# leaves them with no binary installed at all. Go's build cache makes a
# no-change rebuild cheap, so the cost of always building is small next
# to the cost of silently doing nothing.
#
# Usage: sudo ./install.sh

set -euo pipefail

MIN_GO_VERSION="1.22"
GO_TARBALL_VERSION="1.22.5"

log() { echo "==> $*"; }
err() { echo "error: $*" >&2; exit 1; }

[ "$(id -u)" = "0" ] || err "run as root: sudo ./install.sh"
command -v pacman >/dev/null 2>&1 || err "pacman not found — this script is for Arch Linux (HamVoIP's OS)"

REPO_ROOT=$(cd "$(dirname "$0")" && pwd)
cd "$REPO_ROOT"
[ -d .git ] || err "$REPO_ROOT is not a git checkout — clone the repo first"

# --- required tools -----------------------------------------------------

log "Checking required tools"

pacman_install() {
	# -Sy (not -Syu) so this only syncs enough to fetch the specific
	# missing package rather than performing a full system upgrade on a
	# live repeater node, which is a bigger action than "install the
	# tools this script needs" and not something to do unannounced. If
	# this fails because the local package database is too stale, run
	# `pacman -Syu` yourself first and re-run this script.
	pacman -Sy --noconfirm --needed "$@"
}

command -v git >/dev/null 2>&1 || { log "Installing git"; pacman_install git; }
command -v make >/dev/null 2>&1 || { log "Installing make"; pacman_install make; }
command -v curl >/dev/null 2>&1 || { log "Installing curl"; pacman_install curl; }
command -v tar >/dev/null 2>&1 || { log "Installing tar"; pacman_install tar; }

version_ge() { # version_ge A B => A >= B
	[ "$1" = "$2" ] && return 0
	[ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" = "$2" ]
}

go_version() {
	command -v go >/dev/null 2>&1 || return 1
	go version | sed -n 's/^go version go\([0-9.]*\).*/\1/p'
}

need_go_install=1
if v=$(go_version); then
	if version_ge "$v" "$MIN_GO_VERSION"; then
		log "go $v already installed (>= $MIN_GO_VERSION)"
		need_go_install=0
	else
		log "go $v is installed but too old (need >= $MIN_GO_VERSION)"
	fi
fi

if [ "$need_go_install" = "1" ]; then
	# pacman's go package on Arch Linux ARM has been observed to lag far
	# behind — go1.6 (from 2016) on the node this project was tested
	# against, nowhere near new enough for go.mod's "go 1.22" requirement.
	# Try pacman first in case it's current on your system; fall back to
	# installing the official upstream release directly if not.
	log "Trying pacman's go package"
	pacman_install go || true
	v=$(go_version || echo "0")
	if ! version_ge "$v" "$MIN_GO_VERSION"; then
		log "pacman's go ($v) is still too old — installing go $GO_TARBALL_VERSION from go.dev directly"
		case "$(uname -m)" in
			aarch64|arm64)
				GOARCH_TARBALL="arm64" ;;
			armv6l|armv7l|arm)
				GOARCH_TARBALL="armv6l" ;;
			*)
				err "unrecognized architecture $(uname -m) — install Go manually from https://go.dev/dl/" ;;
		esac
		TARBALL="go${GO_TARBALL_VERSION}.linux-${GOARCH_TARBALL}.tar.gz"
		TMP=$(mktemp -d)
		curl -fsSL -o "$TMP/$TARBALL" "https://go.dev/dl/$TARBALL"
		rm -rf /usr/local/go
		tar -C /usr/local -xzf "$TMP/$TARBALL"
		rm -rf "$TMP"
		ln -sf /usr/local/go/bin/go /usr/local/bin/go
		ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
		v=$(go_version) || err "go install from go.dev failed — check manually"
		log "Installed go $v to /usr/local/go"
	fi
fi

# --- pull latest ---------------------------------------------------------

log "Fetching latest from git"
git fetch origin

BRANCH=$(git rev-parse --abbrev-ref HEAD)
LOCAL=$(git rev-parse @)
REMOTE=$(git rev-parse "@{u}" 2>/dev/null) || err "branch $BRANCH has no upstream — check out a branch that tracks origin"

if [ "$LOCAL" = "$REMOTE" ]; then
	# Nothing to pull, but still build and deploy below. A first-time
	# install is exactly this case — the user just cloned, so there is
	# by definition nothing new to fetch, and an earlier version of this
	# script exited here and left them with no binary installed at all.
	log "Already up to date ($LOCAL) — building and deploying the current checkout"
else
	# Only require a clean tree when there is actually something to
	# merge. Someone who tweaked a file locally and just wants to
	# rebuild shouldn't be blocked by a pull they don't need.
	if [ -n "$(git status --porcelain)" ]; then
		err "working tree has uncommitted changes and origin/$BRANCH has new commits — commit or stash, then re-run"
	fi
	log "Updating $BRANCH: $LOCAL -> $REMOTE"
	git pull --ff-only origin "$BRANCH"
fi

# --- build and deploy -----------------------------------------------------

log "Building"
make build

log "Deploying"
./deploy/install.sh "$REPO_ROOT/bin/hamvoip-gui"

log "Done"
