package skillstate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

const validState = `{"status":"active","goal":"ship it","constraints":["no force push"],"verified":{},"current":"plan","next":["write code"],"facts":{},"errors":{}}`

// testStore returns a store backed by a fresh database and a session to
// own its state, mirroring production where every state row is a session's.
func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Release(dir))
	})
	sess, err := session.NewService(db.New(conn), conn).Create(t.Context(), "test")
	require.NoError(t, err)
	return New(conn, 0), sess.ID
}

func seed(t *testing.T, store *Store, sessionID, body string) {
	t.Helper()
	require.NoError(t, db.New(store.conn).UpsertSkillState(context.Background(), db.UpsertSkillStateParams{
		SessionID: sessionID,
		State:     body,
	}))
}

func TestPatchMergesAndDeletes(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	seed(t, store, sessionID, validState)

	size, err := store.Patch(t.Context(), sessionID, map[string]any{
		"current":  "implement",
		"facts":    map[string]any{"go": "1.27"},
		"verified": map[string]any{"build": "go build ./... exit 0"},
	})
	require.NoError(t, err)
	require.Positive(t, size)

	state, err := store.Get(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "implement", state["current"])
	require.Equal(t, "ship it", state["goal"])
	require.Equal(t, map[string]any{"go": "1.27"}, state["facts"])

	_, err = store.Patch(t.Context(), sessionID, map[string]any{"facts": map[string]any{"go": nil}})
	require.NoError(t, err)

	state, err = store.Get(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, map[string]any{}, state["facts"])
}

func TestPatchRejectsGoalChange(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	seed(t, store, sessionID, validState)

	_, err := store.Patch(t.Context(), sessionID, map[string]any{"goal": "something else"})
	require.ErrorContains(t, err, "goal is immutable")

	state, loadErr := store.Get(t.Context(), sessionID)
	require.NoError(t, loadErr)
	require.Equal(t, "ship it", state["goal"])
}

func TestPatchRejectsMissingKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{"required key removed", map[string]any{"errors": nil}, "missing required key: errors"},
		{"wrong type", map[string]any{"current": []any{"x"}}, "key 'current' must be string"},
		{"list type", map[string]any{"next": "later"}, "key 'next' must be list"},
		{"status enum", map[string]any{"status": "running"}, "status must be one of"},
		{"empty goal", map[string]any{"goal": "  "}, "goal must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store, sessionID := testStore(t)
			seed(t, store, sessionID, validState)
			_, err := store.Patch(t.Context(), sessionID, tt.patch)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestPatchRejectsOversize(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	seed(t, store, sessionID, validState)

	big := make([]byte, DefaultMaxBytes)
	for i := range big {
		big[i] = 'x'
	}
	_, err := store.Patch(t.Context(), sessionID, map[string]any{"facts": map[string]any{"blob": string(big)}})
	require.ErrorContains(t, err, "cap")
}

func TestPatchRequiresState(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	_, err := store.Patch(t.Context(), sessionID, map[string]any{"current": "x"})
	require.ErrorIs(t, err, ErrNoState)
	require.ErrorContains(t, err, "execution state")
}

func TestEnsureCreatesTheStateFromThePrompt(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)

	state, err := store.Ensure(t.Context(), sessionID, "Переименуй STATE.json\nи прогони тесты")
	require.NoError(t, err)
	require.Equal(t, "active", state["status"])
	require.Equal(t, "Переименуй STATE.json", state["goal"])
	require.Empty(t, Validate(state, nil))

	// An existing state is never replaced: a new prompt keeps the old goal.
	again, err := store.Ensure(t.Context(), sessionID, "другая задача")
	require.NoError(t, err)
	require.Equal(t, "Переименуй STATE.json", again["goal"])
}

func TestEnsureStartsANewGoalAfterDone(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	seed(t, store, sessionID, `{"status":"done","goal":"старая"}`)

	state, err := store.Ensure(t.Context(), sessionID, "новая задача")
	require.NoError(t, err)
	require.Equal(t, "active", state["status"])
	require.Equal(t, "новая задача", state["goal"])
}

func TestEnsureIsolatesSessions(t *testing.T) {
	t.Parallel()

	store, first := testStore(t)
	_, err := store.Ensure(t.Context(), first, "первая цель")
	require.NoError(t, err)

	second, err := session.NewService(db.New(store.conn), store.conn).Create(t.Context(), "second")
	require.NoError(t, err)

	state, err := store.Ensure(t.Context(), second.ID, "вторая цель")
	require.NoError(t, err)
	require.Equal(t, "вторая цель", state["goal"], "a session never inherits another goal")

	firstState, err := store.Get(t.Context(), first)
	require.NoError(t, err)
	require.Equal(t, "первая цель", firstState["goal"])
}

func TestEnsureRejectsCorruptState(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	seed(t, store, sessionID, `{broken`)

	_, err := store.Ensure(t.Context(), sessionID, "задача")
	require.Error(t, err)

	row, readErr := db.New(store.conn).GetSkillState(t.Context(), sessionID)
	require.NoError(t, readErr)
	require.Equal(t, `{broken`, row.State, "the stored state must stay untouched")
}

func TestDeleteDropsTheState(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	_, err := store.Ensure(t.Context(), sessionID, "цель")
	require.NoError(t, err)

	require.NoError(t, store.deleteState(t.Context(), sessionID))
	_, err = store.Get(t.Context(), sessionID)
	require.ErrorIs(t, err, ErrNoState)
}

func TestGoalLine(t *testing.T) {
	t.Parallel()

	require.Equal(t, "первая строка", goalLine("\n  первая строка \nвторая"))
	require.Equal(t, "unspecified", goalLine("   \n"))
	require.Len(t, []rune(goalLine(strings.Repeat("я", 300))), 200)
}

func TestParseRejectsNonObject(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`[1,2]`))
	require.Error(t, err)

	_, err = Parse([]byte(`{broken`))
	require.ErrorContains(t, err, "not valid JSON")
}

func TestValidateWithoutOriginalSkipsImmutability(t *testing.T) {
	t.Parallel()

	state, err := Parse([]byte(validState))
	require.NoError(t, err)
	require.Empty(t, Validate(state, nil))
}

func TestPatchSerialisesConcurrentWrites(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	seed(t, store, sessionID, validState)

	const writers = 16
	var wg sync.WaitGroup
	failures := make(chan error, writers)
	for i := range writers {
		wg.Go(func() {
			if _, err := store.Patch(t.Context(), sessionID, map[string]any{
				"facts": map[string]any{fmt.Sprintf("key-%d", i): "value"},
			}); err != nil {
				failures <- err
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}

	state, err := store.Get(t.Context(), sessionID)
	require.NoError(t, err)
	require.Len(t, state["facts"], writers, "a concurrent patch was lost")
}

func TestStateGoesAwayWithTheSession(t *testing.T) {
	t.Parallel()

	store, sessionID := testStore(t)
	_, err := store.Ensure(t.Context(), sessionID, "цель")
	require.NoError(t, err)

	sessions := session.NewService(db.New(store.conn), store.conn)
	require.NoError(t, sessions.Delete(t.Context(), sessionID))

	_, err = store.Get(t.Context(), sessionID)
	require.ErrorIs(t, err, ErrNoState, "the state must follow the session")
}

func TestPatchRespectsConfiguredCap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Release(dir))
	})
	sess, err := session.NewService(db.New(conn), conn).Create(t.Context(), "test")
	require.NoError(t, err)

	store := New(conn, 128)
	require.Equal(t, 128, store.MaxStateBytes())
	_, err = store.Ensure(t.Context(), sess.ID, "ship it")
	require.NoError(t, err)

	_, err = store.Patch(t.Context(), sess.ID, map[string]any{
		"facts": map[string]any{"blob": strings.Repeat("x", 256)},
	})
	require.ErrorContains(t, err, "cap 128")
}
