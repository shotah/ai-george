package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shotah/george/internal/agent"
	"github.com/shotah/george/internal/channel/stdio"
	"github.com/shotah/george/internal/config"
	"github.com/shotah/george/internal/mcp"
	"github.com/shotah/george/internal/mcpenable"
	"github.com/shotah/george/internal/memory"
	"github.com/shotah/george/internal/persona"
	"github.com/shotah/george/internal/provider"
	"github.com/shotah/george/internal/selfnote"
	"github.com/shotah/george/internal/session"
	"github.com/shotah/george/internal/websearch"
)

// run boots config, persona, sessions, MCP host, memory, provider, agent, and stdio.
func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	root, err := setGeorgeRoot()
	if err != nil {
		logger.Error("repo root", "err", err)
		return 1
	}
	if err := prependPath(config.BinDir()); err != nil {
		logger.Error("mcp bin dir on PATH", "err", err)
		return 1
	}

	logger.Info("george starting",
		"root", root,
		"version", version,
		"model", cfg.LLMModel,
		"max_tokens", cfg.LLMMaxTokens,
		"reasoning_effort", cfg.LLMReasoningEffort,
		"system_fold", cfg.LLMSystemFold,
		"persona_dir", cfg.PersonaDir,
		"data_dir", cfg.DataDir,
		"mcp_manifest", cfg.MCPManifest,
		"memory_enabled", cfg.MemoryEnabled,
		"memory_backend", cfg.MemoryBackend,
		"self_notes_enabled", cfg.SelfNotesEnabled,
		"web_search_enabled", cfg.WebSearchEnabled,
		"stream_replies", cfg.StreamReplies,
		"show_thinking", cfg.ShowThinking,
		"tool_trace", cfg.ToolTrace,
	)

	personaText, err := persona.Load(cfg.PersonaDir)
	if err != nil {
		logger.Error("persona load failed", "err", err)
		return 1
	}
	logger.Info("persona loaded", "chars", len(personaText))

	tzName, tzLoc, tzSource := persona.ResolveTimezone(personaText, "Local")
	logger.Info("human timezone", "tz", tzName, "source", tzSource)
	if strings.EqualFold(tzName, "UTC") {
		logger.Warn("human timezone is UTC; set Timezone in PERSONA.md (or TZ) to the human's IANA zone")
	}

	completer := provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel).
		WithMaxTokens(cfg.LLMMaxTokens).
		WithReasoningEffort(cfg.LLMReasoningEffort).
		WithSystemFold(cfg.LLMSystemFold)

	sessions, err := session.Open(cfg.DataDir, cfg.HistoryMaxMessages, cfg.HistoryMaxTokens)
	if err != nil {
		logger.Error("session store open failed", "err", err)
		return 1
	}
	sessions.WithSummarizer(&session.LLMSummarizer{Completer: completer})
	defer func() {
		if err := sessions.Close(); err != nil {
			logger.Error("session store close failed", "err", err)
		}
	}()
	logger.Info("session store ready", "path", filepath.Join(cfg.DataDir, "george.db"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Server `budget` counters share george.db so a monthly cap survives a
	// redeploy; the day rolls at the human's midnight, not UTC's.
	budgetStore, err := mcp.OpenBudgetDB(sessions.DB())
	if err != nil {
		logger.Error("mcp budget store open failed", "err", err)
		return 1
	}
	mcpHost, err := mcp.Start(ctx, mcp.Options{
		ManifestPath:   cfg.MCPManifest,
		Logger:         logger,
		ResultMaxChars: cfg.ToolResultMaxChars,
		BudgetStore:    budgetStore,
		Location:       tzLoc,
		SkipServer: func(spec mcp.ServerSpec) bool {
			return cfg.WebSearchEnabled && websearch.IsReplacedMCP(spec.Name, spec.Command)
		},
	})
	if err != nil {
		logger.Error("mcp host failed", "err", err)
		return 1
	}
	defer func() {
		if err := mcpHost.Close(); err != nil {
			logger.Error("mcp host close failed", "err", err)
		}
	}()

	var (
		memBackend memory.Memory
		memBuiltin *memory.Builtin
		hideServer string
		tools      agent.Tools = mcpHost
	)

	if cfg.MemoryEnabled {
		switch {
		case cfg.MemoryBackend == "builtin":
			memBuiltin, err = memory.OpenDB(sessions.DB())
			if err != nil {
				logger.Error("memory open failed", "err", err)
				return 1
			}
			memBackend = memBuiltin
			logger.Info("memory ready", "backend", "builtin")
		case strings.HasPrefix(cfg.MemoryBackend, "mcp:"):
			server := strings.TrimPrefix(cfg.MemoryBackend, "mcp:")
			adapter, err := memory.NewMCPAdapter(mcpHost, server)
			if err != nil {
				logger.Error("memory mcp adapter failed", "err", err)
				return 1
			}
			memBackend = adapter
			hideServer = server
			logger.Info("memory ready", "backend", "mcp", "server", server)
		default:
			logger.Error("memory backend unsupported", "backend", cfg.MemoryBackend)
			return 1
		}
		defer func() {
			if err := memBackend.Close(); err != nil {
				logger.Error("memory close failed", "err", err)
			}
		}()

		memTools := memory.Tools{Backend: memBackend}
		tools = memory.Composite{
			Memory:        memTools,
			Other:         mcpHost,
			HideMCPServer: hideServer,
		}
	}

	var selfStore *selfnote.Store
	if cfg.SelfNotesEnabled {
		selfStore, err = selfnote.Open(cfg.PersonaDir)
		if err != nil {
			// Read-only persona mounts are common; degrade instead of failing boot.
			logger.Warn("self-notes disabled (persona dir not writable)", "err", err)
		} else {
			tools = selfnote.Composite{
				Self:  selfnote.Tools{Store: selfStore},
				Other: tools,
			}
			logger.Info("self-notes ready", "file", filepath.Join(cfg.PersonaDir, selfnote.FileName))
			if text, err := persona.Load(cfg.PersonaDir); err != nil {
				logger.Warn("persona reload after SELF.md stamp failed", "err", err)
			} else {
				personaText = text
			}
			store := selfStore
			sessions.WithFoldHook(func(prior, next string) {
				ok, err := selfnote.GraduateVoice(store, prior, next)
				if err != nil {
					logger.Warn("self-note on trim failed", "err", err)
					return
				}
				if ok {
					logger.Info("self-note on trim", "note_graduated", true)
				}
			})
		}
	}

	if cfg.WebSearchEnabled {
		searchTools, err := websearch.Open(websearch.Options{
			APIKey: cfg.BraveSearchAPIKey,
		})
		if err != nil {
			logger.Warn("web search disabled", "err", err)
		} else {
			tools = websearch.Composite{
				Search: searchTools,
				Other:  tools,
			}
			logger.Info("web search ready")
		}
	}

	var enableStore *mcpenable.Store
	var enableForce mcpenable.Force
	if cfg.ToolsEnabled && mcpHost.DynamicTools() {
		enableStore, err = mcpenable.OpenDB(sessions.DB())
		if err != nil {
			logger.Error("mcp enable store failed", "err", err)
			return 1
		}
		base := tools
		enableForce = mcpenable.Force{
			Prefixes: append(mcpenable.ParseForceCSV(cfg.MCPEnableForce), mcpHost.ForcePrefixes()...),
		}
		tools = mcpenable.Composite{
			Enable: mcpenable.Tools{
				Store: enableStore,
				Index: func() []string { return mcpenable.Index(base.Tools()) },
			},
			Other: base,
		}
		logger.Info("mcp prefix enable on", "force_prefixes", enableForce.Prefixes)
	} else if cfg.ToolsEnabled {
		logger.Info("mcp prefix enable off (dynamic_tools = false); full catalog published")
	}

	agentTools := tools
	if !cfg.ToolsEnabled {
		agentTools = nil
		logger.Info("tools disabled (TOOLS_ENABLED=false); omitting tool schemas from model requests")
	} else {
		budget := mcp.EstimateSchemaBudget(tools.Tools())
		logger.Info("tool schema estimate",
			"tools", budget.Tools,
			"est_tokens", budget.EstTokens,
			"max_tokens", cfg.ToolSchemaMaxTokens,
		)
		for _, s := range budget.ByServer {
			logger.Info("tool schema by server",
				"server", s.Server,
				"tools", s.Tools,
				"est_tokens", s.EstTokens,
			)
		}
		if cfg.ToolSchemaMaxTokens > 0 && budget.EstTokens > cfg.ToolSchemaMaxTokens {
			logger.Error("tool schema exceeds TOOL_SCHEMA_MAX_TOKENS",
				"est_tokens", budget.EstTokens,
				"max_tokens", cfg.ToolSchemaMaxTokens,
			)
			return 1
		}
	}

	agentOpts := agent.Options{
		Persona:             personaText,
		Completer:           completer,
		Sessions:            sessions,
		Tools:               agentTools,
		Memory:              memBackend,
		Model:               cfg.LLMModel,
		MaxToolIters:        cfg.ToolMaxIterations,
		StreamReplies:       cfg.StreamReplies,
		ToolTrace:           cfg.ToolTrace,
		Logger:              logger,
		Location:            tzLoc,
		TZName:              tzName,
		SpinupNotice:        time.Duration(cfg.SpinupNoticeMS) * time.Millisecond,
		HistoryStripFillers: cfg.HistoryStripFillers,
		Enable:              enableStore,
		EnableForce:         enableForce,
	}
	if selfStore != nil {
		agentOpts.SelfNotes = selfStore
	}
	ag, err := agent.New(agentOpts)
	if err != nil {
		logger.Error("agent init failed", "err", err)
		return 1
	}
	if selfStore != nil {
		// SELF.md sits in the persona prefix; reload after every agent write so
		// the note takes effect on the next turn.
		selfStore.OnChange = func() {
			text, err := persona.Load(cfg.PersonaDir)
			if err != nil {
				logger.Error("persona reload after self-note failed", "err", err)
				return
			}
			ag.SetPersona(text)
			logger.Info("persona reloaded after self-note", "chars", len(text))
		}
	}
	ch := stdio.New()
	ch.StreamReplies = cfg.StreamReplies

	if runErr := ch.Run(ctx, ag.Handle); runErr != nil {
		logger.Error("stdio stopped", "err", runErr)
		return 1
	}
	logger.Info("george stopped")
	return 0
}

// newLogger builds the process logger.
func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	// stderr keeps the stdio REPL on stdout readable.
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}
