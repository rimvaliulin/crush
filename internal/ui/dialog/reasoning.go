package dialog

import (
	"cmp"
	"errors"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ui/common"
)

const (
	// ReasoningID is the identifier for the reasoning effort dialog.
	ReasoningID = "reasoning"
)

// NewReasoning creates the reasoning effort dialog for the coder model.
func NewReasoning(com *common.Common) (*Select, error) {
	cfg := com.Config()
	agentCfg, ok := cfg.Agents[config.AgentCoder]
	if !ok {
		return nil, errors.New("agent configuration not found")
	}

	selectedModel := cfg.Models[agentCfg.Model]
	model := cfg.GetModelByType(agentCfg.Model)
	if model == nil {
		return nil, errors.New("model configuration not found")
	}

	levels := model.Reasoning.EffortLevels
	if len(levels) == 0 {
		return nil, errors.New("no reasoning levels available")
	}

	currentEffort := selectedModel.ReasoningEffort
	if currentEffort == "" {
		currentEffort = model.Reasoning.DefaultEffortLevel
	}

	items := make([]*SelectItem, 0, len(levels))
	for _, level := range levels {
		display := cmp.Or(level.Display, common.FormatReasoningEffort(level.Value))
		info := ""
		if level.Value == currentEffort {
			info = "current"
		}
		items = append(items, NewSelectItem(level.Value, display, info, com.Styles))
	}

	effortDialog := NewSelect(com, ReasoningID, "Select Reasoning Effort", items, func(effort string) Action {
		return ActionSelectReasoningEffort{Effort: effort}
	})
	effortDialog.SelectValue(currentEffort)
	return effortDialog, nil
}
