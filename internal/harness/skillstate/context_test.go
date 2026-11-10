package skillstate

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func messageText(message fantasy.Message) string {
	text := ""
	for _, part := range message.Content {
		if value, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
			text += value.Text
		}
	}
	return text
}

func conversation(steps int) []fantasy.Message {
	messages := []fantasy.Message{
		fantasy.NewSystemMessage("system"),
		fantasy.NewUserMessage("task"),
	}
	for i := range steps {
		messages = append(messages,
			fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("call %d", i)}},
			},
			fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("observation %d", i)}},
			},
		)
	}
	return messages
}

func TestSkillStateContextIsBounded(t *testing.T) {
	t.Parallel()

	short := boundContext(conversation(1), stateMessage(`{"goal":"g"}`))
	long := boundContext(conversation(100), stateMessage(`{"goal":"g"}`))

	require.Len(t, short, 5)
	require.Len(t, long, 5, "step context must not grow with the horizon")

	require.Equal(t, fantasy.MessageRoleSystem, long[0].Role)
	require.Equal(t, "system", messageText(long[0]))
	require.Equal(t, fantasy.MessageRoleUser, long[1].Role)
	require.Equal(t, "task", messageText(long[1]))
	require.Contains(t, messageText(long[2]), "EXECUTION STATE")
	require.Contains(t, messageText(long[2]), `{"goal":"g"}`)
	require.Equal(t, fantasy.MessageRoleAssistant, long[3].Role)
	require.Equal(t, "call 99", messageText(long[3]))
	require.Equal(t, fantasy.MessageRoleTool, long[4].Role)
	require.Equal(t, "observation 99", messageText(long[4]))
}

func TestSkillStateContextKeepsTaskOnFirstStep(t *testing.T) {
	t.Parallel()

	messages := []fantasy.Message{fantasy.NewUserMessage("task")}
	out := boundContext(messages, stateMessage(`{"goal":"g"}`))

	require.Len(t, out, 2)
	require.Equal(t, "task", messageText(out[0]))
	require.Contains(t, messageText(out[1]), "EXECUTION STATE")
}

// observations returns the assistant/tool pairs that fantasy appends to the
// conversation after n executed steps.
func observations(steps int) []fantasy.Message {
	messages := make([]fantasy.Message, 0, 2*steps)
	for i := range steps {
		messages = append(messages,
			fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("call %d", i)}},
			},
			fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: fmt.Sprintf("observation %d", i)}},
			},
		)
	}
	return messages
}

// countExecutionState reports how many messages carry the execution state.
func countExecutionState(messages []fantasy.Message) int {
	count := 0
	for _, message := range messages {
		if strings.Contains(messageText(message), "EXECUTION STATE") {
			count++
		}
	}
	return count
}

func TestSkillStateContextIsBoundedAcrossSteps(t *testing.T) {
	t.Parallel()

	// fantasy rebuilds the messages of a step from the system prompt, the
	// task and the responses accumulated so far, so the transformation is
	// applied to a growing slice on every step.
	task := []fantasy.Message{
		fantasy.NewSystemMessage("system"),
		fantasy.NewUserMessage("task"),
	}
	step := func(responses []fantasy.Message) []fantasy.Message {
		messages := append(append([]fantasy.Message{}, task...), responses...)
		return boundContext(messages, stateMessage(`{"goal":"g"}`))
	}

	second := step(observations(1))
	tenth := step(observations(9))

	require.Len(t, second, 5)
	require.Len(t, tenth, 5)
	require.Equal(t, "task", messageText(tenth[1]))
	require.Equal(t, 1, countExecutionState(tenth), "the state must not be repeated")
	require.Equal(t, "observation 8", messageText(tenth[len(tenth)-1]))
}

func TestSkillStateContextKeepsQueuedUserPrompt(t *testing.T) {
	t.Parallel()

	messages := conversation(2)
	messages = append(messages, fantasy.NewUserMessage("follow-up"))

	out := boundContext(messages, stateMessage(`{"goal":"g"}`))

	require.Len(t, out, 6)
	require.Equal(t, "follow-up", messageText(out[len(out)-1]))
}

func TestSkillStateContextKeepsLatestTaskWithoutSteps(t *testing.T) {
	t.Parallel()

	// Queued prompts are appended as user messages before the model runs,
	// so the newest one is the task of the step.
	messages := []fantasy.Message{
		fantasy.NewSystemMessage("system"),
		fantasy.NewUserMessage("first task"),
		fantasy.NewUserMessage("second task"),
	}
	out := boundContext(messages, stateMessage(`{"goal":"g"}`))

	require.Len(t, out, 3)
	require.Equal(t, "second task", messageText(out[1]))
}

func TestSkillStateContextKeepsParallelCalls(t *testing.T) {
	t.Parallel()

	// One step can issue several tool calls; every call and every result
	// stays in the tail, or providers reject the request.
	assistant := fantasy.Message{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{ToolCallID: "a", ToolName: "view", Input: "{}"},
			fantasy.ToolCallPart{ToolCallID: "b", ToolName: "grep", Input: "{}"},
		},
	}
	messages := []fantasy.Message{
		fantasy.NewUserMessage("task"),
		assistant,
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: "a", Output: fantasy.ToolResultOutputContentText{Text: "file"}},
		}},
		{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: "b", Output: fantasy.ToolResultOutputContentText{Text: "match"}},
		}},
	}
	out := boundContext(messages, stateMessage(`{"goal":"g"}`))

	require.Len(t, out, 5)
	require.Len(t, out[2].Content, 2, "both tool calls stay in the tail")
}

func TestSkillStateContextKeepsTaskParts(t *testing.T) {
	t.Parallel()

	// The task keeps its attachments: dropping them would strip the model of
	// the image the user was looking at.
	messages := []fantasy.Message{{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "look at this"},
			fantasy.FilePart{Filename: "shot.png", MediaType: "image/png"},
		},
	}}
	out := boundContext(messages, stateMessage(`{"goal":"g"}`))

	require.Len(t, out, 2)
	require.Len(t, out[0].Content, 2)
}

func TestSkillStateContextHandlesEmptyInput(t *testing.T) {
	t.Parallel()

	require.Empty(t, boundContext(nil, ""))
	require.Len(t, boundContext(nil, stateMessage(`{"goal":"g"}`)), 1)
}
