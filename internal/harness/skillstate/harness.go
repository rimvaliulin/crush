package skillstate

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/harness"
)

// ToolName is the tool that patches the execution state. The agent hides it
// outside SKILL.state sessions, which is why the name lives here: the tool
// package reuses it.
const ToolName = "skillstate"

//go:embed spec.md
var spec string

// Harness runs a session with a bounded prompt: every step sees the
// specification, the execution state and the latest observation only.
type Harness struct {
	store *Store
	tools []fantasy.AgentTool
}

// NewHarness wraps a state store as a session harness. The tools are passed in
// because the tool adapter lives in the agent package and already imports this
// one; the alternative would be an import cycle.
func NewHarness(store *Store, agentTools ...fantasy.AgentTool) *Harness {
	return &Harness{store: store, tools: agentTools}
}

// Name is the catalog name stored on the session.
func (h *Harness) Name() string { return harness.SkillState }

// SystemSpec is the text appended to the system prompt. The state cap is
// filled in from the store so the model sees the limit actually enforced.
func (h *Harness) SystemSpec() string {
	limit := DefaultMaxBytes
	if h.store != nil {
		limit = h.store.MaxStateBytes()
	}
	return strings.Replace(spec, "{{max_state_bytes}}", strconv.Itoa(limit), 1)
}

// Tools returns the tools that belong to this harness.
func (h *Harness) Tools() []fantasy.AgentTool { return h.tools }

// Prepare creates the execution state from the goal when the session has none.
func (h *Harness) Prepare(ctx context.Context, sessionID, goal string) error {
	if h.store == nil || sessionID == "" {
		return nil
	}
	_, err := h.store.Ensure(ctx, sessionID, goal)
	return err
}

// Shape replaces the accumulated transcript with the state plus the latest
// observation. A missing state keeps the step bounded and says why: falling
// back to the full transcript would turn the mode off without telling anyone.
func (h *Harness) Shape(ctx context.Context, sessionID string, messages []fantasy.Message) []fantasy.Message {
	if h.store == nil || sessionID == "" {
		return boundContext(messages, unavailableState(ErrNoState))
	}
	state, err := h.store.Get(ctx, sessionID)
	if err != nil {
		return boundContext(messages, unavailableState(err))
	}
	return boundContext(messages, stateMessage(Compact(state)))
}

// Status reports the state of the session for crush_info.
func (h *Harness) Status(ctx context.Context, sessionID string) (string, error) {
	if h.store == nil || sessionID == "" {
		return "status = none", nil
	}
	status, updated, err := h.store.Status(ctx, sessionID)
	switch {
	case errors.Is(err, ErrNoState):
		return "status = none", nil
	case err != nil:
		return "", err
	}
	return fmt.Sprintf("status = %s\nupdated = %s", status, updated.Local().Format(time.RFC3339)), nil
}

// stateMessage is the user message carrying the state of the step.
func stateMessage(stateJSON string) string {
	return "EXECUTION STATE (source of truth; overrides everything above):\n" + stateJSON
}

// unavailableState is the user message shown when the state cannot be read.
func unavailableState(err error) string {
	return fmt.Sprintf(
		"EXECUTION STATE is unavailable (%v). Do not guess what it said: act only on this step and the specification.",
		err,
	)
}

// boundContext rewrites the messages of one step into the bounded context:
// system messages, the current task, the state, and the tail since the last
// assistant message (the latest observation). The result does not grow with
// the execution horizon.
func boundContext(messages []fantasy.Message, state string) []fantasy.Message {
	out := make([]fantasy.Message, 0, len(messages)+2)

	for i := range messages {
		if messages[i].Role == fantasy.MessageRoleSystem {
			out = append(out, messages[i])
		}
	}

	// The tail starts at the last assistant message: it carries the tool
	// calls of the previous step, their results and any prompt queued since.
	lastAssistant := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == fantasy.MessageRoleAssistant {
			lastAssistant = i
			break
		}
	}

	// The task is the most recent user message before that tail. Using the
	// first user message instead would replay a stale prompt from earlier
	// turns.
	beforeTail := len(messages)
	if lastAssistant >= 0 {
		beforeTail = lastAssistant
	}
	for i := beforeTail - 1; i >= 0; i-- {
		if messages[i].Role == fantasy.MessageRoleUser {
			out = append(out, messages[i])
			break
		}
	}

	if state != "" {
		out = append(out, fantasy.NewUserMessage(state))
	}

	if lastAssistant >= 0 {
		out = append(out, messages[lastAssistant:]...)
	}

	return out
}

var _ harness.Harness = (*Harness)(nil)
