#!/usr/bin/python3
"""
sky_configure.py - a small companion script for SkywarnPlus, installed
alongside it by this app's install.sh.

SkywarnPlus's own SkyControl.py already safely edits a fixed set of
boolean keys (enable, sayalert, sayallclear, tailmessage, courtesytone,
idchange, alertscript) via ruamel.yaml, which preserves config.yaml's
own extensive inline comments on every save -- see SkyControl.py's own
comment: "Use ruamel.yaml instead of PyYAML to preserve comments in the
config file". This script covers the two things SkyControl.py does not:
the two YAML list settings (Alerting.CountyCodes, Asterisk.Nodes), using
the exact same ruamel.yaml approach so comments stay preserved here too.

Deliberately avoids f-strings and argparse in favor of .format() and
manual sys.argv parsing, matching SkyControl.py's own style -- HamVoIP's
documented "very outdated" Python needs the lowest-common-denominator
syntax, not just whatever the box currently running this happens to
support.

Usage:
  sky_configure.py status                  Prints one JSON object to stdout.
  sky_configure.py set-counties C1,C2,...  Replaces Alerting.CountyCodes.
  sky_configure.py add-node <number>       Appends to Asterisk.Nodes if missing.

Exits non-zero with a message on stderr on any failure.

Known cosmetic limitation (verified against a real downloaded
config.yaml, not assumed): if a comment block for the *next* key happens
to sit directly under the list being modified, ruamel.yaml occasionally
drops that one comment block on save -- e.g. editing Asterisk.Nodes can
lose the comment above the following AudioDelay key, though AudioDelay's
own value is untouched. Every comment on keys not adjacent to an edited
list is unaffected. This is inherent to ruamel.yaml's own comment
anchoring (confirmed happening even with a no-op load-then-dump), not
something unique to this script, and SkyControl.py carries the same risk
in principle -- it just never happens to touch a list itself.
"""
import json
import sys
from pathlib import Path

from ruamel.yaml import YAML
from ruamel.yaml.comments import CommentedSeq

SCRIPT_DIR = Path(__file__).parent.absolute()
CONFIG_FILE = SCRIPT_DIR / "config.yaml"

yaml = YAML()
yaml.preserve_quotes = True
# Matches config.yaml's own list-item indent style (verified against a
# real downloaded copy: without this, every list in the file -- not just
# the ones this script edits -- re-renders in ruamel.yaml's default
# style on save, which is a harmless but needlessly noisy diff).
yaml.indent(mapping=2, sequence=4, offset=2)


def load():
    with open(str(CONFIG_FILE), "r") as f:
        return yaml.load(f)


def save(config):
    with open(str(CONFIG_FILE), "w") as f:
        yaml.dump(config, f)


def cmd_status():
    config = load()
    alerting = config.get("Alerting", {}) or {}
    tailmessage = config.get("Tailmessage", {}) or {}
    asterisk = config.get("Asterisk", {}) or {}
    codes = alerting.get("CountyCodes") or []

    # CountyCodes entries can be plain strings or single-key
    # {code: "file.wav"} dicts (for county-name audio tagging) -- flatten
    # to plain codes, since that richer per-county-audio form isn't
    # something this app's UI manages.
    plain_codes = []
    for c in codes:
        if isinstance(c, dict):
            plain_codes.extend(c.keys())
        else:
            plain_codes.append(str(c))

    nodes = [str(n) for n in (asterisk.get("Nodes") or [])]

    status = {
        "enable": bool(config.get("SKYWARNPLUS", {}).get("Enable", False)),
        "sayalert": bool(alerting.get("SayAlert", False)),
        "sayallclear": bool(alerting.get("SayAllClear", False)),
        "tailmessage": bool(tailmessage.get("Enable", False)),
        "countycodes": plain_codes,
        "nodes": nodes,
    }
    print(json.dumps(status))


def _replace_seq(parent, key, values):
    # Mutates the existing CommentedSeq in place rather than assigning a
    # plain list, so any of ruamel.yaml's own tracked formatting for that
    # specific node (as opposed to the document-wide indent style set
    # above) survives -- confirmed against a real config.yaml that this
    # matters less than the indent setting above, but it's free.
    existing = parent.get(key)
    if not isinstance(existing, CommentedSeq):
        existing = CommentedSeq()
        parent[key] = existing
    existing.clear()
    existing.extend(values)
    return existing


def cmd_set_counties(codes_arg):
    codes = [c.strip() for c in codes_arg.split(",") if c.strip()]
    config = load()
    _replace_seq(config.setdefault("Alerting", {}), "CountyCodes", codes)
    save(config)
    print("OK")


def cmd_add_node(node):
    config = load()
    asterisk = config.setdefault("Asterisk", {})
    nodes = [str(n) for n in (asterisk.get("Nodes") or [])]
    if node not in nodes:
        nodes.append(node)
    _replace_seq(asterisk, "Nodes", nodes)
    save(config)
    print("OK")


def main():
    if len(sys.argv) < 2:
        sys.stderr.write("Usage: sky_configure.py <status|set-counties|add-node> [args]\n")
        sys.exit(1)

    cmd = sys.argv[1]
    try:
        if cmd == "status":
            cmd_status()
        elif cmd == "set-counties":
            if len(sys.argv) != 3:
                sys.stderr.write("Usage: sky_configure.py set-counties C1,C2,...\n")
                sys.exit(1)
            cmd_set_counties(sys.argv[2])
        elif cmd == "add-node":
            if len(sys.argv) != 3:
                sys.stderr.write("Usage: sky_configure.py add-node <number>\n")
                sys.exit(1)
            cmd_add_node(sys.argv[2])
        else:
            sys.stderr.write("Unknown command: {}\n".format(cmd))
            sys.exit(1)
    except FileNotFoundError:
        sys.stderr.write("config.yaml not found at {}\n".format(CONFIG_FILE))
        sys.exit(1)
    except Exception as e:
        sys.stderr.write("error: {}\n".format(e))
        sys.exit(1)


if __name__ == "__main__":
    main()
