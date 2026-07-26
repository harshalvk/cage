package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

type view int

const (
	viewSplash view = iota
	viewList
	viewDetail
)

// actionResultMsg carries the result of any create/pause/resume/delete
// action back into Update, so the UI can show a toast and refresh the list.
type actionResultMsg struct {
	message string
	err     error
}

type Model struct {
	client     *cageclient.Client
	list       list.Model
	spinner    spinner.Model
	view       view
	loading    bool
	busy       bool // true while an action (create/pause/delete) is in flight
	err        error
	toast      string
	confirm    bool // true while awaiting y/n confirmation for delete
	detail     detailModel
	width      int
	height     int
	splashCols int  // how many banner columns are currently revealed
	splashDone bool // true once the reveal animation has finished
}

// splashTickMsg drives the banner reveal animation, separate from tickMsg
// (which drives the list's periodic data refresh) so the two loops never
// interfere with each other.
type splashTickMsg struct{}

func splashTick() tea.Cmd {
	return tea.Tick(revealStepMs*time.Millisecond, func(time.Time) tea.Msg { return splashTickMsg{} })
}

func New(client *cageclient.Client) Model {
	l := list.New(nil, sandboxDelegate{}, 0, 0)
	l.Title = " Cage Sandboxes "
	l.Styles.Title = listTitleStyle
	l.SetShowHelp(false) // we render our own themed help bar instead

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return Model{
		client:  client,
		list:    l,
		spinner: s,
		view:    viewSplash,
		loading: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tickEvery(3*time.Second), splashTick())
}

func (m Model) selectedSandbox() (*cageclient.Sandbox, bool) {
	item, ok := m.list.SelectedItem().(sandboxItem)
	if !ok {
		return nil, false
	}
	return item.sb, true
}

func createSandbox(client *cageclient.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		sb, err := client.CreateSandbox(ctx, cageclient.CreateSandboxOptions{Template: "base"})
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{message: fmt.Sprintf("created sandbox %s", sb.ID)}
	}
}

func toggleSandbox(client *cageclient.Client, sb *cageclient.Sandbox) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if sb.Status == cageclient.StatusRunning {
			if _, err := client.PauseSandbox(ctx, sb.ID); err != nil {
				return actionResultMsg{err: err}
			}
			return actionResultMsg{message: fmt.Sprintf("paused %s", sb.ID)}
		}

		if _, err := client.ResumeSandbox(ctx, sb.ID); err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{message: fmt.Sprintf("resumed %s", sb.ID)}
	}
}

func deleteSandbox(client *cageclient.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.DeleteSandbox(ctx, id); err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{message: fmt.Sprintf("deleted %s", id)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-9)
		return m, nil

	case tea.KeyMsg:
		if m.view == viewSplash {
			if !m.splashDone {
				// First keypress mid-animation: skip straight to fully revealed,
				// rather than jumping out of the splash entirely.
				m.splashCols = bannerWidth
				m.splashDone = true
				return m, nil
			}
			m.view = viewList
			return m, fetchSandboxes(m.client)
		}

		// Delete confirmation takes over all key input until answered.
		if m.confirm {
			switch msg.String() {
			case "y":
				m.confirm = false
				sb, ok := m.selectedSandbox()
				if !ok {
					return m, nil
				}
				m.busy = true
				return m, deleteSandbox(m.client, sb.ID)
			default:
				m.confirm = false
				m.toast = "delete cancelled"
				return m, nil
			}
		}

		if m.view == viewDetail {
			if msg.String() == "q" {
				m.view = viewList
				return m, fetchSandboxes(m.client)
			}
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}

		// viewList key handling
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			if sb, ok := m.selectedSandbox(); ok {
				m.view = viewDetail
				m.detail = newDetailModel(m.client, sb)
				return m, m.detail.Init()
			}

		case "r":
			m.loading = true
			return m, fetchSandboxes(m.client)

		case "n":
			m.busy = true
			m.toast = ""
			return m, createSandbox(m.client)

		case "p":
			if sb, ok := m.selectedSandbox(); ok {
				m.busy = true
				return m, toggleSandbox(m.client, sb)
			}

		case "d":
			if _, ok := m.selectedSandbox(); ok {
				m.confirm = true
			}
			return m, nil
		}

	case actionResultMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
			m.toast = ""
		} else {
			m.err = nil
			m.toast = msg.message
		}
		return m, fetchSandboxes(m.client) // refresh list to reflect the action

	case refreshMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		items := make([]list.Item, len(msg.sandboxes))
		for i, sb := range msg.sandboxes {
			items[i] = sandboxItem{sb: sb}
		}
		m.list.SetItems(items)
		return m, nil

	case tickMsg:
		if m.view == viewList && !m.busy {
			return m, tea.Batch(fetchSandboxes(m.client), tickEvery(3*time.Second))
		}
		return m, tickEvery(3 * time.Second)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case splashTickMsg:
		if m.view != viewSplash || m.splashDone {
			return m, nil
		}
		m.splashCols += revealCols
		if m.splashCols >= bannerWidth {
			m.splashCols = bannerWidth
			m.splashDone = true
			return m, nil
		}
		return m, splashTick()
	}

	if m.view == viewDetail {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.view == viewSplash {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, renderSplash(m.splashCols, m.splashDone))
	}

	if m.view == viewDetail {
		return m.detail.View()
	}

	var body string
	if m.err != nil {
		body = errorStyle.Render("✗ "+m.err.Error()) + "\n" + dimStyle.Render("press 'r' to retry")
	} else if m.confirm {
		sb, _ := m.selectedSandbox()
		body = m.list.View() + "\n" + confirmStyle.Render(fmt.Sprintf(" delete %s? y/n ", sb.ID))
	} else {
		body = m.list.View()
		if m.busy {
			body += "\n" + m.spinner.View() + " " + dimStyle.Render("working...")
		} else if m.toast != "" {
			body += "\n" + successMsgStyle.Render("✓ "+m.toast)
		}
	}

	help := helpBarStyle.Render(
		keyHint("↑/↓", "navigate") +
			keyHint("enter", "open") +
			keyHint("n", "new") +
			keyHint("p", "pause/resume") +
			keyHint("d", "delete") +
			keyHint("r", "refresh") +
			keyHint("q", "quit"),
	)

	return panelStyle.Render(body) + "\n" + help
}
