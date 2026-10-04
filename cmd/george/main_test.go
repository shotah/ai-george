package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPrintHelp(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	printHelp()
	_ = w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	help := buf.String()
	if !strings.Contains(help, "george") || !strings.Contains(help, "tools-fetch") {
		t.Fatalf("help = %q", help)
	}
	for _, gone := range []string{"auth", "status", "doctor", "daemon"} {
		if strings.Contains(help, gone) {
			t.Fatalf("help still offers %q: %q", gone, help)
		}
	}
}

func TestNewLogger_Levels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "other"} {
		if newLogger(level, 0, t.TempDir(), io.Discard) == nil {
			t.Fatalf("newLogger(%q) nil", level)
		}
	}
}

func TestNewLogger_TermQuietFileFull(t *testing.T) {
	for _, tc := range []struct {
		verbose  int
		wantInfo bool
	}{{0, false}, {1, true}, {2, true}} {
		dir := t.TempDir()
		var term bytes.Buffer
		log := newLogger("info", tc.verbose, dir, &term)
		log.Info("routine step")
		log.Warn("real trouble")
		if got := strings.Contains(term.String(), "routine step"); got != tc.wantInfo {
			t.Fatalf("-v=%d: info on terminal = %v, want %v: %q", tc.verbose, got, tc.wantInfo, term.String())
		}
		if !strings.Contains(term.String(), "real trouble") {
			t.Fatalf("-v=%d: warn missing from terminal: %q", tc.verbose, term.String())
		}
		file, err := os.ReadFile(filepath.Join(dir, "george.log"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(file), `"msg":"routine step"`) || !strings.Contains(string(file), `"msg":"real trouble"`) {
			t.Fatalf("-v=%d: file log = %q", tc.verbose, file)
		}
	}
}

func TestVerbosity(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		rest []string
		n    int
	}{
		{nil, nil, 0},
		{[]string{"-v"}, nil, 1},
		{[]string{"run", "-vv"}, []string{"run"}, 2},
		{[]string{"-v", "run", "--verbose", "-v"}, []string{"run"}, 2},
		{[]string{"tools-plan", "x"}, []string{"tools-plan", "x"}, 0},
	} {
		rest, n := verbosity(tc.in)
		if n != tc.n || !slices.Equal(rest, tc.rest) {
			t.Fatalf("verbosity(%q) = %q, %d; want %q, %d", tc.in, rest, n, tc.rest, tc.n)
		}
	}
}

func TestRun_BadConfig(t *testing.T) {
	t.Setenv("GEORGE_CONFIG_DIR", t.TempDir())
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	if code := run(0); code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
}
