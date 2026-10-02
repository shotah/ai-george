package main

import (
	"bytes"
	"os"
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
		if newLogger(level) == nil {
			t.Fatalf("newLogger(%q) nil", level)
		}
	}
}

func TestRun_BadConfig(t *testing.T) {
	t.Setenv("GEORGE_CONFIG_DIR", t.TempDir())
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	if code := run(); code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
}
