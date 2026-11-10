package dialog

import (
	"github.com/charmbracelet/crush/internal/harness"
	"github.com/charmbracelet/crush/internal/ui/common"
)

// HarnessID is the identifier for the harness picker dialog.
const HarnessID = "harness"

// NewHarness creates the dialog that picks the harness of the session about to
// be created. The rows come from the catalog, so a new harness adds no dialog.
func NewHarness(com *common.Common) *Select {
	items := make([]*SelectItem, 0, len(harness.Catalog))
	for _, info := range harness.Catalog {
		items = append(items, NewSelectItem(info.Name, info.Label, info.Name, com.Styles))
	}
	return NewSelect(com, HarnessID, "New Session", items, func(name string) Action {
		return ActionNewHarnessSession{Name: name}
	})
}
