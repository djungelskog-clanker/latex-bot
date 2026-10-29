package render

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"latex-bot/internal/mathextract"
)

const typstTemplate = `#import "@preview/typsium:0.3.2": *
#set page(width: auto, height: auto, margin: 8pt, fill: white)
#set text(size: 12pt)
#context {
  let wrap(body) = if measure(body).width > 14cm { block(width: 14cm, body) } else { body }
  let content = %s
  let size = measure(content)
  if size.width > 150cm or size.height > 150cm {
    panic("formula too large")
  }
  [#metadata((width: size.width / 1pt + 16, height: size.height / 1pt + 16)) <latexbot-size>]
  content
}
`

var typstStringEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", "")

func typstText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = `#"` + typstStringEscaper.Replace(line) + `"`
		}
	}
	return strings.Join(lines, "#linebreak()")
}

func typstMath(formula mathextract.Formula) string {
	if formula.Display {
		return "$ " + typstRows(formula.Tex) + " $"
	}
	return "$" + formula.Tex + "$"
}

func typstInline(parts []mathextract.Part) string {
	var body strings.Builder
	for _, part := range parts {
		if part.Formula != nil {
			body.WriteString(typstMath(*part.Formula))
		} else {
			body.WriteString(typstText(part.Text))
		}
	}
	return body.String()
}

func buildTypstDocument(parts []mathextract.Part) string {
	if isSingleFormula(parts) {
		return fmt.Sprintf(typstTemplate, "["+typstMath(*parts[0].Formula)+"]")
	}
	var rows []string
	for _, block := range layoutBlocks(parts) {
		if block.display != nil {
			rows = append(rows, "align(center)["+typstMath(*block.display)+"]")
		} else {
			rows = append(rows, "wrap["+typstInline(block.inline)+"]")
		}
	}
	return fmt.Sprintf(typstTemplate, "stack(dir: ttb, spacing: 10pt, "+strings.Join(rows, ", ")+")")
}

func renderTypst(ctx context.Context, dir string, parts []mathextract.Part) error {
	if err := writeSource(dir, "formula.typ", buildTypstDocument(parts)); err != nil {
		return err
	}
	output, err := run(ctx, dir, "typst", "eval", "--root", "/work", "--ignore-system-fonts", "--package-cache-path", typstPackageMount, "--in", "formula.typ", "query(<latexbot-size>).first().value")
	if err != nil {
		return err
	}
	var size struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := json.Unmarshal([]byte(output), &size); err != nil {
		return invalid("typst did not report a size")
	}
	dpi, err := scaledDPI(size.Width, size.Height)
	if err != nil {
		return err
	}
	_, err = run(ctx, dir, "typst", "compile", "--root", "/work", "--ignore-system-fonts", "--package-cache-path", typstPackageMount, "--ppi", dpi, "--format", "png", "formula.typ", outputFile)
	return err
}
