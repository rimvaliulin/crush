// Package codex runs a session the way the Codex CLI does: the whole
// transcript is replayed every step, the model reads the Codex base
// instructions, and each tool output is bounded before it reaches the model.
//
// The instruction text is a port of openai/codex (codex-rs/protocol/src/
// prompts/base_instructions/default.md, Apache-2.0), rewritten for the tools
// Crush provides. Codex features that do not exist here - code mode, sandbox
// and approval injections, multi-agent tools, hosted web search, server-side
// compaction - are deliberately left out.
package codex

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/harness"
)

// DefaultMaxOutputBytes bounds one tool output. Codex stores 10_000 bytes of
// content plus a 20% allowance for the surrounding JSON; the same total here.
const DefaultMaxOutputBytes = 12000

// markerReserve is room kept for the truncation marker inside the cap.
const markerReserve = 64

//go:embed spec.md
var spec string

// Harness is the Codex-style harness of one session. It keeps no per-session
// data: the transcript stays as it is, only observations are bounded.
type Harness struct {
	maxOutputBytes int
	instructions   bool

	mu    sync.Mutex
	stats map[string]outputStats
}

// outputStats accumulates what shaping the transcript cut, per session.
type outputStats struct {
	truncated int
	removed   int
	// countedThrough is how many messages of the transcript it covers. The
	// transcript grows by appending, so counting the messages beyond it is
	// what makes the counters cumulative without counting a step twice.
	countedThrough int
}

// NewHarness builds the harness. A non-positive cap falls back to
// DefaultMaxOutputBytes; instructions adds the Codex text to the system prompt.
func NewHarness(maxOutputBytes int, instructions bool) *Harness {
	h := &Harness{maxOutputBytes: maxOutputBytes, instructions: instructions, stats: make(map[string]outputStats)}
	if h.maxOutputBytes <= 0 {
		h.maxOutputBytes = DefaultMaxOutputBytes
	}
	return h
}

// Name is the catalog name stored on the session.
func (h *Harness) Name() string { return harness.Codex }

// SystemSpec is the Codex instruction text, with the tool output cap filled in
// from the configured limit. It is empty when the instructions are turned off:
// the transcript is still bounded, the model just reads Crush's own prompt only.
func (h *Harness) SystemSpec() string {
	if !h.instructions {
		return ""
	}
	return strings.Replace(spec, "{{max_output_bytes}}", strconv.Itoa(h.cap()), 1)
}

// Prepare does nothing: this harness keeps no state of its own.
func (h *Harness) Prepare(ctx context.Context, sessionID, goal string) error { return nil }

// Tools is empty: the harness changes what the model sees, not what it can do.
func (h *Harness) Tools() []fantasy.AgentTool { return nil }

// Shape bounds every tool output and keeps the rest of the transcript as it is.
// The counters it keeps are cumulative for the session: a step only counts the
// messages it has not seen yet, and the transcript shrinking (a summary
// replacing it) starts the count over.
func (h *Harness) Shape(ctx context.Context, sessionID string, messages []fantasy.Message) []fantasy.Message {
	total, countedThrough := h.counters(sessionID, len(messages))
	out := make([]fantasy.Message, 0, len(messages))
	for i, message := range messages {
		if message.Role != fantasy.MessageRoleTool {
			out = append(out, message)
			continue
		}
		bounded, step := h.boundMessage(message)
		if i >= countedThrough {
			total.truncated += step.truncated
			total.removed += step.removed
		}
		out = append(out, bounded)
	}
	h.record(sessionID, total, len(messages))
	return out
}

// Status reports the output cap and what shaping the transcript cut so far in
// the session.
func (h *Harness) Status(ctx context.Context, sessionID string) (string, error) {
	h.mu.Lock()
	stats := h.stats[sessionID]
	h.mu.Unlock()
	return fmt.Sprintf(
		"cap_bytes = %d\ntruncated_outputs = %d\nremoved_bytes = %d\nwindow = session",
		h.cap(), stats.truncated, stats.removed,
	), nil
}

// counters returns what the session has cut so far and how much of the
// transcript it covers. A transcript shorter than that boundary was replaced,
// so the counters start over.
func (h *Harness) counters(sessionID string, messageCount int) (outputStats, int) {
	if sessionID == "" {
		return outputStats{}, 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	stats := h.stats[sessionID]
	if messageCount < stats.countedThrough {
		return outputStats{}, 0
	}
	return stats, stats.countedThrough
}

// record stores the cumulative counters and the boundary they cover.
func (h *Harness) record(sessionID string, stats outputStats, messageCount int) {
	if sessionID == "" {
		return
	}
	stats.countedThrough = messageCount
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stats[sessionID] = stats
}

// cap is the effective output limit.
func (h *Harness) cap() int {
	if h.maxOutputBytes <= 0 {
		return DefaultMaxOutputBytes
	}
	return h.maxOutputBytes
}

// boundMessage rewrites the text results of a tool message that exceed the cap.
func (h *Harness) boundMessage(message fantasy.Message) (fantasy.Message, outputStats) {
	var stats outputStats
	content := make([]fantasy.MessagePart, 0, len(message.Content))
	for _, part := range message.Content {
		result, ok := part.(fantasy.ToolResultPart)
		if !ok {
			content = append(content, part)
			continue
		}
		text, ok := resultText(result.Output)
		if !ok {
			content = append(content, result)
			continue
		}
		bounded, removed, truncated := h.bound(text)
		if truncated {
			result.Output = fantasy.ToolResultOutputContentText{Text: bounded}
			stats.truncated++
			stats.removed += removed
		}
		content = append(content, result)
	}
	message.Content = content
	return message, stats
}

// resultText is the text of a tool result, if it carries one.
func resultText(output fantasy.ToolResultOutputContent) (string, bool) {
	text, ok := output.(fantasy.ToolResultOutputContentText)
	return text.Text, ok
}

// bound cuts an oversized output in the middle, the way Codex does: the model
// keeps the beginning and the end, sees how much was dropped and how large the
// original was. The result never exceeds the cap, so bounding it again is a
// no-op.
func (h *Harness) bound(text string) (string, int, bool) {
	limit := h.cap()
	if len(text) <= limit {
		return text, 0, false
	}
	header := fmt.Sprintf("Warning: truncated output (original byte count: %d)\n", len(text))
	budget := limit - len(header) - markerReserve
	if budget < 2 {
		// The cap is too small for the warning and the marker: keep the front
		// of the output, so the result still fits and bounding it again is a
		// no-op.
		cut := headBoundary(text, limit)
		return text[:cut], len(text) - cut, true
	}
	headEnd := headBoundary(text, budget/2)
	tailStart := tailBoundary(text, len(text)-(budget-budget/2))
	removed := tailStart - headEnd
	marker := fmt.Sprintf("\n…%d bytes truncated…\n", removed)
	return header + text[:headEnd] + marker + text[tailStart:], removed, true
}

// headBoundary is the largest rune boundary at or before index.
func headBoundary(text string, index int) int {
	index = min(index, len(text))
	for index > 0 && !utf8.RuneStart(text[index]) {
		index--
	}
	return index
}

// tailBoundary is the smallest rune boundary at or after index.
func tailBoundary(text string, index int) int {
	index = max(index, 0)
	for index < len(text) && !utf8.RuneStart(text[index]) {
		index++
	}
	return index
}

var _ harness.Harness = (*Harness)(nil)
