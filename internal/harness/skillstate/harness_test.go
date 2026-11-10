package skillstate

import (
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

// testHarness returns a harness whose session exists but has no state yet.
func testHarness(t *testing.T) (*Harness, string) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Release(dir))
	})
	sess, err := session.NewService(db.New(conn), conn).Create(t.Context(), "test")
	require.NoError(t, err)
	return NewHarness(New(conn, 0)), sess.ID
}

func stepMessages(steps int) []fantasy.Message {
	messages := []fantasy.Message{fantasy.NewSystemMessage("system")}
	for i := range steps {
		messages = append(messages,
			fantasy.Message{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("task %d", i)}}},
			fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("call %d", i)}}},
			fantasy.Message{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("observation %d", i)}}},
		)
	}
	return messages
}

func TestShapeIsBoundedAndCarriesTheState(t *testing.T) {
	t.Parallel()

	h, sessionID := testHarness(t)
	require.NoError(t, h.Prepare(t.Context(), sessionID, "ship it"))

	short := h.Shape(t.Context(), sessionID, stepMessages(1))
	long := h.Shape(t.Context(), sessionID, stepMessages(100))

	require.Len(t, short, len(long), "the step context must not grow with the horizon")
	require.Contains(t, messageText(long[2]), "EXECUTION STATE")
	require.Contains(t, messageText(long[2]), "ship it")
}

func TestShapeStaysBoundedWithoutState(t *testing.T) {
	t.Parallel()

	h, sessionID := testHarness(t)

	// No Prepare: the state row does not exist. The step must stay bounded
	// and say so instead of replaying the transcript.
	out := h.Shape(t.Context(), sessionID, stepMessages(50))

	require.Len(t, out, 5)
	require.Contains(t, messageText(out[2]), "unavailable")
}

func TestPrepareIsIdempotent(t *testing.T) {
	t.Parallel()

	h, sessionID := testHarness(t)
	require.NoError(t, h.Prepare(t.Context(), sessionID, "ship it"))
	require.NoError(t, h.Prepare(t.Context(), sessionID, "ship it"))

	state, err := h.store.Get(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "ship it", state["goal"])
}
