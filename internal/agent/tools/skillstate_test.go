package tools

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/harness/skillstate"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

func skillStateToolFixture(t *testing.T) (*skillstate.Store, string) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Release(dir))
	})
	sess, err := session.NewService(db.New(conn), conn).Create(t.Context(), "test")
	require.NoError(t, err)
	store := skillstate.New(conn, 0)
	_, err = store.Ensure(t.Context(), sess.ID, "ship it")
	require.NoError(t, err)
	return store, sess.ID
}

func runSkillStateTool(t *testing.T, store *skillstate.Store, sessionID string, params SkillStateParams) fantasy.ToolResponse {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, sessionID)
	response, err := NewSkillStateTool(store).Run(ctx, fantasy.ToolCall{
		Name:  SkillStateToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return response
}

func TestSkillStateToolPatches(t *testing.T) {
	t.Parallel()

	store, sessionID := skillStateToolFixture(t)
	response := runSkillStateTool(t, store, sessionID, SkillStateParams{Patch: map[string]any{
		"current": "implement",
		"next":    []any{"run tests"},
	}})

	require.False(t, response.IsError, response.Content)
	require.Contains(t, response.Content, `"current":"implement"`)

	state, err := store.Get(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "implement", state["current"])
	require.Equal(t, "ship it", state["goal"])
}

func TestSkillStateToolRejectsGoalChange(t *testing.T) {
	t.Parallel()

	store, sessionID := skillStateToolFixture(t)
	response := runSkillStateTool(t, store, sessionID, SkillStateParams{Patch: map[string]any{"goal": "other"}})

	require.True(t, response.IsError)
	require.Contains(t, response.Content, "goal is immutable")
}

func TestSkillStateToolRequiresPatch(t *testing.T) {
	t.Parallel()

	store, sessionID := skillStateToolFixture(t)
	response := runSkillStateTool(t, store, sessionID, SkillStateParams{})

	require.True(t, response.IsError)
	require.Contains(t, response.Content, "patch is required")
}

func TestSkillStateToolRequiresSession(t *testing.T) {
	t.Parallel()

	store, _ := skillStateToolFixture(t)
	input, err := json.Marshal(SkillStateParams{Patch: map[string]any{"current": "x"}})
	require.NoError(t, err)
	response, err := NewSkillStateTool(store).Run(context.Background(), fantasy.ToolCall{
		Name:  SkillStateToolName,
		Input: string(input),
	})
	require.NoError(t, err)

	require.True(t, response.IsError)
	require.Contains(t, response.Content, "no session")
}
