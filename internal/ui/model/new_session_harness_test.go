package model

import (
	"testing"

	"github.com/charmbracelet/crush/internal/harness"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

func TestNewSessionAsksForTheHarness(t *testing.T) {
	t.Parallel()

	m := newBusyUI(&countingWorkspace{})
	runDialogAction(m, dialog.ActionNewSession{})

	require.True(t, m.dialog.ContainsDialog(dialog.HarnessID), "the harness picker opens")
	require.Equal(t, "s1", m.session.ID, "the session waits for the harness")
}

func TestHarnessChoiceStartsTheHarnessSession(t *testing.T) {
	t.Parallel()

	m := newBusyUI(&countingWorkspace{})
	m.dialog.OpenDialog(dialog.NewHarness(m.com))

	deliver(m, dialog.ActionNewHarnessSession{Name: harness.SkillState})

	require.Equal(t, harness.SkillState, m.pendingHarness, "the next session runs the picked harness")
	require.Nil(t, m.session, "the session is created with the first prompt")
	require.False(t, m.dialog.ContainsDialog(dialog.HarnessID), "the picker closes")
}

func TestHarnessPickerRefusesToOpenWhileTheAgentWorks(t *testing.T) {
	t.Parallel()

	ws := &countingWorkspace{}
	m := newBusyUI(ws)
	m.agentBusyCache.set(true)

	runDialogAction(m, dialog.ActionNewSession{})

	require.False(t, m.dialog.ContainsDialog(dialog.HarnessID))
}

// deliver hands m action the way an open dialog would, leaving the resulting
// command unrun: the handlers assert on state they already changed, and the
// commands only call into the workspace.
func deliver(m *UI, action dialog.Action) {
	d := &actionDialog{action: action}
	m.dialog.OpenDialog(d)
	defer m.dialog.CloseDialog(d.ID())
	m.handleDialogMsg(struct{}{})
}
