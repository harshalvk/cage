package tui

import "strings"

// font5x7 maps each letter to its 7-row, 5-column pixel pattern.
// '#' = filled pixel, '.' = empty.
var font5x7 = map[rune][]string{
	'C': {
		".###.",
		"#...#",
		"#....",
		"#....",
		"#....",
		"#...#",
		".###.",
	},
	'A': {
		".###.",
		"#...#",
		"#...#",
		"#####",
		"#...#",
		"#...#",
		"#...#",
	},
	'G': {
		".###.",
		"#...#",
		"#....",
		"#..##",
		"#...#",
		"#...#",
		".###.",
	},
	'E': {
		"#####",
		"#....",
		"#....",
		"###..",
		"#....",
		"#....",
		"#####",
	},
}

// renderBannerScaled draws `word` as a big block-letter banner, scaling each
// pixel of the base 5x7 font up by `scale` (both horizontally and
// vertically) so the result reads as a genuinely large hero title rather
// than a small terminal glyph. Returns one string per output row.
func renderBannerScaled(word string, scale int) []string {
	rowCount := 7 * scale
	rows := make([]string, rowCount)

	for _, ch := range word {
		glyph, ok := font5x7[ch]
		if !ok {
			continue
		}
		for srcRow, line := range glyph {
			// Build this glyph row scaled horizontally first...
			var scaledLine strings.Builder
			for _, px := range line {
				block := " "
				if px == '#' {
					block = "█"
				}
				scaledLine.WriteString(strings.Repeat(block, scale))
			}
			// ...then repeat it `scale` times vertically.
			for v := 0; v < scale; v++ {
				rows[srcRow*scale+v] += scaledLine.String() + strings.Repeat(" ", scale)
			}
		}
	}
	return rows
}
