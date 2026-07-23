package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hamvoipconfiggui/internal/automation"
	"hamvoipconfiggui/internal/soundschedule"
	"hamvoipconfiggui/internal/system"
)

// soundSchedulePollInterval is how often the sound-schedule ticker
// checks for a matching entry. Deliberately finer than 60s: a bare
// 60-second ticker started at an arbitrary process-start offset can
// drift across a minute boundary and skip a scheduled minute entirely.
const soundSchedulePollInterval = 15 * time.Second

// StartSoundSchedulePoller checks on soundSchedulePollInterval whether
// any scheduled sound entry (see internal/soundschedule) matches the
// current wall-clock minute, calling out to `asterisk -rx "rpt
// localplay/playback ..."` when one does. This is the GUI-side half of
// the "Automation" tab — unlike scheduled connect/disconnect (which uses
// app_rpt's own native scheduler and needs nothing running here), there
// is no native app_rpt mechanism to schedule arbitrary sound-file
// playback, so this ticker exists to fill that gap; it only fires while
// this process is running. lastFired is only ever touched from this one
// goroutine (ticks are handled sequentially, never concurrently with
// themselves), so it needs no locking of its own. Runs until ctx is
// cancelled. Call once, from main.
func (s *Server) StartSoundSchedulePoller(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(soundSchedulePollInterval)
		defer ticker.Stop()
		lastFired := make(map[string]string)
		s.checkSoundSchedule(ctx, lastFired)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkSoundSchedule(ctx, lastFired)
			}
		}
	}()
}

// checkSoundSchedule fires every entry that matches now and hasn't
// already fired for this exact minute, tracked via lastFired (entry ID ->
// the minute it last fired, truncated). A play failure is logged, not
// fatal — the next scheduled minute (or a different entry) should still
// get a chance to run.
func (s *Server) checkSoundSchedule(ctx context.Context, lastFired map[string]string) {
	entries, err := s.soundSchedule.List()
	if err != nil {
		return
	}
	now := time.Now()
	minuteKey := now.Truncate(time.Minute).Format(time.RFC3339)
	for _, e := range entries {
		if !e.Matches(now) {
			continue
		}
		if lastFired[e.ID] == minuteKey {
			continue
		}
		lastFired[e.ID] = minuteKey

		var playErr error
		if e.Reach == soundschedule.ReachNetwork {
			playErr = system.RptPlayback(ctx, s.asteriskBin, e.Node, e.File)
		} else {
			playErr = system.RptLocalPlay(ctx, s.asteriskBin, e.Node, e.File)
		}
		if playErr != nil {
			log.Printf("sound schedule: node %s file %s: %v", e.Node, e.File, playErr)
		}
	}
}

// populateNodeSoundSchedule fills data's "Scheduled sound playback" rows.
// Best-effort, like the rest of this page's supplementary data.
func (s *Server) populateNodeSoundSchedule(data *nodeFormData) {
	node := data.Node
	if node == nil || node.Number == "" {
		return
	}
	entries, err := s.soundSchedule.ListForNode(node.Number)
	if err != nil {
		return
	}
	data.SoundSchedules = entries
}

// handleNodeAutomationSoundSave adds one scheduled sound-playback entry.
// Unlike the connect/disconnect side, multiple selected weekdays stay a
// single entry (DaysOfWeek is a real list here — see
// soundschedule.Entry's doc comment for why that's fine for this half
// but not the native-scheduler half).
func (s *Server) handleNodeAutomationSoundSave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	file := strings.TrimSpace(r.FormValue("sound_file"))
	if file == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Pick a sound file to schedule"))
		return
	}
	reach := r.FormValue("reach")
	if reach != soundschedule.ReachNetwork {
		reach = soundschedule.ReachLocal
	}

	minute := strings.TrimSpace(r.FormValue("minute"))
	hour := strings.TrimSpace(r.FormValue("hour"))
	dom := strings.TrimSpace(r.FormValue("dom"))
	month := strings.TrimSpace(r.FormValue("month"))
	for _, v := range []string{minute, hour, dom, month} {
		if !automation.TimeFieldRe.MatchString(v) {
			s.renderNodeEditPage(w, r, number, flash("error", "Minute/hour/day-of-month/month must each be a single number or *"))
			return
		}
	}

	var weekdays []int
	for _, wd := range r.Form["weekday"] {
		n, err := strconv.Atoi(wd)
		if err != nil || n < 0 || n > 6 {
			s.renderNodeEditPage(w, r, number, flash("error", "Invalid day-of-week value"))
			return
		}
		weekdays = append(weekdays, n)
	}

	entry := soundschedule.Entry{
		Node:       number,
		File:       file,
		Reach:      reach,
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dom,
		Month:      month,
		DaysOfWeek: weekdays,
	}
	if err := s.soundSchedule.Save(entry); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeAutomationSoundDelete removes one scheduled sound-playback
// entry.
func (s *Server) handleNodeAutomationSoundDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	id := r.PathValue("id")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.soundSchedule.Delete(id); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}
