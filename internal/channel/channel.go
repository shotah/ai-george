// Package channel defines the Channel interface for inbound/outbound messaging.
package channel

import (
	"context"
)

// Message is an inbound user message from a channel.
type Message struct {
	SessionID string
	UserID    string
	Text      string
}

// AgentSession is the default conversation id when the caller names none.
const AgentSession = "george"

// HandleFailedText is what the channel says when Handle returns an error.
const HandleFailedText = "sorry — something went wrong handling that message"

// Handler processes one inbound message and returns reply text.
type Handler func(ctx context.Context, msg Message) (reply string, err error)

// Channel delivers messages until the context is cancelled or a fatal error.
type Channel interface {
	Run(ctx context.Context, handle Handler) error
}
