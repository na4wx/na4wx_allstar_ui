// Package server wires HTTP routes to the config and auth packages and
// renders the embedded templates.
package server

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"

	"hamvoipconfiggui/internal/auth"
	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/system"
)

const sessionCookie = "hamvoip_gui_session"

type Server struct {
	store          *config.Store
	auth           *auth.Manager
	tmpl           map[string]*template.Template
	mux            *http.ServeMux
	asteriskBin    string
	asteriskLog    string
	sa818Tool      string
	sa818StatePath string

	// history is the rolling per-node record of connection changes shown
	// on the home page, filled by StartLinkHistoryPoller (see
	// linkhistory.go). Always non-nil, so page renders work whether or
	// not the poller was started.
	history *linkHistory
}

// New builds a Server. templatesFS should contain web/templates and
// staticFS should contain web/static (both typically embed.FS values
// from main). asteriskBin is the path (or bare name, if it's on PATH)
// to the asterisk binary — it's not always just "asterisk" (e.g.
// HamVoIP installs it at /usr/local/hamvoip-asterisk/sbin/asterisk),
// so it's a caller-supplied value rather than hardcoded — see the
// -asterisk-bin flag in main.go. All Asterisk control (status, restart,
// the Connections page's live status/DTMF relay) goes through this
// binary's own CLI rather than systemd, since Asterisk is very often
// supervised some other way (e.g. a safe_asterisk watchdog script)
// rather than as a native systemd unit. asteriskLog is the path to
// Asterisk's full log file, shown on the System page — like
// asteriskBin, it follows wherever Asterisk is actually installed
// rather than a fixed standard location. sa818Tool is the path (or bare
// name, if on PATH) to the 818-prog SA818/DRA818 radio module
// programmer used by the System page's radio module card; sa818StatePath
// is where the last settings sent to it are recorded (see internal/sa818).
func New(store *config.Store, authMgr *auth.Manager, templatesFS, staticFS fs.FS, asteriskBin, asteriskLog, sa818Tool, sa818StatePath string) (*Server, error) {
	s := &Server{store: store, auth: authMgr, mux: http.NewServeMux(), asteriskBin: asteriskBin, asteriskLog: asteriskLog, sa818Tool: sa818Tool, sa818StatePath: sa818StatePath, history: newLinkHistory()}

	tmpl, err := parseTemplates(templatesFS)
	if err != nil {
		return nil, err
	}
	s.tmpl = tmpl

	s.routes(staticFS)
	return s, nil
}

func parseTemplates(templatesFS fs.FS) (map[string]*template.Template, error) {
	pages := []string{"setup.html", "login.html", "home.html", "node_new.html", "node_form.html", "config.html", "system.html", "radio_form.html"}
	out := map[string]*template.Template{}
	for _, page := range pages {
		// radio_device_fields.html is a shared partial ({{template
		// "radio_device_fields" ...}}), included for every page since
		// it's harmless where unused and needed by both node_form.html
		// and radio_form.html.
		t, err := template.ParseFS(templatesFS, "layout.html", "radio_device_fields.html", page)
		if err != nil {
			return nil, err
		}
		out[page] = t
	}
	return out, nil
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes(staticFS fs.FS) {
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	s.mux.HandleFunc("GET /setup", s.handleSetupForm)
	s.mux.HandleFunc("POST /setup", s.handleSetupSubmit)
	s.mux.HandleFunc("GET /login", s.handleLoginForm)
	s.mux.HandleFunc("POST /login", s.handleLoginSubmit)
	s.mux.HandleFunc("POST /logout", s.requireAuth(s.handleLogout))

	s.mux.HandleFunc("GET /{$}", s.requireAuth(s.handleHome))
	s.mux.HandleFunc("GET /api/status", s.requireAuth(s.handleAPIStatus))

	// Node pages: the one-stop shop for a node's identity, radio
	// hardware, command/tone set, AllStarLink registration, and live
	// status/DTMF/macros — everything that used to be split across the
	// Nodes, Radio, and Connections pages.
	s.mux.HandleFunc("GET /nodes/new", s.requireAuth(s.handleNodeNewForm))
	s.mux.HandleFunc("POST /nodes", s.requireAuth(s.handleNodeCreate))
	s.mux.HandleFunc("POST /nodes/sync-extensions", s.requireAuth(s.handleNodesSyncExtensions))
	s.mux.HandleFunc("GET /nodes/{number}", s.requireAuth(s.handleNodeEditForm))
	s.mux.HandleFunc("POST /nodes/{number}", s.requireAuth(s.handleNodeSave))
	s.mux.HandleFunc("POST /nodes/{number}/link", s.requireAuth(s.handleNodeLink))
	s.mux.HandleFunc("POST /nodes/{number}/recreate-device", s.requireAuth(s.handleNodeRecreateDevice))
	s.mux.HandleFunc("POST /nodes/{number}/registration", s.requireAuth(s.handleNodeRegistrationSave))
	s.mux.HandleFunc("POST /nodes/{number}/clone-config", s.requireAuth(s.handleNodeCloneConfig))
	s.mux.HandleFunc("POST /nodes/{number}/macros", s.requireAuth(s.handleNodeMacroSave))
	s.mux.HandleFunc("POST /nodes/{number}/macros/{digits}/delete", s.requireAuth(s.handleNodeMacroDelete))
	s.mux.HandleFunc("POST /nodes/{number}/macrodefs", s.requireAuth(s.handleNodeMacroDefSave))
	s.mux.HandleFunc("POST /nodes/{number}/macrodefs/{digits}/delete", s.requireAuth(s.handleNodeMacroDefDelete))
	s.mux.HandleFunc("POST /nodes/{number}/dtmf", s.requireAuth(s.handleNodeSendDTMF))
	s.mux.HandleFunc("POST /nodes/{number}/delete", s.requireAuth(s.handleNodeDelete))

	s.mux.HandleFunc("GET /config", s.requireAuth(s.handleConfigIndex))
	s.mux.HandleFunc("GET /config/{file}", s.requireAuth(s.handleConfigFile))
	s.mux.HandleFunc("POST /config/{file}", s.requireAuth(s.handleConfigSave))

	s.mux.HandleFunc("GET /system", s.requireAuth(s.handleSystemPage))
	s.mux.HandleFunc("POST /system/hostname", s.requireAuth(s.handleSystemHostname))
	s.mux.HandleFunc("POST /system/password", s.requireAuth(s.handleSystemPassword))
	s.mux.HandleFunc("POST /system/network", s.requireAuth(s.handleSystemNetwork))
	s.mux.HandleFunc("POST /system/sharipi/apply", s.requireAuth(s.handleSystemShariApply))
	s.mux.HandleFunc("POST /system/sa818/apply", s.requireAuth(s.handleSystemSA818Apply))
	s.mux.HandleFunc("POST /system/restart-asterisk", s.requireAuth(s.handleSystemRestartAsterisk))
	s.mux.HandleFunc("POST /system/reboot", s.requireAuth(s.handleSystemReboot))
	// Radio device editing (adjusting an already-created device) — device
	// *creation* only happens inline from a node's own page now.
	s.mux.HandleFunc("GET /system/radio/{file}/{name}", s.requireAuth(s.handleRadioEditForm))
	s.mux.HandleFunc("POST /system/radio/{file}/{name}", s.requireAuth(s.handleRadioSave))
	s.mux.HandleFunc("POST /system/radio/{file}/{name}/delete", s.requireAuth(s.handleRadioDelete))
	s.mux.HandleFunc("POST /system/radio/{file}/placeholder", s.requireAuth(s.handleSystemAddPlaceholderDevice))

	// Courtesy redirects for old bookmarks — Nodes/Radio/Connections no
	// longer exist as standalone pages; everything they offered is
	// reachable from Home, System, or a node's own page now.
	s.mux.HandleFunc("GET /nodes", s.requireAuth(redirectTo("/")))
	s.mux.HandleFunc("GET /radio", s.requireAuth(redirectTo("/system")))
	s.mux.HandleFunc("GET /connections", s.requireAuth(redirectTo("/")))
}

func redirectTo(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, path, http.StatusSeeOther)
	}
}

// pageData is the common template context. Handlers embed it and add
// page-specific fields.
type pageData struct {
	LoggedIn  bool
	FlashKind string
	Flash     string
}

func flash(kind, msg string) pageData {
	return pageData{LoggedIn: true, FlashKind: kind, Flash: msg}
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	t, ok := s.tmpl[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

// requireAuth wraps a handler so it 302s to /login (or /setup, if no
// account has been created yet) without a valid session cookie.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.Configured() {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if _, ok := s.auth.ValidateSession(c.Value); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// currentUsername returns the logged-in user's name, or "" if called
// outside a requireAuth-wrapped handler (where a valid session is
// already guaranteed).
func (s *Server) currentUsername(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	username, _ := s.auth.ValidateSession(c.Value)
	return username
}

func (s *Server) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	if s.auth.Configured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, "setup.html", pageData{})
}

func (s *Server) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if s.auth.Configured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")
	if password != confirm {
		s.render(w, "setup.html", pageData{Flash: "Passwords do not match", FlashKind: "error"})
		return
	}
	if err := s.auth.SetCredentials(username, password); err != nil {
		s.render(w, "setup.html", pageData{Flash: err.Error(), FlashKind: "error"})
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if !s.auth.Configured() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", pageData{})
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	if !s.auth.Verify(username, password) {
		s.render(w, "login.html", pageData{Flash: "Invalid username or password", FlashKind: "error"})
		return
	}
	token, err := s.auth.CreateSession(username)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.DestroySession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}


func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	status := system.Snapshot(ctx, s.asteriskBin)
	writeJSON(w, status)
}
