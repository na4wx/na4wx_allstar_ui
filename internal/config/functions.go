package config

// FunctionMacro is one DTMF command mapping from an app_rpt "functions"
// stanza in rpt.conf: the digit sequence a user dials on the radio
// (after the node's funcchar) mapped to the app_rpt command it runs,
// e.g. "1" -> "ilink,3,2000" (connect permanently to 2000, on some
// configs) or more commonly a generic "ilink,%s" macro form. Digits are
// only unique within the section they're defined in — a node picks
// which functions stanza it uses via its own "functions" field (see
// Node.Functions), so callers pass that section name explicitly rather
// than assuming a fixed "functions" section.
type FunctionMacro struct {
	Digits  string
	Command string
}

// ListFunctionMacros returns a functions stanza's DTMF mappings in file
// order.
func (s *Store) ListFunctionMacros(section string) ([]FunctionMacro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	kv := f.SectionKeys(section)
	out := make([]FunctionMacro, 0, len(kv))
	for _, p := range kv {
		out = append(out, FunctionMacro{Digits: p.Key, Command: p.Value})
	}
	return out, nil
}

// SetFunctionMacro adds or updates one DTMF mapping.
func (s *Store) SetFunctionMacro(section, digits, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.EnsureSection(section)
	f.Set(section, digits, command)
	return s.save(RptConfFile, f)
}

// DeleteFunctionMacro removes one DTMF mapping.
func (s *Store) DeleteFunctionMacro(section, digits string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.Delete(section, digits)
	return s.save(RptConfFile, f)
}
