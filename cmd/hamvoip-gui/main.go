// Command hamvoip-gui serves a browser-based configuration UI for a
// HamVoIP AllStar node: editing rpt.conf/iax.conf/usbradio.conf/
// extensions.conf and controlling the Asterisk service, without needing
// SSH or a text editor.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"hamvoipconfiggui/internal/auth"
	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/server"
	"hamvoipconfiggui/web"
)

func main() {
	addr := flag.String("addr", ":8088", "listen address")
	asteriskEtc := flag.String("asterisk-etc", "/etc/asterisk", "path to Asterisk's config directory")
	authFile := flag.String("auth-file", "/etc/hamvoip-gui/auth.json", "path to store admin credentials")
	asteriskBin := flag.String("asterisk-bin", "asterisk", "path to the asterisk binary, or bare name if it's on PATH (some distros install it somewhere non-standard, e.g. HamVoIP's /usr/local/hamvoip-asterisk/sbin/asterisk); used for status checks, restarts, and the Connections page — Asterisk's own CLI is used rather than systemd, since it's frequently supervised some other way (e.g. a safe_asterisk watchdog script)")
	asteriskLog := flag.String("asterisk-log", "/var/log/asterisk/full", "path to Asterisk's full log file, shown on the System page (varies with where Asterisk is installed, same as -asterisk-bin)")
	sa818Tool := flag.String("sa818-tool", "818-prog", "path to the 818-prog SA818/DRA818 radio module programmer, or bare name if it's on PATH (used by the System page's radio module card to send frequency/tone/squelch settings over serial)")
	sa818StatePath := flag.String("sa818-state-file", "/etc/hamvoip-gui/sa818-last.json", "path to store the last settings sent to the SA818/DRA818 module (there's no way to query the module itself, so this is only a record of what this app last sent)")
	flag.Parse()

	templatesFS, err := fs.Sub(web.Templates, "templates")
	if err != nil {
		log.Fatalf("templates: %v", err)
	}
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatalf("static: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(*authFile), 0700); err != nil {
		log.Fatalf("create auth dir: %v", err)
	}
	authMgr, err := auth.NewManager(*authFile)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	if !authMgr.Configured() {
		log.Printf("no admin account configured yet; visit http://<this-host>%s/setup to create one", *addr)
	}

	store := config.NewStore(*asteriskEtc)

	srv, err := server.New(store, authMgr, templatesFS, staticFS, *asteriskBin, *asteriskLog, *sa818Tool, *sa818StatePath)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	log.Printf("hamvoip-gui listening on %s (asterisk config dir: %s)", *addr, *asteriskEtc)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
