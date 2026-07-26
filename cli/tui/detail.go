package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

type execResultMsg struct {
	result *cageclient.ExecResult
	err    error
}

type detailModel struct {
	client  *cageclient.Client
	sandbox *cageclient.Sandbox
	input   textinput.Model
	spinner spinner.Model
	running bool
	output  string
	errText string
}

func newDetailModel(client *cageclient.Client, sb *cageclient.Sandbox) detailModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. python3 --version"
	ti.Focus()
	ti.CharLimit = 200
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	ti.Prompt = "❯ "

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return detailModel{client: client, sandbox: sb, input: ti, spinner: s}
}

func (m detailModel) Init() tea.Cmd {
	return textinput.Blink
}

func runExec(client *cageclient.Client, sandboxID, rawCmd string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := client.Exec(ctx, sandboxID, strings.Fields(rawCmd))
		return execResultMsg{result: result, err: err}
	}
}

func (m detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" && !m.running && m.input.Value() != "" {
			m.running = true
			m.errText = ""
			cmd := m.input.Value()
			m.input.SetValue("")
			return m, tea.Batch(runExec(m.client, m.sandbox.ID, cmd), m.spinner.Tick)
		}

	case execResultMsg:
		m.running = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		exitStyle := successMsgStyle
		if msg.result.ExitCode != 0 {
			exitStyle = errorStyle
		}
		m.output = exitStyle.Render(fmt.Sprintf("exit code %d", msg.result.ExitCode)) + "\n" + msg.result.Stdout + msg.result.Stderr
		return m, nil

	case spinner.TickMsg:
		if m.running {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m detailModel) View() string {
	var b strings.Builder

	b.WriteString(appTitleStyle.Render("Sandbox " + m.sandbox.ID))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "%s  %s\n%s  %s\n%s  %s\n\n",
		dimStyle.Render("template"), m.sandbox.TemplateSlug,
		dimStyle.Render("status  "), styleStatus(string(m.sandbox.Status)),
		dimStyle.Render("expires "), m.sandbox.ExpiresAt.Format(time.Kitchen),
	)

	b.WriteString(dimStyle.Render("run a command:") + "\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")

	if m.running {
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("running..."))
	} else if m.errText != "" {
		b.WriteString(errorStyle.Render("✗ " + m.errText))
	} else if m.output != "" {
		b.WriteString(panelStyle.Render(m.output))
	}

	help := helpBarStyle.Render(keyHint("enter", "run") + keyHint("q", "back"))

	return panelAccentStyle.Render(b.String()) + "\n" + help
}
