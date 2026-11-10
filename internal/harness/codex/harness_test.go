package codex

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/harness"
	"github.com/stretchr/testify/require"
)

// toolMessage wraps a tool result the way the agent records one.
func toolMessage(text string) fantasy.Message {
	return fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{fantasy.ToolResultPart{
			ToolCallID: "call",
			Output:     fantasy.ToolResultOutputContentText{Text: text},
		}},
	}
}

// resultTextOf returns the text of the first tool result of a message.
func resultTextOf(t *testing.T, message fantasy.Message) string {
	t.Helper()
	part, ok := message.Content[0].(fantasy.ToolResultPart)
	require.True(t, ok)
	text, ok := part.Output.(fantasy.ToolResultOutputContentText)
	require.True(t, ok)
	return text.Text
}

func TestNameIsTheCatalogName(t *testing.T) {
	t.Parallel()

	require.Equal(t, harness.Codex, NewHarness(0, true).Name())
	info, ok := harness.Lookup(NewHarness(0, true).Name())
	require.True(t, ok)
	require.Equal(t, "Codex", info.Label)
}

func TestSystemSpecCarriesTheOutputCap(t *testing.T) {
	t.Parallel()

	spec := NewHarness(2048, true).SystemSpec()
	require.Contains(t, spec, "2048 bytes")
	require.NotContains(t, spec, "{{max_output_bytes}}")
}

func TestShapeKeepsShortOutputs(t *testing.T) {
	t.Parallel()

	h := NewHarness(1024, true)
	message := toolMessage("short output")
	shaped := h.Shape(context.Background(), "s1", []fantasy.Message{message})

	require.Equal(t, "short output", resultTextOf(t, shaped[0]))
}

func TestShapeCutsLongOutputsInTheMiddle(t *testing.T) {
	t.Parallel()

	h := NewHarness(512, true)
	head := strings.Repeat("a", 150)
	body := strings.Repeat("m", 5000)
	tail := strings.Repeat("z", 150)
	shaped := h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(head + body + tail)})

	text := resultTextOf(t, shaped[0])
	require.LessOrEqual(t, len(text), 512, "the cap is a hard limit")
	require.True(t, strings.HasPrefix(text, "Warning: truncated output (original byte count: 5300)"), text)
	require.Contains(t, text, head)
	require.Contains(t, text, tail)
	require.NotContains(t, text, strings.Repeat("m", 100), "the middle is what gets dropped")
	require.Contains(t, text, "bytes truncated")
}

func TestShapeLeavesOtherMessagesAlone(t *testing.T) {
	t.Parallel()

	h := NewHarness(64, true)
	long := strings.Repeat("x", 500)
	shaped := h.Shape(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage(long),
		fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: long}}},
	})

	require.Equal(t, long, shaped[0].Content[0].(fantasy.TextPart).Text)
	require.Equal(t, long, shaped[1].Content[0].(fantasy.TextPart).Text)
}

func TestShapeIsIdempotent(t *testing.T) {
	t.Parallel()

	h := NewHarness(256, true)
	long := strings.Repeat("q", 4000)
	once := h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(long)})
	twice := h.Shape(context.Background(), "s1", once)

	require.Equal(t, resultTextOf(t, once[0]), resultTextOf(t, twice[0]))
}

func TestShapeKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	h := NewHarness(200, true)
	shaped := h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(strings.Repeat("д", 500))})

	text := resultTextOf(t, shaped[0])
	require.True(t, utf8.ValidString(text), "cutting must not split a rune")
}

// statusInt reads one key of the [Harness.Status] report.
func statusInt(t *testing.T, status, key string) int {
	t.Helper()
	for _, line := range strings.Split(status, "\n") {
		value, ok := strings.CutPrefix(line, key+" = ")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(value)
		require.NoError(t, err)
		return n
	}
	t.Fatalf("%q is missing from the status:\n%s", key, status)
	return 0
}

func TestStatusCountsTheSession(t *testing.T) {
	t.Parallel()

	h := NewHarness(128, true)
	long := strings.Repeat("y", 2000)
	first := toolMessage(long)
	second := toolMessage(long)

	h.Shape(context.Background(), "s1", []fantasy.Message{first})
	status, err := h.Status(context.Background(), "s1")
	require.NoError(t, err)
	require.Contains(t, status, "cap_bytes = 128")
	require.Contains(t, status, "window = session")
	perStep := statusInt(t, status, "removed_bytes")
	require.Equal(t, 1, statusInt(t, status, "truncated_outputs"))
	require.Positive(t, perStep)

	h.Shape(context.Background(), "s1", []fantasy.Message{first, second})
	status, err = h.Status(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, 2, statusInt(t, status, "truncated_outputs"), "an appended step adds to the session count")
	require.Equal(t, 2*perStep, statusInt(t, status, "removed_bytes"))

	h.Shape(context.Background(), "s1", []fantasy.Message{second})
	status, err = h.Status(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, 1, statusInt(t, status, "truncated_outputs"), "a replaced transcript starts the count over")
}

func TestInstructionsCanBeTurnedOff(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, NewHarness(1024, true).SystemSpec())

	h := NewHarness(1024, false)
	require.Empty(t, h.SystemSpec(), "the harness then only bounds tool output")
	require.Equal(t, 1024, h.cap(), "the cap still applies")
}
