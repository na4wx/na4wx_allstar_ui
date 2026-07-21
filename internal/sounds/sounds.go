// Package sounds manages the audio files an app_rpt node can reference
// for station ID and telemetry playback (e.g. rpt.conf's "idrecording",
// or a telemetry entry like "patchup=rpt/callproceeding").
//
// Two directories are involved, and they're treated very differently:
//
//   - The "custom" directory (on a stock HamVoIP install,
//     /etc/asterisk/local/) is where an operator's own recordings live —
//     confirmed on real hardware to already hold a working
//     "node-id.gsm", the station ID file that install's rpt.conf
//     idrecording most likely points at. This app can list, upload to,
//     and reference files here.
//   - The "stock" directory (/var/lib/asterisk/sounds/rpt/) is
//     app_rpt's own prompt library — confirmed on real hardware to hold
//     ~90 files in a mix of formats. This app only ever reads this
//     directory to offer existing prompts (like "rpt/callproceeding")
//     as pick-list options; it never writes to it. That library isn't
//     this app's to own, and rewriting one of its files could break
//     stock behavior in ways this app has no way to verify.
package sounds

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// soundExtensions lists the file extensions this package recognizes as
// playable audio, based on what's actually present in a real HamVoIP
// sounds directory (a mix of .gsm/.ulaw/.pcm was confirmed there).
// Extensions not in this list are ignored when listing, so e.g. the
// custom directory's own README/example/script files never show up as
// if they were sounds.
var soundExtensions = map[string]bool{
	".gsm":  true,
	".ulaw": true,
	".ul":   true,
	".alaw": true,
	".al":   true,
	".wav":  true,
	".pcm":  true,
	".sln":  true,
	".g722": true,
}

// File is one playable sound this app knows about.
type File struct {
	// Name is the bare reference name with no directory prefix or
	// extension, e.g. "node-id" or "callproceeding" — how the file is
	// identified in the UI.
	Name string
	// Ref is exactly what app_rpt/rpt.conf wants written into a field
	// like idrecording or a telemetry value to play this file, e.g.
	// "node-id" for a custom file or "rpt/callproceeding" for a stock
	// one (both without an extension — Asterisk appends whichever
	// format it actually wants at playback time).
	Ref string
	// Custom is true for a file in the custom (uploadable/manageable)
	// directory, false for a stock library file (read-only reference).
	Custom bool
}

// Store manages the custom sound directory and lists the stock one.
type Store struct {
	customDir string
	stockDir  string
	soxTool   string
}

// New builds a Store. customDir and stockDir are the two directories
// described in the package doc comment. soxTool is the sox binary path,
// or a bare name ("sox") to resolve via PATH — matching how this app
// already shells out to other external tools it doesn't reimplement
// (818-prog, asterisk itself).
func New(customDir, stockDir, soxTool string) *Store {
	return &Store{customDir: customDir, stockDir: stockDir, soxTool: soxTool}
}

// listDir scans dir for recognized sound files, deduplicating by base
// name — a real sound is often stored in more than one format (e.g. both
// callproceeding.gsm and a .ulaw copy), and app_rpt's own reference to
// it never includes an extension, so from this app's perspective those
// are one sound, not two.
func listDir(dir string, custom bool, refPrefix string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]bool)
	var out []File
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !soundExtensions[ext] {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, File{Name: name, Ref: refPrefix + name, Custom: custom})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ListCustom lists the operator's own uploaded/managed sounds.
func (s *Store) ListCustom() ([]File, error) {
	return listDir(s.customDir, true, "")
}

// ListStock lists app_rpt's own prompt library, read-only. Ref is
// prefixed "rpt/" to match how those prompts are actually referenced
// elsewhere in rpt.conf (e.g. "patchup=rpt/callproceeding") — this
// assumes the stock directory is app_rpt's own "rpt" sound subdirectory,
// true on every HamVoIP install this app has been run against.
func (s *Store) ListStock() ([]File, error) {
	return listDir(s.stockDir, false, "rpt/")
}

// ListAll returns custom sounds first (what an operator is most likely
// to want, and what they can also delete/replace) followed by the stock
// library, for a combined picker.
func (s *Store) ListAll() ([]File, error) {
	custom, err := s.ListCustom()
	if err != nil {
		return nil, err
	}
	stock, err := s.ListStock()
	if err != nil {
		return nil, err
	}
	return append(custom, stock...), nil
}

// validNameRe restricts an uploaded sound's name to characters safe to
// use directly as a filename — no path separators or traversal, nothing
// a shell or filesystem could reinterpret.
var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidName reports whether name is safe to use as a custom sound's
// filename.
func ValidName(name string) bool {
	return name != "" && len(name) <= 64 && validNameRe.MatchString(name)
}

// uploadTimeout bounds the sox conversion. Generous: sox transcoding a
// short voice recording is fast, but this also covers however long it
// takes the Pi to read a large upload off a slow SD card.
const uploadTimeout = 30 * time.Second

// Upload saves src (an uploaded audio file in whatever format the
// operator recorded it in — typically WAV) as name in the custom
// directory, transcoding it with sox to 8kHz mono mu-law (".ulaw"),
// which every sox build supports natively with no optional codec
// library, unlike GSM. That matches an existing convention already
// present in a real HamVoIP sounds directory (plenty of its own stock
// prompts are .ulaw), so this isn't introducing a new format, just
// picking the one this app can most reliably produce.
//
// sox's own exit code and combined output are the source of truth for
// whether the conversion actually worked, not an assumption — the same
// pattern this app already uses for the external 818-prog tool. output
// is returned even on success, so a caller can show it for
// troubleshooting.
func (s *Store) Upload(ctx context.Context, name string, src io.Reader) (output string, err error) {
	if !ValidName(name) {
		return "", fmt.Errorf("sound name must be letters, numbers, - or _ only")
	}
	if err := os.MkdirAll(s.customDir, 0o755); err != nil {
		return "", fmt.Errorf("create sounds directory: %w", err)
	}

	tmp, err := os.CreateTemp(s.customDir, ".upload-*")
	if err != nil {
		return "", fmt.Errorf("stage upload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once sox has consumed it

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return "", fmt.Errorf("save upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("save upload: %w", err)
	}

	dest := filepath.Join(s.customDir, name+".ulaw")
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()

	// -t ul (raw mu-law) is specified explicitly on the output rather
	// than relying on sox to infer a format from the ".ulaw" extension —
	// unlike ".wav", that extension isn't one of sox's own recognized
	// format names, and this app would rather be explicit than depend on
	// guessing right.
	cmd := exec.CommandContext(ctx, s.soxTool, tmpPath, "-r", "8000", "-c", "1", "-t", "ul", dest)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	output = out.String()
	if runErr != nil {
		os.Remove(dest) // sox may have written a partial file before failing
		return output, fmt.Errorf("%s: %w", s.soxTool, runErr)
	}
	return output, nil
}

// DeleteCustom removes one of the operator's own sound files (all
// extensions sharing that base name, in case more than one format was
// ever produced for it) from the custom directory. Never touches the
// stock library — there is no delete path for it at all.
func (s *Store) DeleteCustom(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid sound name")
	}
	entries, err := os.ReadDir(s.customDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var lastErr error
	removed := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if base != name {
			continue
		}
		if err := os.Remove(filepath.Join(s.customDir, e.Name())); err != nil {
			lastErr = err
		} else {
			removed = true
		}
	}
	if lastErr != nil {
		return lastErr
	}
	if !removed {
		return fmt.Errorf("sound %q not found", name)
	}
	return nil
}
