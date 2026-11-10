package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/harness"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func newHarnessForTest() *Select {
	s := styles.CharmtonePantera()
	return NewHarness(&common.Common{Styles: &s})
}

func TestHarnessOffersEveryCatalogEntry(t *testing.T) {
	t.Parallel()

	d := newHarnessForTest()
	items := d.list.FilteredItems()
	require.Len(t, items, len(harness.Catalog))
	for i, info := range harness.Catalog {
		row, ok := items[i].(*SelectItem)
		require.True(t, ok)
		require.Equal(t, info.Name, row.value)
		require.Equal(t, info.Label, row.title)
	}
	require.Equal(t, harness.DefaultName, d.list.SelectedItem().(*SelectItem).value,
		"the default harness is preselected")
}

func TestHarnessSelectsTheHighlightedHarness(t *testing.T) {
	t.Parallel()

	d := newHarnessForTest()
	for i := 1; i < len(harness.Catalog); i++ {
		d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyDown})
		action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

		picked, ok := action.(ActionNewHarnessSession)
		require.True(t, ok)
		require.Equal(t, harness.Catalog[i].Name, picked.Name, "the highlight follows the catalog")
	}
}

func TestHarnessFiltersByName(t *testing.T) {
	t.Parallel()

	d := newHarnessForTest()
	for _, r := range "SKILL" {
		d.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	items := d.list.FilteredItems()
	require.Len(t, items, 1)
	require.Equal(t, harness.SkillState, items[0].(*SelectItem).value)
}

func TestHarnessClosesOnEscape(t *testing.T) {
	t.Parallel()

	d := newHarnessForTest()
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.IsType(t, ActionClose{}, action)
}
