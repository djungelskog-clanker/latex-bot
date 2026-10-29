package render

import "strings"

func splitTopLevelLines(tex, opening, closing string) []string {
	var lines []string
	depth := 0
	start := 0
	inString := false
	for i := 0; i < len(tex); i++ {
		c := tex[i]
		switch {
		case c == '\\' && i+1 < len(tex) && tex[i+1] != '\n':
			i++
		case c == '"' && opening == "([{":
			inString = !inString
		case inString:
		case strings.IndexByte(opening, c) != -1:
			depth++
		case strings.IndexByte(closing, c) != -1:
			depth--
		case c == '\n' && depth == 0:
			lines = append(lines, tex[start:i])
			start = i + 1
		}
	}
	return append(lines, tex[start:])
}

func stripRowBreak(line string) (string, bool) {
	if !strings.HasSuffix(line, `\`) {
		return line, false
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, `\`), `\`)
	return strings.TrimSpace(line), true
}

func rows(tex, opening, closing string) []string {
	lines := splitTopLevelLines(tex, opening, closing)
	explicitBreaks := false
	for _, line := range lines {
		if _, breaks := stripRowBreak(strings.TrimSpace(line)); breaks {
			explicitBreaks = true
		}
	}
	var result []string
	var current []string
	flush := func() {
		if row := strings.Join(current, " "); row != "" {
			result = append(result, row)
		}
		current = nil
	}
	for _, line := range lines {
		line, breaks := stripRowBreak(strings.TrimSpace(line))
		if line != "" {
			current = append(current, line)
		}
		if breaks || !explicitBreaks {
			flush()
		}
	}
	flush()
	return result
}

func latexRows(tex string) string {
	if strings.Contains(tex, `\begin`) {
		return tex
	}
	lines := rows(tex, "{", "}")
	if len(lines) < 2 {
		return tex
	}
	environment := "gathered"
	if strings.Contains(tex, "&") {
		environment = "aligned"
	}
	return `\begin{` + environment + `}` + strings.Join(lines, ` \\ `) + `\end{` + environment + `}`
}

func typstRows(tex string) string {
	lines := rows(tex, "([{", ")]}")
	if len(lines) < 2 {
		return tex
	}
	return strings.Join(lines, ` \ `)
}
