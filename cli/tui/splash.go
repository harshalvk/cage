package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	cageVersion  = "v0.1.0"
	bannerScale  = 2 // 2x the base 5x7 font — a genuinely large hero title
	revealStepMs = 16
	revealCols   = 3 // columns revealed per animation tick
)

var bannerRows = renderBannerScaled("CAGE", bannerScale)
var bannerWidth = func() int {
	max := 0
	for _, r := range bannerRows {
		if l := len([]rune(r)); l > max {
			max = l
		}
	}
	return max
}()

var bannerStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

// renderBannerFrame returns the banner with only the first `cols` columns
// of each row visible — everything beyond that is padded with spaces so
// the overall block keeps a stable width/position as it reveals, rather
// than the layout jumping around frame to frame.
func renderBannerFrame(cols int) string {
	if cols > bannerWidth {
		cols = bannerWidth
	}
	var b strings.Builder
	for _, row := range bannerRows {
		runes := []rune(row)
		visible := string(runes[:min(cols, len(runes))])
		padding := strings.Repeat(" ", bannerWidth-len([]rune(visible)))
		b.WriteString(bannerStyle.Render(visible))
		b.WriteString(padding)
		b.WriteString("\n")
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderSplashBelow renders the version line and keybind cheatsheet that
// appear once the banner has finished revealing. Kept separate from the
// banner itself, which stays unboxed and large; this part gets a light
// frame so it doesn't read as a flat wall of text underneath the hero title.
func renderSplashBelow() string {
	var b strings.Builder

	b.WriteString(dimStyle.Render(cageVersion + " · self-hosted sandbox infrastructure"))
	b.WriteString("\n\n")

	keys := []struct{ key, desc string }{
		{"↑/↓", "navigate"},
		{"enter", "open sandbox"},
		{"n", "new sandbox"},
		{"p", "pause / resume"},
		{"d", "delete"},
		{"r", "refresh"},
		{"q", "quit"},
	}
	for _, k := range keys {
		b.WriteString(keyHint(k.key, k.desc))
		b.WriteString("\n")
	}

	panel := panelStyle.Render(strings.TrimRight(b.String(), "\n"))
	return panel + "\n\n" + dimStyle.Render("press any key to continue")
}

// renderSplash composes the full splash screen for a given animation frame.
// cols is how many banner columns are currently revealed; done indicates
// whether the reveal has finished (controls whether the panel below shows).
func renderSplash(cols int, done bool) string {
	var b strings.Builder
	b.WriteString(renderBannerFrame(cols))
	b.WriteString("\n")

	if done {
		b.WriteString(renderSplashBelow())
	}

	return lipgloss.NewStyle().Align(lipgloss.Center).Render(b.String())
}
