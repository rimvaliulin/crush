package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/harness/skillstate"
)

// SkillStateToolName is the tool that patches the explicit execution state.
const SkillStateToolName = "skillstate"

//go:embed skillstate.md
var skillStateDescription string

// SkillStateParams is the input of the state tool.
type SkillStateParams struct {
	Patch map[string]any `json:"patch" description:"JSON object merged into the execution state; set a key to null to delete it."`
}

// NewSkillStateTool returns a tool that merges a validated patch into the
// execution state of the calling session.
func NewSkillStateTool(store *skillstate.Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SkillStateToolName,
		skillStateDescription,
		func(ctx context.Context, params SkillStateParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if len(params.Patch) == 0 {
				return fantasy.NewTextErrorResponse("patch is required"), nil
			}
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("REJECTED: no session in context"), nil
			}
			if store == nil {
				return fantasy.NewTextErrorResponse("REJECTED: this session does not run in SKILL.state mode"), nil
			}
			size, err := store.Patch(ctx, sessionID, params.Patch)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("REJECTED: %v", err)), nil
			}
			state, err := store.Get(ctx, sessionID)
			if err != nil {
				return fantasy.NewTextResponse(fmt.Sprintf("ok (%d bytes)", size)), nil
			}
			return fantasy.NewTextResponse(
				fmt.Sprintf("ok (%d bytes)\n%s", size, skillstate.Compact(state)),
			), nil
		},
	)
}
