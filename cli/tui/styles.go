package tui

import "github.com/charmbracelet/lipgloss"

// Palette — a warm, muted terracotta accent (matching Claude's brand color)
// against a dark neutral background, rather than a harsh neon orange.
const (
	colorAccent    = lipgloss.Color("#E8B04B") // primary accent — amber/gold
	colorAccentDim = lipgloss.Color("#96712F") // muted version, for less emphasis
	colorBg        = lipgloss.Color("#1B1917") // near-black warm-neutral background
	colorSurface   = lipgloss.Color("#252220") // slightly lifted panel background
	colorBorder    = lipgloss.Color("#3A3733")
	colorText      = lipgloss.Color("#EAE6E0") // warm off-white
	colorTextMuted = lipgloss.Color("#8E8981")
	colorTextFaint = lipgloss.Color("#5A5650")
	colorSuccess   = lipgloss.Color("#7FB88A") // muted green — running
	colorWarning   = lipgloss.Color("#D97F5E") // muted coral — paused (distinct from gold accent)
	colorDanger    = lipgloss.Color("#C7654F") // muted red — errors/delete confirm
)

var (
	// Header / branding
	appTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1)

	headerBarStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted).
			Background(colorSurface).
			Padding(0, 1).
			Width(100)

	// Status pills
	statusRunning = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	statusPaused  = lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
	statusOther   = lipgloss.NewStyle().Foreground(colorTextFaint)

	// List
	listTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBg).
			Background(colorAccent).
			Padding(0, 1)

	itemSelectedStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorAccent).
				BorderLeft(true).
				Padding(0, 0, 0, 1)

	itemNormalStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(0, 0, 0, 2)

	itemDescStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted)

	itemDescSelectedStyle = lipgloss.NewStyle().
				Foreground(colorAccentDim)

	// Panels / boxes
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Background(colorSurface).
			Foreground(colorText).
			Padding(1, 2)

	panelAccentStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Background(colorSurface).
				Foreground(colorText).
				Padding(1, 2)

	// Help / key bar
	keyStyle = lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorAccent).
			Bold(true).
			Padding(0, 1)

	keyDescStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted).
			PaddingRight(2)

	helpBarStyle = lipgloss.NewStyle().
			Foreground(colorTextMuted).
			Padding(1, 1, 0, 1)

	// Feedback
	errorStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	successMsgStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	confirmStyle = lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorDanger).
			Bold(true).
			Padding(0, 1)

	dimStyle = lipgloss.NewStyle().Foreground(colorTextFaint)
)

func styleStatus(status string) string {
	switch status {
	case "running":
		return statusRunning.Render("● running")
	case "paused":
		return statusPaused.Render("◐ paused")
	default:
		return statusOther.Render("○ " + status)
	}
}

// keyHint renders a single "[key] description" pill for the help bar.
func keyHint(key, desc string) string {
	return keyStyle.Render(key) + " " + keyDescStyle.Render(desc)
}
