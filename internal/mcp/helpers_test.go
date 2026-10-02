package mcp

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSchemaToMap(t *testing.T) {
	m, err := schemaToMap(nil)
	if err != nil || m["type"] != "object" {
		t.Fatalf("%v %#v", err, m)
	}
	in := map[string]any{"type": "object", "properties": map[string]any{}}
	m, err = schemaToMap(in)
	if err != nil || m["type"] != "object" {
		t.Fatal(err)
	}
	type sch struct {
		Type string `json:"type"`
	}
	m, err = schemaToMap(sch{Type: "string"})
	if err != nil || m["type"] != "string" {
		t.Fatalf("%v %#v", err, m)
	}
}

func TestContentToString(t *testing.T) {
	if contentToString(nil) != "" {
		t.Fatal("nil")
	}
	res := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "hello"}},
	}
	if contentToString(res) != "hello" {
		t.Fatalf("got %q", contentToString(res))
	}
	res = &mcpsdk.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	if contentToString(res) == "" {
		t.Fatal("expected structured json")
	}

	png := bytes.Repeat([]byte{0x89, 0x50, 0x4e, 0x47}, 400)
	res = &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: `{"prompt":"red bike","bytes":8}`},
			&mcpsdk.ImageContent{Data: png, MIMEType: "image/png"},
		},
	}
	got := contentToString(res)
	if got != `{"prompt":"red bike","bytes":8}` {
		t.Fatalf("text+image: %q", got)
	}
	if strings.Contains(got, `"type":"image"`) || strings.Contains(got, "iVBOR") {
		t.Fatal("must not dump ImageContent JSON or base64 into the model prompt")
	}

	res = &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"}},
	}
	if got := contentToString(res); got != "[image] 1 picture(s)" {
		t.Fatalf("image-only: %q", got)
	}
}

func TestLineLoggerWrite(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	l := newLineLogger(log, "demo")
	n, err := l.Write([]byte("one\n\ntwo\n"))
	if err != nil || n == 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
}
