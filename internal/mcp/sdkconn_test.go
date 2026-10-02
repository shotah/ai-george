package mcp

import (
	"context"
	"sync"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKConn_ListCallClose(t *testing.T) {
	ctx := context.Background()
	type in struct {
		Name string `json:"name"`
	}
	type out struct {
		Greeting string `json:"greeting"`
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mem", Version: "v1"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "greet", Description: "hi"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input in) (*mcpsdk.CallToolResult, out, error) {
		return nil, out{Greeting: "Hello " + input.Name}, nil
	})
	t1, t2 := mcpsdk.NewInMemoryTransports()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = server.Connect(ctx, t1, nil)
	}()
	t.Cleanup(func() { wg.Wait() })

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "george-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn := &sdkConn{session: session}

	tools, err := conn.ListTools(ctx)
	if err != nil || len(tools) != 1 || tools[0].OriginalName != "greet" {
		t.Fatalf("tools=%v err=%v", tools, err)
	}
	got, err := conn.CallTool(ctx, "greet", map[string]any{"name": "george"})
	if err != nil || got == "" {
		t.Fatalf("call=%q err=%v", got, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSDKConn_ImageContentStaysOffModelText(t *testing.T) {
	ctx := context.Background()
	type in struct {
		Prompt string `json:"prompt"`
	}
	type out struct{}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "image", Version: "v1"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "photo_generate", Description: "draw"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input in) (*mcpsdk.CallToolResult, out, error) {
		sum := `{"prompt":"` + input.Prompt + `","bytes":3}`
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: sum},
				&mcpsdk.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"},
			},
		}, out{}, nil
	})
	t1, t2 := mcpsdk.NewInMemoryTransports()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = server.Connect(ctx, t1, nil)
	}()
	t.Cleanup(func() { wg.Wait() })

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "george-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn := &sdkConn{session: session}
	t.Cleanup(func() { _ = conn.Close() })

	got, err := conn.CallTool(ctx, "photo_generate", map[string]any{"prompt": "red bike"})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"prompt":"red bike","bytes":3}` {
		t.Fatalf("summary=%q", got)
	}
}
