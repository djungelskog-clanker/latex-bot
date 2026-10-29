package mathextract

import (
	"regexp"
	"strings"
)

const (
	MaxFormulas  = 10
	MaxTexLength = 8000
)

type Syntax int

const (
	SyntaxAuto Syntax = iota
	SyntaxLatex
	SyntaxTypst
)

type Formula struct {
	Tex     string
	Display bool
	Syntax  Syntax
}

type Part struct {
	Text    string
	Formula *Formula
}

var (
	slackEntities = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")
	slackLink     = regexp.MustCompile(`<([^<>|]*)(?:\|([^<>]*))?>`)
	codeBlock     = regexp.MustCompile("(?s)```.*?```")
	inlineCode    = regexp.MustCompile("`[^`\n]*`")
	nonNewline    = regexp.MustCompile("[^\n]")
	backticks     = regexp.MustCompile("`+")
)

func plainSlackReference(match string) string {
	groups := slackLink.FindStringSubmatch(match)
	target, label := groups[1], groups[2]
	switch {
	case strings.HasPrefix(target, "@"):
		return "@someone"
	case strings.HasPrefix(target, "#"):
		if label != "" {
			return "#" + label
		}
		return "#channel"
	case strings.HasPrefix(target, "!"):
		if label != "" {
			return label
		}
		return "@" + strings.TrimPrefix(strings.SplitN(target, "^", 2)[0], "!")
	case label != "":
		return label
	default:
		return target
	}
}

func blankOut(match string) string {
	return nonNewline.ReplaceAllString(match, " ")
}

func stripCode(text string) string {
	text = codeBlock.ReplaceAllStringFunc(text, blankOut)
	return inlineCode.ReplaceAllStringFunc(text, blankOut)
}

func isEscaped(text string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && text[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func isSpaceAt(text string, index int) bool {
	if index < 0 || index >= len(text) {
		return true
	}
	switch text[index] {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

func isAlphanumericAt(text string, index int) bool {
	if index < 0 || index >= len(text) {
		return false
	}
	c := text[index]
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func syntaxBefore(text string, dollar int) Syntax {
	if dollar > 0 && text[dollar-1] == 't' && !isAlphanumericAt(text, dollar-2) {
		return SyntaxTypst
	}
	return SyntaxAuto
}

func isDigitAt(text string, index int) bool {
	return index < len(text) && text[index] >= '0' && text[index] <= '9'
}

func restOfLineIsBlank(text string, from int) bool {
	for i := from; i < len(text) && text[i] != '\n'; i++ {
		if !isSpaceAt(text, i) {
			return false
		}
	}
	return true
}

func startsLine(text string, index int) bool {
	for i := index - 1; i >= 0; i-- {
		if text[i] == '\n' {
			return true
		}
		if !isSpaceAt(text, i) {
			return false
		}
	}
	return false
}

func findDisplayEnd(text string, from int) int {
	for i := from; i < len(text)-1; i++ {
		if text[i] == '$' && text[i+1] == '$' && !isEscaped(text, i) {
			return i
		}
	}
	return -1
}

func findBlockEnd(text string, from int) int {
	for i := from; i < len(text); i++ {
		if text[i] == '$' && !isEscaped(text, i) && startsLine(text, i) && restOfLineIsBlank(text, i+1) {
			return i
		}
	}
	return -1
}

func findInlineEnd(text string, from int) int {
	for i := from; i < len(text); i++ {
		switch {
		case text[i] == '\n':
			return -1
		case text[i] != '$' || isEscaped(text, i):
			continue
		case isSpaceAt(text, i-1) || isDigitAt(text, i+1):
			return -1
		default:
			return i
		}
	}
	return -1
}

type match struct {
	start, end int
	formula    Formula
}

func nextFormula(scan string, i int) (match, int, bool) {
	if i+1 < len(scan) && scan[i+1] == '$' {
		end := findDisplayEnd(scan, i+2)
		if end == -1 {
			return match{}, len(scan), false
		}
		return match{start: i, end: end + 2, formula: Formula{Tex: strings.TrimSpace(scan[i+2 : end]), Display: true}}, end + 2, true
	}
	if restOfLineIsBlank(scan, i+1) {
		end := findBlockEnd(scan, i+1)
		if end == -1 {
			return match{}, i + 1, false
		}
		return match{start: i, end: end + 1, formula: Formula{Tex: strings.TrimSpace(scan[i+1 : end]), Display: true}}, end + 1, true
	}
	if isSpaceAt(scan, i+1) {
		return match{}, i + 1, false
	}
	end := findInlineEnd(scan, i+1)
	if end == -1 {
		return match{}, i + 1, false
	}
	return match{start: i, end: end + 1, formula: Formula{Tex: scan[i+1 : end]}}, end + 1, true
}

func cleanText(text string) string {
	return backticks.ReplaceAllString(text, "")
}

func Parse(rawText string) []Part {
	text := slackEntities.Replace(slackLink.ReplaceAllStringFunc(rawText, plainSlackReference))
	scan := stripCode(text)
	var parts []Part
	textStart := 0
	formulas := 0
	for i := 0; i < len(scan) && formulas < MaxFormulas; {
		if scan[i] != '$' || isEscaped(scan, i) {
			i++
			continue
		}
		found, next, ok := nextFormula(scan, i)
		i = next
		if !ok || found.formula.Tex == "" || len(found.formula.Tex) > MaxTexLength {
			continue
		}
		start := found.start
		found.formula.Syntax = syntaxBefore(scan, start)
		if found.formula.Syntax == SyntaxTypst {
			start--
		}
		if prefix := cleanText(text[textStart:start]); prefix != "" {
			parts = append(parts, Part{Text: prefix})
		}
		formula := found.formula
		parts = append(parts, Part{Formula: &formula})
		textStart = found.end
		formulas++
	}
	if formulas == 0 {
		return nil
	}
	if suffix := cleanText(text[textStart:]); suffix != "" {
		parts = append(parts, Part{Text: suffix})
	}
	return parts
}

func Formulas(parts []Part) []Formula {
	var formulas []Formula
	for _, part := range parts {
		if part.Formula != nil {
			formulas = append(formulas, *part.Formula)
		}
	}
	return formulas
}

func Extract(rawText string) []Formula {
	return Formulas(Parse(rawText))
}
