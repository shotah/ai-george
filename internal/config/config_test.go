package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/shotah/george/internal/config"
)

// setRequiredLLM sets the LLM_* trio and clears every retired name, so a
// developer shell that still exports one cannot fail an unrelated test.
func setRequiredLLM(t *testing.T) {
	t.Helper()
	for _, name := range config.Retired {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LLM_BASE_URL", "https://example.com/v1")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "test-model")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredLLM(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.PersonaDir != "/persona" {
		t.Errorf("PersonaDir = %q, want /persona", cfg.PersonaDir)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", cfg.DataDir)
	}
	if cfg.MCPManifest != "/etc/george/mcp.toml" {
		t.Errorf("MCPManifest = %q, want /etc/george/mcp.toml", cfg.MCPManifest)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("LLMMaxTokens = %d, want 4096", cfg.LLMMaxTokens)
	}
	if cfg.HistoryMaxMessages != 200 {
		t.Errorf("HistoryMaxMessages = %d, want 200", cfg.HistoryMaxMessages)
	}
	if cfg.HistoryMaxTokens != 32000 {
		t.Errorf("HistoryMaxTokens = %d, want 32000", cfg.HistoryMaxTokens)
	}
	if !cfg.HistoryStripFillers {
		t.Error("HistoryStripFillers = false, want true")
	}
	if cfg.ToolResultMaxChars != 6000 {
		t.Errorf("ToolResultMaxChars = %d, want 6000", cfg.ToolResultMaxChars)
	}
	if cfg.ToolMaxIterations != 25 {
		t.Errorf("ToolMaxIterations = %d, want 25", cfg.ToolMaxIterations)
	}
	if !cfg.ToolsEnabled {
		t.Error("ToolsEnabled = false, want true")
	}
	if !cfg.WebSearchEnabled {
		t.Error("WebSearchEnabled = false, want true")
	}
	if !cfg.MemoryEnabled {
		t.Error("MemoryEnabled = false, want true")
	}
	if cfg.MemoryBackend != "builtin" {
		t.Errorf("MemoryBackend = %q, want builtin", cfg.MemoryBackend)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoad_MissingRequiredLLM(t *testing.T) {
	setRequiredLLM(t)
	// Intentionally leave LLM_* unset / empty.
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for missing LLM_* vars")
	}
}

func TestLoad_MemoryBackendMCP(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("MEMORY_BACKEND", "mcp:custom-memory")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MemoryBackend != "mcp:custom-memory" {
		t.Errorf("MemoryBackend = %q, want mcp:custom-memory", cfg.MemoryBackend)
	}
}

func TestLoad_InvalidMemoryBackend(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("MEMORY_BACKEND", "redis")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for invalid memory backend")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for invalid log level")
	}
}

func TestLoad_Bounds(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("HISTORY_MAX_MESSAGES", "0")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for HISTORY_MAX_MESSAGES=0")
	}
}

func TestLoad_MoreValidation(t *testing.T) {
	setRequiredLLM(t)

	cases := []struct {
		key, val, want string
	}{
		{"LLM_MAX_TOKENS", "-1", "LLM_MAX_TOKENS"},
		{"HISTORY_MAX_TOKENS", "0", "HISTORY_MAX_TOKENS"},
		{"TOOL_RESULT_MAX_CHARS", "0", "TOOL_RESULT_MAX_CHARS"},
		{"TOOL_MAX_ITERATIONS", "0", "TOOL_MAX_ITERATIONS"},
		{"TOOL_SCHEMA_MAX_TOKENS", "-1", "TOOL_SCHEMA_MAX_TOKENS"},
		{"TOOL_TRACE", "verbose", "TOOL_TRACE"},
		{"LLM_SYSTEM_FOLD", "sideways", "LLM_SYSTEM_FOLD"},
		{"MEMORY_BACKEND", "mcp:", "MEMORY_BACKEND"},
		{"PERSONA_DIR", "   ", "PERSONA_DIR"},
		{"DATA_DIR", "   ", "DATA_DIR"},
		{"MCP_MANIFEST", "   ", "MCP_MANIFEST"},
		{"LOG_LEVEL", "DEBUG", ""}, // valid after normalize
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			setRequiredLLM(t)
			t.Setenv(tc.key, tc.val)
			cfg, err := config.Load()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if cfg.LogLevel != "debug" {
					t.Fatalf("LogLevel=%q", cfg.LogLevel)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoad_SystemFold(t *testing.T) {
	setRequiredLLM(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMSystemFold != "auto" {
		t.Fatalf("default LLMSystemFold=%q want auto", cfg.LLMSystemFold)
	}
	t.Setenv("LLM_SYSTEM_FOLD", " ONE ")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMSystemFold != "one" {
		t.Fatalf("LLMSystemFold=%q want one", cfg.LLMSystemFold)
	}
}

// A retired variable fails boot by name, even when empty: an old env file
// that still says CRON_ENABLED= is an old env file.
func TestLoad_RetiredVariableFails(t *testing.T) {
	for _, name := range []string{"CHANNEL", "TELEGRAM_BOT_TOKEN", "CRON_ENABLED", "CRON_TZ", "COALESCE_SETTLE_MS"} {
		t.Run(name, func(t *testing.T) {
			setRequiredLLM(t)
			t.Setenv(name, "")
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "removed") {
				t.Fatalf("err = %v, want %s removed", err, name)
			}
		})
	}
}

func TestCheckRetired(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	if err := config.CheckRetired(none); err != nil {
		t.Fatal(err)
	}
	slack := func(name string) (string, bool) { return "x", name == "SLACK_TOKEN" }
	if err := config.CheckRetired(slack); err != nil {
		t.Fatalf("a name george never read must pass: %v", err)
	}
}
