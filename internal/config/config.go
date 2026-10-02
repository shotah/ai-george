// Package config loads and validates george configuration from the environment.
// Boot is fail-fast: missing required values return a clear error.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Retired are the variables the assistant build read. A coding build does
// not, and an old env file that still sets one should fail boot rather
// than quietly do nothing.
var Retired = []string{
	"CHANNEL",
	"TELEGRAM_BOT_TOKEN", "TELEGRAM_ALLOWED_USERS", "TELEGRAM_ERROR_REPORTING",
	"DISCORD_BOT_TOKEN", "DISCORD_ALLOWED_USERS",
	"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN", "SLACK_ALLOWED_USERS",
	"PENDANT_MAILBOX_URL", "PENDANT_BEARER", "PENDANT_ALLOWED_USERS",
	"CRON_ENABLED", "CRON_TZ", "CRON_MAX_JOBS", "CRON_TICK_SECONDS", "DAILY_PLANNER_AT",
	"WATCH_ENABLED", "WATCH_MAX",
	"EXAMPLES_QTY", "EXAMPLES_START_HOUR", "EXAMPLES_END_HOUR", "EXAMPLES_SKIP_RECENT_MINUTES",
	"MEMORY_CONSOLIDATE_MINUTES", "COALESCE_SETTLE_MS",
}

// Config is the complete env-driven configuration surface.
// Secrets and scalars live here; structure (persona, MCP manifest) is files in Dir().
type Config struct {
	LLMBaseURL string `env:"LLM_BASE_URL,required"`
	LLMAPIKey  string `env:"LLM_API_KEY,required"`
	LLMModel   string `env:"LLM_MODEL,required"`
	// LLMMaxTokens caps completion output (incl. tool-call args). 0 = provider default.
	LLMMaxTokens int `env:"LLM_MAX_TOKENS" envDefault:"4096"`
	// LLMReasoningEffort is sent as reasoning_effort when non-empty (Ollama/Qwen:
	// "none" disables thinking so max_tokens is not eaten by hidden chain-of-thought).
	LLMReasoningEffort string `env:"LLM_REASONING_EFFORT"`
	// LLMSystemFold is how system blocks reach the wire: auto/one (a single
	// leading system message), or many (the agent layout).
	LLMSystemFold string `env:"LLM_SYSTEM_FOLD" envDefault:"auto"`

	// Unset, these default to Dir(), DataHome(), and Dir()/mcp.toml.
	PersonaDir  string `env:"PERSONA_DIR"`
	DataDir     string `env:"DATA_DIR"`
	MCPManifest string `env:"MCP_MANIFEST"`

	HistoryMaxMessages int `env:"HISTORY_MAX_MESSAGES" envDefault:"200"`
	HistoryMaxTokens   int `env:"HISTORY_MAX_TOKENS" envDefault:"32000"` // estimated (chars/4); older turns fold into Facts/Voice
	// HistoryStripFillers drops a small function-word list from older user
	// history at prompt time. Last 40 messages stay verbatim; assistant turns
	// are never stripped. SQLite stays verbatim.
	HistoryStripFillers bool `env:"HISTORY_STRIP_FILLERS" envDefault:"true"`
	ToolResultMaxChars  int  `env:"TOOL_RESULT_MAX_CHARS" envDefault:"6000"`
	ToolMaxIterations   int  `env:"TOOL_MAX_ITERATIONS" envDefault:"25"`
	// ToolSchemaMaxTokens is an optional hard cap on estimated tool-schema tokens
	// (chars/4 of name+description+parameters). 0 = log estimate only.
	ToolSchemaMaxTokens int `env:"TOOL_SCHEMA_MAX_TOKENS" envDefault:"0"`
	// MCPEnableForce is a comma-separated list of prefixes that stay published
	// when dynamic_tools is on (e.g. fs,git,shell).
	MCPEnableForce string `env:"MCP_ENABLE_FORCE"`

	// ToolsEnabled controls whether tool schemas are sent to the model.
	// false omits MCP, memory_*, and web_search from every completion — required
	// for models that reject tools (e.g. Ollama gemma3). The memory backend may
	// still start; only the agent tool surface is cleared.
	ToolsEnabled bool `env:"TOOLS_ENABLED" envDefault:"true"`

	// WebSearchEnabled publishes builtin web_search (Brave Search HTTP).
	WebSearchEnabled bool `env:"WEB_SEARCH_ENABLED" envDefault:"true"`
	// BraveSearchAPIKey is the Brave Search subscription token.
	BraveSearchAPIKey string `env:"BRAVE_SEARCH_API_KEY"`

	// SelfNotesEnabled lets the agent keep SELF.md in PERSONA_DIR: a self_note
	// tool for jotting how the human likes the work done, plus a distill pass
	// on /new that folds what the dying session taught into the file so it
	// survives the reset. Auto-disables when PERSONA_DIR is not writable.
	SelfNotesEnabled bool `env:"SELF_NOTES_ENABLED" envDefault:"true"`

	MemoryEnabled bool   `env:"MEMORY_ENABLED" envDefault:"true"`
	MemoryBackend string `env:"MEMORY_BACKEND" envDefault:"builtin"`

	StreamReplies bool `env:"STREAM_REPLIES" envDefault:"true"`

	// ShowThinking controls whether chain-of-thought is rendered in the
	// stream. On by default; set false for a quieter terminal. Pair with LLM_REASONING_EFFORT=none on
	// slow local models so CoT is not generated (and therefore not shown).
	// Needs STREAM_REPLIES=true. Does not change model-side think on/off.
	ShowThinking bool `env:"SHOW_THINKING" envDefault:"true"`

	// ToolTrace controls user-visible tool activity when STREAM_REPLIES is on.
	// compact = Making Calls: ✓, ✗ (default); full = → name / ✓ timing lines;
	// off = hide tool activity entirely. Journal logs are unaffected.
	ToolTrace string `env:"TOOL_TRACE" envDefault:"compact"`

	// SpinupNoticeMS posts a "still working" line once a turn has gone this
	// long without model output. The first turn after start posts immediately.
	// Needs STREAM_REPLIES=true. 0 disables. Default 4000ms.
	SpinupNoticeMS int `env:"SPINUP_NOTICE_MS" envDefault:"4000"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// Load reads EnvFile into the environment (the process env wins), parses it
// into Config, and validates it. Returns a descriptive error on any failure.
func Load() (*Config, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}
	if err := CheckRetired(os.LookupEnv); err != nil {
		return nil, err
	}
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	if cfg.PersonaDir == "" {
		cfg.PersonaDir = Dir()
	}
	if cfg.DataDir == "" {
		cfg.DataDir = DataHome()
	}
	if cfg.MCPManifest == "" {
		cfg.MCPManifest = filepath.Join(Dir(), "mcp.toml")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CheckRetired fails on the first Retired variable lookup reports as set.
func CheckRetired(lookup func(string) (string, bool)) error {
	for _, name := range Retired {
		if _, ok := lookup(name); ok {
			return fmt.Errorf("%s: removed in the coding build; delete it from the environment", name)
		}
	}
	return nil
}

// Validate checks cross-field rules after env parsing.
func (c *Config) Validate() error {
	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	c.MemoryBackend = strings.TrimSpace(c.MemoryBackend)

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL: must be debug|info|warn|error, got %q", c.LogLevel)
	}

	c.ToolTrace = strings.ToLower(strings.TrimSpace(c.ToolTrace))
	switch c.ToolTrace {
	case "", "full", "compact", "off":
		if c.ToolTrace == "" {
			c.ToolTrace = "compact"
		}
	default:
		return fmt.Errorf("TOOL_TRACE: must be full|compact|off, got %q", c.ToolTrace)
	}

	if c.LLMMaxTokens < 0 {
		return fmt.Errorf("LLM_MAX_TOKENS: must be >= 0, got %d", c.LLMMaxTokens)
	}
	c.LLMReasoningEffort = strings.TrimSpace(c.LLMReasoningEffort)
	if c.LLMReasoningEffort != "" {
		switch c.LLMReasoningEffort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("LLM_REASONING_EFFORT: must be none|minimal|low|medium|high|xhigh|max, got %q", c.LLMReasoningEffort)
		}
	}
	c.LLMSystemFold = strings.ToLower(strings.TrimSpace(c.LLMSystemFold))
	switch c.LLMSystemFold {
	case "", "auto", "one", "many":
	default:
		return fmt.Errorf("LLM_SYSTEM_FOLD: must be auto|one|many, got %q", c.LLMSystemFold)
	}
	if c.HistoryMaxMessages < 1 {
		return fmt.Errorf("HISTORY_MAX_MESSAGES: must be >= 1, got %d", c.HistoryMaxMessages)
	}
	if c.HistoryMaxTokens < 1 {
		return fmt.Errorf("HISTORY_MAX_TOKENS: must be >= 1, got %d", c.HistoryMaxTokens)
	}
	if c.ToolResultMaxChars < 1 {
		return fmt.Errorf("TOOL_RESULT_MAX_CHARS: must be >= 1, got %d", c.ToolResultMaxChars)
	}
	if c.ToolMaxIterations < 1 {
		return fmt.Errorf("TOOL_MAX_ITERATIONS: must be >= 1, got %d", c.ToolMaxIterations)
	}
	if c.ToolSchemaMaxTokens < 0 {
		return fmt.Errorf("TOOL_SCHEMA_MAX_TOKENS: must be >= 0, got %d", c.ToolSchemaMaxTokens)
	}
	if c.SpinupNoticeMS < 0 {
		return fmt.Errorf("SPINUP_NOTICE_MS: must be >= 0, got %d", c.SpinupNoticeMS)
	}
	if err := validateMemoryBackend(c.MemoryBackend); err != nil {
		return err
	}

	if strings.TrimSpace(c.PersonaDir) == "" {
		return fmt.Errorf("PERSONA_DIR: must not be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("DATA_DIR: must not be empty")
	}
	if strings.TrimSpace(c.MCPManifest) == "" {
		return fmt.Errorf("MCP_MANIFEST: must not be empty")
	}
	if strings.TrimSpace(c.LLMBaseURL) == "" {
		return fmt.Errorf("LLM_BASE_URL: must not be empty")
	}
	if strings.TrimSpace(c.LLMAPIKey) == "" {
		return fmt.Errorf("LLM_API_KEY: must not be empty")
	}
	if strings.TrimSpace(c.LLMModel) == "" {
		return fmt.Errorf("LLM_MODEL: must not be empty")
	}

	return nil
}

func validateMemoryBackend(backend string) error {
	if backend == "builtin" {
		return nil
	}
	if strings.HasPrefix(backend, "mcp:") {
		name := strings.TrimPrefix(backend, "mcp:")
		if name == "" {
			return fmt.Errorf("MEMORY_BACKEND: mcp:<server-name> requires a server name")
		}
		return nil
	}
	return fmt.Errorf("MEMORY_BACKEND: must be %q or %q, got %q", "builtin", "mcp:<server-name>", backend)
}
