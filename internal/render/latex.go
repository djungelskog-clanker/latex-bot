package render

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"latex-bot/internal/mathextract"
)

var (
	forbiddenControlWord = regexp.MustCompile(`\\(input|include|includeonly|InputIfFileExists|IfFileExists|openin|openout|closein|closeout|read|readline|write|immediate|newwrite|newread|def|edef|gdef|xdef|let|futurelet|newcommand|renewcommand|providecommand|DeclareRobustCommand|DeclareMathOperator|newenvironment|renewenvironment|catcode|uccode|lccode|sfcode|mathcode|delcode|csname|endcsname|makeatletter|makeatother|usepackage|RequirePackage|documentclass|special|loop|repeat|jobname|everyjob|everypar|everymath|everydisplay|everyhbox|everyvbox|everycr|output|shipout|scantokens|errmessage|message|batchmode|nonstopmode|scrollmode|errorstopmode|dump|stop|bye|afterassignment|aftergroup|expandafter|globaldefs|endinput|endlinechar|newlinechar|escapechar|directlua|luaexec|pdfprimitive|primitive|tracingonline|showlists|showbox|showthe|show)([^a-zA-Z]|$)`)
	forbiddenDocumentEnv = regexp.MustCompile(`\\(begin|end)\s*\{\s*document\s*\}`)
	alignEnvironment     = regexp.MustCompile(`\\(begin|end)\s*\{(align|flalign|eqnarray)\*?\}`)
	alignatEnvironment   = regexp.MustCompile(`\\(begin|end)\s*\{alignat\*?\}`)
	gatherEnvironment    = regexp.MustCompile(`\\(begin|end)\s*\{(gather|multline)\*?\}`)
	latexSize            = regexp.MustCompile(`latexbot-size=([0-9.]+)pt;([0-9.]+)pt`)
	equationEnvironment  = regexp.MustCompile(`\\(begin|end)\s*\{equation\*?\}`)
)

func checkSafe(tex string) error {
	if match := forbiddenControlWord.FindStringSubmatch(tex); match != nil {
		return invalid(`\%s is not allowed`, match[1])
	}
	if forbiddenDocumentEnv.MatchString(tex) {
		return invalid("document environment is not allowed")
	}
	if strings.Contains(tex, "^^") {
		return invalid("^^ character codes are not allowed")
	}
	return nil
}

func toInlineEnvironments(tex string) string {
	tex = alignEnvironment.ReplaceAllString(tex, `\${1}{aligned}`)
	tex = alignatEnvironment.ReplaceAllString(tex, `\${1}{alignedat}`)
	tex = gatherEnvironment.ReplaceAllString(tex, `\${1}{gathered}`)
	return equationEnvironment.ReplaceAllString(tex, "")
}

const latexTemplate = `\documentclass[12pt]{article}
\usepackage{amsmath,amssymb,amsthm,mathtools,varwidth,tikz,siunitx,chemfig}
\usepackage[version=4]{mhchem}
\usepackage[active,tightpage]{preview}
\setlength\PreviewBorder{8pt}
\setlength\parindent{0pt}
\pagestyle{empty}
\newsavebox\formulabox
\begin{document}
\begin{lrbox}{\formulabox}%s
\end{lrbox}
\ifdim\wd\formulabox>150cm \errmessage{formula too wide}\fi
\ifdim\dimexpr\ht\formulabox+\dp\formulabox\relax>150cm \errmessage{formula too tall}\fi
\typeout{latexbot-size=\the\dimexpr\wd\formulabox+16pt\relax;\the\dimexpr\ht\formulabox+\dp\formulabox+16pt\relax}
\begin{preview}\usebox\formulabox\end{preview}
\end{document}
`

var latexTextEscaper = strings.NewReplacer(
	`\`, `\textbackslash{}`,
	`{`, `\{`,
	`}`, `\}`,
	`$`, `\$`,
	`&`, `\&`,
	`#`, `\#`,
	`%`, `\%`,
	`_`, `\_`,
	`^`, `\textasciicircum{}`,
	`~`, `\textasciitilde{}`,
	`<`, `$<$`,
	`>`, `$>$`,
	`|`, `$|$`,
	"\n", "\\par\n",
)

func isLatexTextRune(r rune) bool {
	switch {
	case r == '\n':
		return true
	case r < 0x20 || r == 0x7f:
		return false
	case r < 0x7f:
		return true
	case r >= 0xc0 && r <= 0xff:
		return r != 0xd7 && r != 0xf7
	}
	return strings.ContainsRune("‘’“”–—…", r)
}

func latexText(text string) string {
	var kept strings.Builder
	for _, r := range text {
		if isLatexTextRune(r) {
			kept.WriteRune(r)
		}
	}
	return latexTextEscaper.Replace(kept.String())
}

func latexMath(formula mathextract.Formula, block bool) string {
	tex := toInlineEnvironments(formula.Tex)
	switch {
	case formula.Display && block:
		return `\[` + latexRows(tex) + "\n" + `\]`
	case formula.Display:
		return `$\displaystyle ` + latexRows(tex) + "\n$"
	default:
		return "$" + tex + "\n$"
	}
}

func buildLatexDocument(parts []mathextract.Part) (string, error) {
	for _, formula := range mathextract.Formulas(parts) {
		if err := checkSafe(formula.Tex); err != nil {
			return "", err
		}
	}
	if isSingleFormula(parts) {
		return fmt.Sprintf(latexTemplate, latexMath(*parts[0].Formula, false)), nil
	}
	var rows []string
	for _, block := range layoutBlocks(parts) {
		if block.display != nil {
			rows = append(rows, `\multicolumn{1}{@{}c@{}}{$\displaystyle `+latexRows(toInlineEnvironments(block.display.Tex))+"\n$}")
			continue
		}
		var paragraph strings.Builder
		for _, part := range block.inline {
			if part.Formula != nil {
				paragraph.WriteString(latexMath(*part.Formula, false))
			} else {
				paragraph.WriteString(latexText(part.Text))
			}
		}
		rows = append(rows, `\begin{varwidth}{14cm}`+paragraph.String()+`\end{varwidth}`)
	}
	body := `\begin{tabular}{@{}l@{}}` + strings.Join(rows, `\\[6pt]`+"\n") + `\end{tabular}`
	return fmt.Sprintf(latexTemplate, body), nil
}

func renderLatex(ctx context.Context, dir string, parts []mathextract.Part) error {
	document, err := buildLatexDocument(parts)
	if err != nil {
		return err
	}
	if err := writeSource(dir, "formula.tex", document); err != nil {
		return err
	}
	output, err := run(ctx, dir, "pdflatex", "-no-shell-escape", "-interaction=nonstopmode", "-halt-on-error", "formula.tex")
	if err != nil {
		return err
	}
	size := latexSize.FindStringSubmatch(output)
	if size == nil {
		return invalid("latex did not report a size")
	}
	width, _ := strconv.ParseFloat(size[1], 64)
	height, _ := strconv.ParseFloat(size[2], 64)
	dpi, err := scaledDPI(width, height)
	if err != nil {
		return err
	}
	_, err = run(ctx, dir, "pdftoppm", "-r", dpi, "-png", "-singlefile", "formula.pdf", strings.TrimSuffix(outputFile, ".png"))
	return err
}
