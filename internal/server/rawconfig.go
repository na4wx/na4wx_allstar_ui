package server

import (
	"net/http"
	"strconv"
	"strings"

	"hamvoipconfiggui/internal/asteriskconf"
)

// allowedConfigFiles whitelists which files the generic editor may touch,
// since the "file" path segment ultimately becomes a filesystem path.
var allowedConfigFiles = []string{
	"rpt.conf",
	"iax.conf",
	"usbradio.conf",
	"simpleusb.conf",
	"voter.conf",
	"extensions.conf",
}

func isAllowedConfigFile(name string) bool {
	for _, f := range allowedConfigFiles {
		if f == name {
			return true
		}
	}
	return false
}

// configPageData backs config.html for both the file-picker index and a
// selected file's editor, so the template can reference .Selected /
// .Sections unconditionally regardless of which handler rendered it.
type configPageData struct {
	pageData
	Files    []string
	Selected string
	Sections []configSection
}

type configSection struct {
	Name string
	Keys []asteriskconf.KV
}

func (s *Server) handleConfigIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, "config.html", configPageData{
		pageData: pageData{LoggedIn: true},
		Files:    allowedConfigFiles,
	})
}

func (s *Server) handleConfigFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !isAllowedConfigFile(name) {
		http.NotFound(w, r)
		return
	}
	f, err := s.store.RawFile(name)
	if err != nil {
		s.render(w, "config.html", configPageData{
			pageData: flash("error", err.Error()),
			Files:    allowedConfigFiles,
		})
		return
	}

	var sections []configSection
	for _, sec := range f.Sections() {
		sections = append(sections, configSection{Name: sec, Keys: f.SectionKeys(sec)})
	}

	s.render(w, "config.html", configPageData{
		pageData: pageData{LoggedIn: true},
		Files:    allowedConfigFiles,
		Selected: name,
		Sections: sections,
	})
}

// handleConfigSave applies edits posted as repeated form fields named
// "kv:<section>:<n>", where n is the line's position among that
// section's key/value lines (i.e. its index in SectionKeys(section)).
// Indexing by position rather than by key lets duplicate keys within a
// section (e.g. extensions.conf's repeated "exten =>" lines) be edited
// independently. Also handles "new_key:<section>" / "new_value:<section>"
// for adding one new key per section, and "new_section" for adding a
// section.
func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !isAllowedConfigFile(name) {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	f, err := s.store.RawFile(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Apply edits to existing keys, keyed by section so we can walk each
	// section's key list in file order and line them up with the posted
	// "kv:<section>:<index>" values (index = position within
	// SectionKeys(section), stable because we don't reorder).
	for _, sec := range f.Sections() {
		kv := f.SectionKeys(sec)
		for i, pair := range kv {
			formKey := "kv:" + sec + ":" + strconv.Itoa(i)
			if newVal, ok := r.Form[formKey]; ok && len(newVal) > 0 {
				if newVal[0] != pair.Value {
					setNthLineInSection(f, sec, i, newVal[0])
				}
			}
		}
		if newKey := strings.TrimSpace(r.FormValue("new_key:" + sec)); newKey != "" {
			f.Set(sec, newKey, r.FormValue("new_value:"+sec))
		}
	}

	if newSection := strings.TrimSpace(r.FormValue("new_section")); newSection != "" {
		f.EnsureSection(newSection)
	}

	if err := s.store.SaveRaw(name, f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/config/"+name, http.StatusSeeOther)
}

// setNthLineInSection updates the value of the n-th key/value line
// within section, where n matches its position in
// asteriskconf.File.SectionKeys(section) — i.e. its position among all
// key/value lines in that section, not just among same-named keys. This
// is what lets duplicate keys (extensions.conf's repeated "exten =>")
// be addressed unambiguously; asteriskconf.File.Set only ever targets
// the first occurrence of a key, which isn't enough here.
func setNthLineInSection(f *asteriskconf.File, section string, n int, value string) {
	count := -1
	inSection := false
	for _, l := range f.Lines {
		if l.Kind == asteriskconf.KindSection {
			inSection = l.Section == section
			continue
		}
		if !inSection || l.Kind != asteriskconf.KindKeyValue {
			continue
		}
		count++
		if count == n {
			l.SetValue(value)
			return
		}
	}
}
