package harness

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

type stubHarness struct {
	name  string
	tools []fantasy.AgentTool
}

func (s stubHarness) Name() string                                  { return s.name }
func (s stubHarness) SystemSpec() string                            { return "" }
func (s stubHarness) Prepare(context.Context, string, string) error { return nil }
func (s stubHarness) Shape(_ context.Context, _ string, msgs []fantasy.Message) []fantasy.Message {
	return msgs
}
func (s stubHarness) Tools() []fantasy.AgentTool                     { return s.tools }
func (s stubHarness) Status(context.Context, string) (string, error) { return "", nil }

func tool(name string) fantasy.AgentTool {
	return fantasy.NewAgentTool(name, "", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse(""), nil
	})
}

func TestFilterToolsKeepsOnlyTheActiveHarness(t *testing.T) {
	t.Parallel()

	registry := New(
		stubHarness{name: "one", tools: []fantasy.AgentTool{tool("one_tool")}},
		stubHarness{name: "two", tools: []fantasy.AgentTool{tool("two_tool")}},
	)
	available := []fantasy.AgentTool{tool("bash"), tool("one_tool"), tool("two_tool")}

	plain := registry.FilterTools("", available)
	require.Len(t, plain, 1)
	require.Equal(t, "bash", plain[0].Info().Name)

	one := registry.FilterTools("one", available)
	require.Len(t, one, 2)
	require.Equal(t, "bash", one[0].Info().Name)
	require.Equal(t, "one_tool", one[1].Info().Name)
}

func TestAddAppendsHarnessTools(t *testing.T) {
	t.Parallel()

	registry := New(stubHarness{name: "one", tools: []fantasy.AgentTool{tool("one_tool")}})
	all := registry.Add([]fantasy.AgentTool{tool("bash")})

	require.Len(t, all, 2)
	require.Equal(t, "one_tool", all[1].Info().Name)
}

func TestGetReadsTheEmptyNameAsTheDefaultHarness(t *testing.T) {
	t.Parallel()

	registry := New(NewDefault(), stubHarness{name: "one"})
	require.Equal(t, DefaultName, registry.Get("").Name())
	require.Equal(t, DefaultName, registry.Get(DefaultName).Name())
	require.Nil(t, registry.Get("unknown"))
	require.NotNil(t, registry.Get("one"))
}

func TestGetWithoutADefaultHarnessReturnsNil(t *testing.T) {
	t.Parallel()

	require.Nil(t, New(stubHarness{name: "one"}).Get(""))
}

func TestDefaultHarnessChangesNothing(t *testing.T) {
	t.Parallel()

	h := NewDefault()
	require.Equal(t, DefaultName, h.Name())
	require.Empty(t, h.SystemSpec())
	require.NoError(t, h.Prepare(context.Background(), "session", "goal"))
	require.Empty(t, h.Tools())

	messages := []fantasy.Message{fantasy.NewUserMessage("hi")}
	require.Equal(t, messages, h.Shape(context.Background(), "session", messages))
}

func TestCatalogOffersTheDefaultHarness(t *testing.T) {
	t.Parallel()

	info, ok := Lookup(DefaultName)
	require.True(t, ok)
	require.Equal(t, "Default", info.Label)
	require.Contains(t, Names(), DefaultName)
}

func TestFilterToolsReadsTheEmptyNameAsTheDefaultHarness(t *testing.T) {
	t.Parallel()

	registry := New(NewDefault(), stubHarness{name: "one", tools: []fantasy.AgentTool{tool("one_tool")}})
	plain := registry.FilterTools("", []fantasy.AgentTool{tool("bash"), tool("one_tool")})

	require.Len(t, plain, 1)
	require.Equal(t, "bash", plain[0].Info().Name)
}
