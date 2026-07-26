package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// sandboxDelegate renders each sandboxItem in the list, styling the
// selected row with the accent color instead of Bubbles' default.
type sandboxDelegate struct{}

func (d sandboxDelegate) Height() int                               { return 2 }
func (d sandboxDelegate) Spacing() int                              { return 1 }
func (d sandboxDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d sandboxDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(sandboxItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	title := item.Title()
	desc := item.Description()

	if isSelected {
		fmt.Fprintln(w, itemSelectedStyle.Render("▸ "+title))
		fmt.Fprintln(w, itemDescSelectedStyle.Render("  "+desc))
	} else {
		fmt.Fprintln(w, itemNormalStyle.Render(title))
		fmt.Fprintln(w, itemDescStyle.Render(strings.Repeat(" ", 2)+desc))
	}
}
