// Package stdio implements a REPL channel for local development.
package stdio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/slash"
)

const userID = "local"

// Channel is an interactive line-oriented REPL on stdin/stdout.
type Channel struct {
	In            io.Reader
	Out           io.Writer
	Err           io.Writer
	StreamReplies bool
	// SessionID is the conversation every line belongs to (the repo id).
	// Empty is channel.AgentSession.
	SessionID string
}

// New returns a Channel bound to process stdio.
func New() *Channel {
	return &Channel{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}

// Run reads lines, invokes handle, and prints replies until EOF or ctx cancel.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	in := c.In
	if in == nil {
		in = os.Stdin
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := c.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	sessionID := c.SessionID
	if sessionID == "" {
		sessionID = channel.AgentSession
	}

	_, _ = fmt.Fprintln(errOut, slash.ReadyLine())

	scanner := bufio.NewScanner(in)
	// Allow long paste dumps in the REPL.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		_, _ = fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("stdio: read: %w", err)
			}
			_, _ = fmt.Fprintln(out)
			return nil // EOF
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch strings.ToLower(line) {
		case "/quit", "/exit", "/q":
			return nil
		}

		var stream *printStream
		handleCtx := ctx
		if c.StreamReplies {
			stream = newPrintStream(out)
			handleCtx = channel.WithReplyWriter(ctx, stream)
		}

		reply, err := handle(handleCtx, channel.Message{
			SessionID: sessionID,
			UserID:    userID,
			Text:      line,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			_, _ = fmt.Fprintf(errOut, "error: %v\n", err)
			continue
		}
		if stream != nil && stream.Started() {
			_ = stream.Finish(ctx, reply)
			continue
		}
		if reply != "" {
			_, _ = fmt.Fprintln(out, reply)
		}
	}
}
