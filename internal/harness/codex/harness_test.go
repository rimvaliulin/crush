package codex

import (
	"context"
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

	require.Equal(t, harness.Codex, NewHarness(0).Name())
	info, ok := harness.Lookup(NewHarness(0).Name())
	require.True(t, ok)
	require.Equal(t, "Codex", info.Label)
}

func TestSystemSpecCarriesTheOutputCap(t *testing.T) {
	t.Parallel()

	spec := NewHarness(2048).SystemSpec()
	require.Contains(t, spec, "2048 bytes")
	require.NotContains(t, spec, "{{max_output_bytes}}")
}

func TestShapeKeepsShortOutputs(t *testing.T) {
	t.Parallel()

	h := NewHarness(1024)
	message := toolMessage("short output")
	shaped := h.Shape(context.Background(), "s1", []fantasy.Message{message})

	require.Equal(t, "short output", resultTextOf(t, shaped[0]))
}

func TestShapeCutsLongOutputsInTheMiddle(t *testing.T) {
	t.Parallel()

	h := NewHarness(512)
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

	h := NewHarness(64)
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

	h := NewHarness(256)
	long := strings.Repeat("q", 4000)
	once := h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(long)})
	twice := h.Shape(context.Background(), "s1", once)

	require.Equal(t, resultTextOf(t, once[0]), resultTextOf(t, twice[0]))
}

func TestShapeKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	h := NewHarness(200)
	shaped := h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(strings.Repeat("д", 500))})

	text := resultTextOf(t, shaped[0])
	require.True(t, utf8.ValidString(text), "cutting must not split a rune")
}

func TestStatusCountsTheLastStep(t *testing.T) {
	t.Parallel()

	h := NewHarness(128)
	long := strings.Repeat("y", 2000)
	h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(long), toolMessage(long)})

	status, err := h.Status(context.Background(), "s1")
	require.NoError(t, err)
	require.Contains(t, status, "cap_bytes = 128")
	require.Contains(t, status, "truncated_outputs = 2")
	require.Contains(t, status, "window = last step")

	h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage("short")})
	status, err = h.Status(context.Background(), "s1")
	require.NoError(t, err)
	require.Contains(t, status, "truncated_outputs = 0", "the counters describe the latest step")
}

func TestShapeHandlesACapTooSmallForTheWarning(t *testing.T) {
	t.Parallel()

	h := NewHarness(24)
	long := strings.Repeat("w", 1000)
	once := h.Shape(context.Background(), "s1", []fantasy.Message{toolMessage(long)})
	twice := h.Shape(context.Background(), "s1", once)

	text := resultTextOf(t, once[0])
	require.LessOrEqual(t, len(text), 24)
	require.True(t, utf8.ValidString(text))
	require.Equal(t, text, resultTextOf(t, twice[0]), "a cut output stays stable")
}
