// Package tts generates sound files from typed text using Piper
// (https://github.com/OHF-Voice/piper1-gpl) — a free, fully offline
// neural text-to-speech engine with a documented, well-supported
// Raspberry Pi target, matching this app's whole approach of shelling
// out to real local tools (sox, 818-prog, asterisk itself) rather than
// depending on a cloud service or API key. Piper needs a downloaded
// voice model (a ".onnx" file, plus a same-named ".onnx.json" config it
// reads automatically) for each voice; this package only lists whichever
// models already exist in the configured voices directory and runs the
// piper binary against one — it never downloads voices itself.
package tts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Voice is one downloaded Piper voice model.
type Voice struct {
	// Name is the bare model name with no directory or extension, e.g.
	// "en_US-lessac-medium" — how the voice is identified in the UI and
	// submitted back in a form.
	Name string
	// ModelPath is the full path to the voice's ".onnx" file, passed to
	// piper's --model flag. Never taken directly from user input — always
	// resolved by looking up a submitted Name against ListVoices, so a
	// request can't point piper at an arbitrary file.
	ModelPath string
}

// ListVoices returns every Piper voice model found in dir, sorted by
// name. A missing directory (no voices downloaded yet) is not an error —
// it just means the "Create from text" UI has nothing to offer until the
// operator downloads one (e.g. `python3 -m piper.download_voices
// en_US-lessac-medium`).
func ListVoices(dir string) ([]Voice, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Voice
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".onnx") {
			continue
		}
		out = append(out, Voice{
			Name:      strings.TrimSuffix(e.Name(), ".onnx"),
			ModelPath: filepath.Join(dir, e.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// FindVoice looks up name (as returned by ListVoices) in dir, returning
// ok=false if there's no such voice — the only safe way a submitted voice
// name should ever be turned into a model path.
func FindVoice(dir, name string) (Voice, bool, error) {
	voices, err := ListVoices(dir)
	if err != nil {
		return Voice{}, false, err
	}
	for _, v := range voices {
		if v.Name == name {
			return v, true, nil
		}
	}
	return Voice{}, false, nil
}

// synthesizeTimeout bounds one piper run. Generous: even a long paragraph
// synthesizes in well under this on a Pi, and this also covers a slow SD
// card write.
const synthesizeTimeout = 30 * time.Second

// Synthesize renders text using the voice model at modelPath, via `<tool>
// --model <modelPath> --output_file <tmpfile>` with text piped to stdin
// (Piper's own documented CLI form), returning the generated WAV audio.
// piper's own exit code and combined output are the source of truth for
// whether synthesis actually worked, not an assumption — the same
// pattern this app already uses for sox and 818-prog. output is returned
// even on success, so a caller can show it for troubleshooting.
func Synthesize(ctx context.Context, tool, modelPath, text string) (wav []byte, output string, err error) {
	tmp, err := os.CreateTemp("", "piper-*.wav")
	if err != nil {
		return nil, "", fmt.Errorf("stage output file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(ctx, synthesizeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, tool, "--model", modelPath, "--output_file", tmpPath)
	cmd.Stdin = strings.NewReader(text)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	output = out.String()
	if runErr != nil {
		return nil, output, fmt.Errorf("%s: %w", tool, runErr)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, output, fmt.Errorf("read synthesized audio: %w", err)
	}
	return data, output, nil
}
