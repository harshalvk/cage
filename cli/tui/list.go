package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

type sandboxItem struct {
	sb *cageclient.Sandbox
}

func (i sandboxItem) Title() string { return i.sb.ID }
func (i sandboxItem) Description() string {
	return fmt.Sprintf("%s · %s · expires %s", i.sb.TemplateSlug, styleStatus(string(i.sb.Status)), i.sb.ExpiresAt.Format(time.Kitchen))
}
func (i sandboxItem) FilterValue() string { return i.sb.ID + " " + i.sb.TemplateSlug }

// refreshMsg carries fresh sandbox data back from a background fetch.
type refreshMsg struct {
	sandboxes []*cageclient.Sandbox
	err       error
}

func fetchSandboxes(client *cageclient.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sandboxes, err := client.ListSandboxes(ctx)
		return refreshMsg{sandboxes: sandboxes, err: err}
	}
}

// tickMsg drives the auto-refresh loop.
type tickMsg time.Time

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}
