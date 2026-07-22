package config

// ScheduleEntry is one row of an app_rpt "schedule" stanza in rpt.conf —
// app_rpt's own native cron-like mechanism. The key is a macro number
// (referencing an entry in the node's own "macro" stanza — see
// FunctionMacro/Node.Macro); Asterisk itself dials that macro's DTMF
// value when the current time matches TimeSpec, entirely on its own, no
// external process required. TimeSpec is "MM HH DayOfMonth MonthOfYear
// DayOfWeek" using only single numeric values or "*" wildcards — app_rpt's
// own docs are explicit that this is not real cron syntax: no ranges,
// lists, or step values are supported.
type ScheduleEntry struct {
	MacroNum string
	TimeSpec string
}

// ListScheduleEntries returns a schedule stanza's entries in file order.
func (s *Store) ListScheduleEntries(section string) ([]ScheduleEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	kv := f.SectionKeys(section)
	out := make([]ScheduleEntry, 0, len(kv))
	for _, p := range kv {
		out = append(out, ScheduleEntry{MacroNum: p.Key, TimeSpec: p.Value})
	}
	return out, nil
}

// SetScheduleEntry adds or updates one schedule entry, creating the
// section if it doesn't exist yet.
func (s *Store) SetScheduleEntry(section, macroNum, timeSpec string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.EnsureSection(section)
	f.Set(section, macroNum, timeSpec)
	return s.save(RptConfFile, f)
}

// DeleteScheduleEntry removes one schedule entry.
func (s *Store) DeleteScheduleEntry(section, macroNum string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.Delete(section, macroNum)
	return s.save(RptConfFile, f)
}
