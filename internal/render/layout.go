package render

import (
	"strings"

	"latex-bot/internal/mathextract"
)

type block struct {
	display *mathextract.Formula
	inline  []mathextract.Part
}

func layoutBlocks(parts []mathextract.Part) []block {
	var blocks []block
	var inline []mathextract.Part
	flush := func() {
		if hasVisibleContent(inline) {
			blocks = append(blocks, block{inline: inline})
		}
		inline = nil
	}
	for _, part := range parts {
		if part.Formula != nil && part.Formula.Display {
			flush()
			blocks = append(blocks, block{display: part.Formula})
			continue
		}
		if part.Formula == nil {
			part.Text = strings.Trim(part.Text, "\n")
		}
		inline = append(inline, part)
	}
	flush()
	return blocks
}

func hasVisibleContent(parts []mathextract.Part) bool {
	for _, part := range parts {
		if part.Formula != nil || strings.TrimSpace(part.Text) != "" {
			return true
		}
	}
	return false
}

func isSingleFormula(parts []mathextract.Part) bool {
	return len(parts) == 1 && parts[0].Formula != nil
}
