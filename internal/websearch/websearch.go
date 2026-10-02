// Package websearch is the harness builtin for web search.
//
// Search is a first-class agent capability (like memory and cron), not an MCP
// grant. The model calls unprefixed web_search; george GETs Brave Search and
// returns titles, URLs, and snippets. No second model, no Gemini grounding.
package websearch

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// ToolName is the builtin exposed to the model (not MCP-prefixed).
const ToolName = "web_search"

const (
	defaultEndpoint = "https://api.search.brave.com/res/v1/web/search"
	defaultNum      = 8
	httpTimeout     = 15 * time.Second
	tokenHeader     = "X-Subscription-Token"
)

// Options is the Brave Search client (BRAVE_SEARCH_API_KEY).
type Options struct {
	APIKey     string
	Endpoint   string       // tests; default Brave web search
	HTTPClient *http.Client // tests
}

type searchService struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

func errNeedAPIKey() error {
	return errors.New("BRAVE_SEARCH_API_KEY is required for web_search")
}

// Open builds an in-process Brave Search client. The HTTP call happens on Search.
func Open(opts Options) (Tools, error) {
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		return Tools{}, errNeedAPIKey()
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return Tools{
		svc: &searchService{
			apiKey:     apiKey,
			endpoint:   endpoint,
			httpClient: client,
		},
	}, nil
}

// IsSearchTool reports the builtin name and the alias models invent for it.
func IsSearchTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolName, "google_search":
		return true
	default:
		return false
	}
}
