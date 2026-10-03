//go:build !windows

package stdio_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/shotah/george/internal/channel"
	"github.com/shotah/george/internal/channel/stdio"
)

func TestChannel_InterruptCancelsTurnOnly(t *testing.T) {
	in := strings.NewReader("long job\nnext\n/q\n")
	var errOut bytes.Buffer
	ch := &stdio.Channel{In: in, Out: &bytes.Buffer{}, Err: &errOut}
	var seen []string
	err := ch.Run(context.Background(), func(ctx context.Context, msg channel.Message) (string, error) {
		seen = append(seen, msg.Text)
		if msg.Text != "long job" {
			return "ok", nil
		}
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			// The agent reports a cancelled turn as an empty reply, not an error.
			return "", nil
		case <-time.After(5 * time.Second):
			t.Fatal("SIGINT did not cancel the turn")
			return "", nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(seen, ",") != "long job,next" {
		t.Fatalf("seen=%v", seen)
	}
	if !strings.Contains(errOut.String(), "cancelled") {
		t.Fatalf("errOut=%q", errOut.String())
	}
}
