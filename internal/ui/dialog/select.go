package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"
)

const (
	selectDialogMaxWidth  = 50
	selectDialogMinHeight = 8
	selectDialogMaxHeight = 16
)

// Select is a dialog that filters a fixed list and reports the chosen row. The
// harness picker and the reasoning effort picker are the same shape; they
// differ in the rows and in the action a choice produces.
type Select struct {
	com    *common.Common
	id     string
	title  string
	help   help.Model
	list   *list.FilterableList
	input  textinput.Model
	choose func(value string) Action

	keyMap struct {
		Select   key.Binding
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Close    key.Binding
	}
}

// SelectItem is one row of a [Select] dialog: a title, an optional note in the
// right column, and the value the dialog reports when the row is chosen.
type SelectItem struct {
	*list.Versioned
	value   string
	title   string
	info    string
	t       *styles.Styles
	m       fuzzy.Match
	cache   map[int]string
	focused bool
}

// NewSelectItem builds a row. The value is what [Select] reports through its
// choose callback; the title is what the user filters on.
func NewSelectItem(value, title, info string, t *styles.Styles) *SelectItem {
	return &SelectItem{
		Versioned: list.NewVersioned(),
		value:     value,
		title:     title,
		info:      info,
		t:         t,
	}
}

// Finished implements list.Item. Select rows are render-stable outside of
// explicit SetFocused / SetMatch.
func (s *SelectItem) Finished() bool {
	return true
}

var (
	_ Dialog   = (*Select)(nil)
	_ ListItem = (*SelectItem)(nil)
)

// NewSelect creates a dialog over items that reports the chosen value through
// choose.
func NewSelect(com *common.Common, id, title string, items []*SelectItem, choose func(value string) Action) *Select {
	s := &Select{com: com, id: id, title: title, choose: choose}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	s.help = help

	s.list = list.NewFilterableList()
	s.list.Focus()

	s.input = textinput.New()
	s.input.SetVirtualCursor(false)
	s.input.Placeholder = "Type to filter"
	s.input.SetStyles(com.Styles.TextInput)
	s.input.Focus()

	s.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "confirm"),
	)
	s.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next item"),
	)
	s.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	s.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	s.keyMap.Close = CloseKey

	listItems := make([]list.FilterableItem, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, item)
	}
	s.list.SetItems(listItems...)
	s.list.SetSelected(0)
	s.list.ScrollToSelected()

	return s
}

// ID implements Dialog.
func (s *Select) ID() string {
	return s.id
}

// SelectValue moves the highlight to the row with the given value, leaving it
// on the first row when no row matches.
func (s *Select) SelectValue(value string) {
	for i, item := range s.list.Items() {
		row, ok := item.(*SelectItem)
		if ok && row.value == value {
			s.list.SetSelected(i)
			s.list.ScrollToSelected()
			return
		}
	}
}

// HandleMsg implements [Dialog].
func (s *Select) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, s.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, s.keyMap.Previous):
			s.list.Focus()
			if s.list.IsSelectedFirst() {
				s.list.SelectLast()
				s.list.ScrollToBottom()
				break
			}
			s.list.SelectPrev()
			s.list.ScrollToSelected()
		case key.Matches(msg, s.keyMap.Next):
			s.list.Focus()
			if s.list.IsSelectedLast() {
				s.list.SelectFirst()
				s.list.ScrollToTop()
				break
			}
			s.list.SelectNext()
			s.list.ScrollToSelected()
		case key.Matches(msg, s.keyMap.Select):
			item, ok := s.list.SelectedItem().(*SelectItem)
			if !ok {
				break
			}
			return s.choose(item.value)
		default:
			prevValue := s.input.Value()
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			value := s.input.Value()
			if value != prevValue {
				s.list.SetFilter(value)
				s.list.ScrollToTop()
				s.list.SetSelected(0)
			}
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns the cursor position relative to the dialog.
func (s *Select) Cursor() *tea.Cursor {
	return InputCursor(s.com.Styles, s.input.Cursor())
}

// Draw implements [Dialog].
func (s *Select) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := s.com.Styles
	width := max(0, min(selectDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	s.input.SetWidth(dialogInputTextWidth(t, s.input, innerWidth))

	// Size the dialog to fit the list content, clamped to min/max bounds.
	listTotalHeight := s.list.TotalHeight()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()
	desiredHeight := heightOffset + listTotalHeight
	maxAvailable := area.Dy() - t.Dialog.View.GetVerticalBorderSize()
	height := max(selectDialogMinHeight, min(selectDialogMaxHeight, desiredHeight, maxAvailable))

	listHeight, listTotalHeight, _ := sizeDialogList(t, s.list, innerWidth, height)

	rc := NewRenderContext(t, width)
	rc.Title = s.title
	inputView := t.Dialog.InputPrompt.Render(s.input.View())
	rc.AddPart(inputView)

	visibleCount := len(s.list.FilteredItems())
	if s.list.Height() >= visibleCount {
		s.list.ScrollToTop()
	} else {
		s.list.ScrollToSelected()
	}

	listView := t.Dialog.List.Height(s.list.Height()).Render(s.list.Render())
	listView = joinScrollbar(t, listView, listHeight, listTotalHeight, listHeight, s.list.Offset())
	rc.AddPart(listView)
	rc.Help = renderDialogHelp(t, &s.help, s, innerWidth)

	view := rc.Render()

	cur := s.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (s *Select) ShortHelp() []key.Binding {
	return []key.Binding{
		s.keyMap.UpDown,
		s.keyMap.Select,
		s.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (s *Select) FullHelp() [][]key.Binding {
	m := [][]key.Binding{}
	slice := []key.Binding{
		s.keyMap.Select,
		s.keyMap.Next,
		s.keyMap.Previous,
		s.keyMap.Close,
	}
	for i := 0; i < len(slice); i += 4 {
		end := min(i+4, len(slice))
		m = append(m, slice[i:end])
	}
	return m
}

// Filter returns the filter value for the row.
func (s *SelectItem) Filter() string {
	return s.title
}

// ID returns the value the dialog reports for the row.
func (s *SelectItem) ID() string {
	return s.value
}

// SetFocused sets the focus state of the row.
func (s *SelectItem) SetFocused(focused bool) {
	if s.focused == focused {
		return
	}
	s.cache = nil
	s.focused = focused
	if s.Versioned != nil {
		s.Bump()
	}
}

// SetMatch sets the fuzzy match for the row.
func (s *SelectItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(s.m, m) {
		return
	}
	s.cache = nil
	s.m = m
	if s.Versioned != nil {
		s.Bump()
	}
}

// Render returns the string representation of the row.
func (s *SelectItem) Render(width int) string {
	styles := ListItemStyles{
		ItemBlurred:     s.t.Dialog.NormalItem,
		ItemFocused:     s.t.Dialog.SelectedItem,
		InfoTextBlurred: s.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: s.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(styles, s.title, s.info, s.focused, width, s.cache, &s.m)
}
