package server

import (
	"net/http"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/system"
)

var radioFiles = []string{config.UsbradioConfFile, config.SimpleusbConfFile}

func isRadioFileParam(file string) bool {
	return file == config.UsbradioConfFile || file == config.SimpleusbConfFile
}

// standardCTCSSTones is the standard EIA set of sub-audible tone
// frequencies (Hz) used across ham and land-mobile radio, offered as
// suggestions on the transmit tone field — not an exhaustive validation
// list, since non-standard tones do exist.
var standardCTCSSTones = []string{
	"67.0", "69.3", "71.9", "74.4", "77.0", "79.7", "82.5", "85.4", "88.5", "91.5",
	"94.8", "97.4", "100.0", "103.5", "107.2", "110.9", "114.8", "118.8", "123.0", "127.3",
	"131.8", "136.5", "141.3", "146.2", "151.4", "156.7", "159.8", "162.2", "165.5", "167.9",
	"171.3", "173.8", "177.3", "179.9", "183.5", "186.2", "189.9", "192.8", "196.6", "199.5",
	"203.5", "206.5", "210.7", "213.8", "218.1", "221.3", "225.7", "229.1", "233.6", "237.1",
	"241.8", "245.5", "250.3", "254.1",
}

type radioIndexData struct {
	pageData
	Files   []string
	File    string
	Devices []string
}

// handleRadioIndex lists devices for one radio file (defaulting to
// usbradio.conf) with a picker for the other.
func (s *Server) handleRadioIndex(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	if !isRadioFileParam(file) {
		file = config.UsbradioConfFile
	}
	devices, err := s.store.ListRadioDevices(file)
	if err != nil {
		s.render(w, "radio_index.html", radioIndexData{
			pageData: flash("error", err.Error()),
			Files:    radioFiles,
			File:     file,
		})
		return
	}
	s.render(w, "radio_index.html", radioIndexData{
		pageData: pageData{LoggedIn: true},
		Files:    radioFiles,
		File:     file,
		Devices:  devices,
	})
}

type radioFormData struct {
	pageData
	File          string
	Device        *config.RadioDevice
	IsNew         bool
	DetectedCards []system.SoundCard
	CTCSSTones    []string
}

func (s *Server) handleRadioNewForm(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if !isRadioFileParam(file) {
		http.NotFound(w, r)
		return
	}
	cards, _ := system.ListSoundCards()
	s.render(w, "radio_form.html", radioFormData{
		pageData:      pageData{LoggedIn: true},
		File:          file,
		Device:        &config.RadioDevice{},
		IsNew:         true,
		DetectedCards: cards,
		CTCSSTones:    standardCTCSSTones,
	})
}

func (s *Server) handleRadioEditForm(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	name := r.PathValue("name")
	if !isRadioFileParam(file) {
		http.NotFound(w, r)
		return
	}
	d, err := s.store.LoadRadioDevice(file, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "radio_form.html", radioFormData{
		pageData:   pageData{LoggedIn: true},
		File:       file,
		Device:     d,
		CTCSSTones: standardCTCSSTones,
	})
}

func radioDeviceFromForm(r *http.Request, name string) *config.RadioDevice {
	d := &config.RadioDevice{
		Name:           name,
		CarrierFrom:    r.FormValue("carrierfrom"),
		CTCSSFrom:      r.FormValue("ctcssfrom"),
		RXDemod:        r.FormValue("rxdemod"),
		TXPrelim:       r.FormValue("txprelim"),
		TXMixA:         r.FormValue("txmixa"),
		TXMixB:         r.FormValue("txmixb"),
		InvertPTT:      r.FormValue("invertptt"),
		TXCTCSSDefault: r.FormValue("txctcssdefault"),
		RXMixerSet:     r.FormValue("rxmixerset"),
		TXMixerSet:     r.FormValue("txmixerset"),
		RXBoost:        r.FormValue("rxboost"),
		HdwType:        r.FormValue("hdwtype"),
		Duplex3:        r.FormValue("duplex3"),
	}
	if name == "" {
		d.Name = r.FormValue("name")
	}
	return d
}

func (s *Server) handleRadioCreate(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if !isRadioFileParam(file) {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	d := radioDeviceFromForm(r, "")
	if err := s.store.SaveRadioDevice(file, d); err != nil {
		cards, _ := system.ListSoundCards()
		s.render(w, "radio_form.html", radioFormData{
			pageData:      flash("error", err.Error()),
			File:          file,
			Device:        d,
			IsNew:         true,
			DetectedCards: cards,
			CTCSSTones:    standardCTCSSTones,
		})
		return
	}
	http.Redirect(w, r, "/radio/"+file+"/"+d.Name, http.StatusSeeOther)
}

func (s *Server) handleRadioSave(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	name := r.PathValue("name")
	if !isRadioFileParam(file) {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	d := radioDeviceFromForm(r, name)
	if err := s.store.SaveRadioDevice(file, d); err != nil {
		s.render(w, "radio_form.html", radioFormData{
			pageData:   flash("error", err.Error()),
			File:       file,
			Device:     d,
			CTCSSTones: standardCTCSSTones,
		})
		return
	}
	http.Redirect(w, r, "/radio/"+file+"/"+name, http.StatusSeeOther)
}

func (s *Server) handleRadioDelete(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	name := r.PathValue("name")
	if !isRadioFileParam(file) {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteRadioDevice(file, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/radio?file="+file, http.StatusSeeOther)
}
