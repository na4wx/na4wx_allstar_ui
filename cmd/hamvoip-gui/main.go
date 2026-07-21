// Command hamvoip-gui serves a browser-based configuration UI for a
// HamVoIP AllStar node: editing rpt.conf/iax.conf/usbradio.conf/
// extensions.conf and controlling the Asterisk service, without needing
// SSH or a text editor.
package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"hamvoipconfiggui/internal/auth"
	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/nodedb"
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
	nodeDBPath := flag.String("node-db-file", nodedb.DefaultPath, "path to the local copy of AllStarLink's node directory (node number -> callsign/description/location), used only to show callsigns beside node numbers; this is the same path ASL's own asl3-update-astdb uses, so other dashboards on the system share it")
	nodeDBURL := flag.String("node-db-url", nodedb.DefaultURL, "where to download the node directory from, refreshed daily")
	nodeDBRefresh := flag.Bool("node-db-refresh", true, "download the node directory daily; set false to only read whatever copy already exists on disk and never make outbound requests")
	soundsCustomDir := flag.String("sounds-custom-dir", "/etc/asterisk/local", "directory for the operator's own uploadable sound files (station ID, custom courtesy tones) — confirmed on real HamVoIP hardware to already hold the node's station-ID recording")
	soundsStockDir := flag.String("sounds-stock-dir", "/var/lib/asterisk/sounds/rpt", "app_rpt's own built-in prompt library, offered as read-only pick-list options (e.g. \"rpt/callproceeding\") — never written to")
	soxTool := flag.String("sox-tool", "sox", "path to the sox audio tool, or bare name if it's on PATH (used to transcode an uploaded sound file to the 8kHz mono format app_rpt expects)")
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

	srv, err := server.New(store, authMgr, templatesFS, staticFS, *asteriskBin, *asteriskLog, *sa818Tool, *sa818StatePath, *nodeDBPath, *nodeDBURL, *soundsCustomDir, *soundsStockDir, *soxTool)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	// Sample each node's link state in the background so the home page's
	// connection history reflects what actually happened, not just what
	// someone had the page open for. Started here rather than in
	// server.New so constructing a Server (in tests) doesn't shell out
	// to the asterisk binary.
	srv.StartLinkHistoryPoller(context.Background())

	// Node directory: read whatever copy is on disk, and (unless
	// disabled) keep it current. A download failure is logged and
	// otherwise ignored — this file only decorates node numbers with
	// callsigns, so nothing about the node's operation depends on it.
	if *nodeDBRefresh {
		srv.NodeDB().StartDailyRefresh(context.Background(), func(err error) {
			log.Printf("node directory: %v", err)
		})
	} else if err := srv.NodeDB().LoadFile(); err != nil {
		log.Printf("node directory: %v", err)
	}

	log.Printf("hamvoip-gui listening on %s (asterisk config dir: %s)", *addr, *asteriskEtc)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
