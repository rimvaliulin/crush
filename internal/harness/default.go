package harness

import (
	"context"

	"charm.land/fantasy"
)

// Default is the plain agent: full transcript, no specification, no extra
// tools. Every session runs a harness; this one is what a session gets when the
// user picks nothing.
type Default struct{}

// NewDefault returns the default harness.
func NewDefault() *Default { return &Default{} }

// Name is the catalog name stored on the session.
func (d *Default) Name() string { return DefaultName }

// SystemSpec is empty: the default agent adds nothing to the system prompt.
func (d *Default) SystemSpec() string { return "" }

// Prepare does nothing: the default agent needs no per-session setup.
func (d *Default) Prepare(ctx context.Context, sessionID, goal string) error { return nil }

// Shape keeps the transcript as it is.
func (d *Default) Shape(ctx context.Context, sessionID string, messages []fantasy.Message) []fantasy.Message {
	return messages
}

// Tools is empty: the default agent owns no tool.
func (d *Default) Tools() []fantasy.AgentTool { return nil }

// Status reports nothing beyond the name.
func (d *Default) Status(ctx context.Context, sessionID string) (string, error) {
	return "status = none", nil
}

var _ Harness = (*Default)(nil)
