package render

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"latex-bot/internal/mathextract"
)

func newTestRenderer() *Renderer {
	return NewRenderer(2, 10*time.Second)
}

func render(t *testing.T, tex string, display bool) ([]byte, error) {
	t.Helper()
	return newTestRenderer().Render(context.Background(), mathextract.Formula{Tex: tex, Display: display, Syntax: mathextract.SyntaxLatex})
}

func renderTypstFormula(t *testing.T, tex string) ([]byte, error) {
	t.Helper()
	return newTestRenderer().Render(context.Background(), mathextract.Formula{Tex: tex, Syntax: mathextract.SyntaxTypst})
}

func assertPNG(t *testing.T, data []byte, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width == 0 || config.Height == 0 {
		t.Fatalf("bad png: %v %+v", err, config)
	}
}

func assertInvalid(t *testing.T, err error) {
	t.Helper()
	var invalidErr *InvalidFormulaError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("expected InvalidFormulaError, got %v", err)
	}
}

func TestRenderValidFormulas(t *testing.T) {
	formulas := []mathextract.Formula{
		{Tex: `e^{i\pi} + 1 = 0`},
		{Tex: `\int_0^\infty e^{-x^2}\,dx = \frac{\sqrt{\pi}}{2}`, Display: true},
		{Tex: `\begin{pmatrix}a&b\\c&d\end{pmatrix} \in \mathbb{R}^{2\times 2}`, Display: true},
		{Tex: `\begin{align*} a &= b \\ c &= d \end{align*}`, Display: true},
		{Tex: `f(x) = \begin{cases} 1 & x > 0 \\ 0 & \text{otherwise} \end{cases}`, Display: true},
		{Tex: `\left( \frac{a}{b} \right) \coloneqq x % trailing comment`},
	}
	for _, formula := range formulas {
		t.Run(formula.Tex, func(t *testing.T) {
			data, err := newTestRenderer().Render(context.Background(), formula)
			assertPNG(t, data, err)
		})
	}
}

func TestRenderRejectsBrokenTex(t *testing.T) {
	for _, tex := range []string{`\frac{1}{`, `\notamacro x`, `a}b`, `\end{lrbox}`} {
		t.Run(tex, func(t *testing.T) {
			_, err := render(t, tex, false)
			assertInvalid(t, err)
		})
	}
}

func TestRenderRejectsDangerousTex(t *testing.T) {
	for _, tex := range []string{
		`\input{/etc/passwd}`,
		`\include{x}`,
		`\immediate\write18{id}`,
		`\def\x{\x}\x`,
		`\catcode`,
		`\csname input\endcsname`,
		`^^5cinput`,
		`\end{document}`,
		`\newcommand{\x}{y}`,
		`\openin1=x`,
		`\usepackage{x}`,
	} {
		t.Run(tex, func(t *testing.T) {
			_, err := render(t, tex, false)
			assertInvalid(t, err)
		})
	}
}

func TestRenderAllowsLookalikeMacros(t *testing.T) {
	for _, tex := range []string{`\left(x\right)`, `\showcaseless`, `\leftarrow`} {
		if err := checkSafe(tex); err != nil {
			t.Errorf("checkSafe(%q) = %v", tex, err)
		}
	}
}

func TestRenderRejectsHugeOutput(t *testing.T) {
	_, err := render(t, `\rule{200cm}{1cm}`, false)
	assertInvalid(t, err)
	_, err = render(t, `\rule{1cm}{200cm}`, false)
	assertInvalid(t, err)
}

func TestRunKillsRunawayTex(t *testing.T) {
	dir := t.TempDir()
	document := `\documentclass{article}\begin{document}\def\x{\x}\x\end{document}`
	if err := os.WriteFile(filepath.Join(dir, "loop.tex"), []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := run(ctx, dir, "latex", "-interaction=nonstopmode", "-halt-on-error", "loop.tex")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("took %v to stop", elapsed)
	}
	assertInvalid(t, err)
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout, got %v", err)
	}
}

func TestStack(t *testing.T) {
	a, err := render(t, "a", false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := render(t, `\frac{x}{y} + \sum_{i=0}^n i`, true)
	if err != nil {
		t.Fatal(err)
	}
	stacked, err := Stack([][]byte{a, b})
	if err != nil {
		t.Fatal(err)
	}
	configA, _ := png.DecodeConfig(bytes.NewReader(a))
	configB, _ := png.DecodeConfig(bytes.NewReader(b))
	config, err := png.DecodeConfig(bytes.NewReader(stacked))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != max(configA.Width, configB.Width) || config.Height != configA.Height+configB.Height+stackGapPx {
		t.Errorf("unexpected stacked size %dx%d", config.Width, config.Height)
	}
	single, err := Stack([][]byte{a})
	if err != nil || !bytes.Equal(single, a) {
		t.Errorf("single image should pass through unchanged")
	}
	if _, err := Stack(nil); err == nil {
		t.Errorf("expected error for empty stack")
	}
}

func TestSandboxHidesHostFiles(t *testing.T) {
	dir := t.TempDir()
	document := `\documentclass{article}\begin{document}\input{/etc/hostname}\end{document}`
	if err := os.WriteFile(filepath.Join(dir, "leak.tex"), []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := run(context.Background(), dir, "latex", "-no-shell-escape", "-interaction=nonstopmode", "-halt-on-error", "leak.tex")
	assertInvalid(t, err)
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected file not found, got %v", err)
	}
}

func TestRenderTypst(t *testing.T) {
	for _, tex := range []string{`frac(a, b) + sqrt(x)`, `mat(1, 2; 3, 4)`, `sum_(i=0)^n i = (n(n+1))/2`, `x "if" x > 0`} {
		t.Run(tex, func(t *testing.T) {
			data, err := renderTypstFormula(t, tex)
			assertPNG(t, data, err)
		})
	}
}

func TestRenderTypstRejectsBrokenInput(t *testing.T) {
	for _, tex := range []string{`frac(a`, `undefinedthing(x)`, `#panic("x")`} {
		t.Run(tex, func(t *testing.T) {
			_, err := renderTypstFormula(t, tex)
			assertInvalid(t, err)
		})
	}
}

func TestRenderTypstCannotReadHostFiles(t *testing.T) {
	for _, tex := range []string{`#read("/etc/hostname")`, `#read("../../etc/hostname")`, `#import "@preview/cetz:0.3.0"`} {
		t.Run(tex, func(t *testing.T) {
			_, err := renderTypstFormula(t, tex)
			assertInvalid(t, err)
		})
	}
}

func TestRenderTypstRejectsHugeOutput(t *testing.T) {
	_, err := renderTypstFormula(t, `#box(width: 1cm, height: 200cm)`)
	assertInvalid(t, err)
}

func TestRenderTypstTimesOut(t *testing.T) {
	renderer := NewRenderer(1, 2*time.Second)
	start := time.Now()
	_, err := renderer.Render(context.Background(), mathextract.Formula{Tex: `#{ while true {} }`, Syntax: mathextract.SyntaxTypst})
	if time.Since(start) > 5*time.Second {
		t.Fatalf("render took too long: %v", time.Since(start))
	}
	assertInvalid(t, err)
}

func TestRenderAutoDetectsSyntax(t *testing.T) {
	for _, tex := range []string{`\frac{a}{b}`, `frac(a, b)`, `x^2 + y`, `alpha + beta`, `ab + c`} {
		t.Run(tex, func(t *testing.T) {
			data, err := newTestRenderer().Render(context.Background(), mathextract.Formula{Tex: tex})
			assertPNG(t, data, err)
		})
	}
}

func TestRenderAutoDoesNotFallBackFromCertainLatex(t *testing.T) {
	_, err := newTestRenderer().Render(context.Background(), mathextract.Formula{Tex: `\frac{1}{`})
	assertInvalid(t, err)
}

func TestRenderBlockMathRows(t *testing.T) {
	for _, text := range []string{
		"$\nx &= y^2 + z^3 \\\nx_2 &= y^3.1 + b\n$",
		"$\nx &= y^2 + z^3 \\\\\nx_2 &= y^3.1 + b\n$",
		"$\na + b\nc + d\n$",
		"t$\nx &= y^2 \\\nx_2 &= y^3\n$",
	} {
		t.Run(text, func(t *testing.T) {
			formulas := mathextract.Extract(text)
			if len(formulas) != 1 {
				t.Fatalf("expected one formula, got %+v", formulas)
			}
			data, err := newTestRenderer().Render(context.Background(), formulas[0])
			assertPNG(t, data, err)
		})
	}
}

func TestRenderMessageWithContext(t *testing.T) {
	for _, text := range []string{
		`if $a = \frac{1}{2}$ and $b^2 = 4$ then $$a \cdot b = 1$$ obviously {50% & #1_ ~^\ <>|} äöüß é “ok” 🐻`,
		"t$a/b$ and t$sqrt(x)$ with \"quotes\" and \\ backslash # and ] [ $ ok",
		"so $alpha$ and $beta$\non a new line",
		"lines:\n$\nx &= 1 \\\ny &= 2\n$\nand $z$",
	} {
		t.Run(text, func(t *testing.T) {
			data, err := newTestRenderer().RenderMessage(context.Background(), mathextract.Parse(text))
			assertPNG(t, data, err)
		})
	}
}

func TestRenderMessageRejectsMixedSyntaxes(t *testing.T) {
	_, err := newTestRenderer().RenderMessage(context.Background(), mathextract.Parse(`$\frac{a}{b}$ and t$a/b$`))
	assertInvalid(t, err)
}

func TestRenderMessageStillBlocksDangerousLatex(t *testing.T) {
	_, err := newTestRenderer().RenderMessage(context.Background(), mathextract.Parse(`$\input{/etc/passwd}$ and $\frac{a}{b}$`))
	assertInvalid(t, err)
}

func TestLatexTextCannotInjectCommands(t *testing.T) {
	document, err := buildLatexDocument(mathextract.Parse(`\input{/etc/passwd} $\frac{a}{b}$ and $x$ \end{varwidth}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(document, `\input{`) || strings.Count(document, `\end{varwidth}`) != 1 {
		t.Errorf("text was not escaped:\n%s", document)
	}
}

func TestRenderScalesDownWideFormulas(t *testing.T) {
	for _, formula := range []mathextract.Formula{
		{Tex: `\rule{80cm}{1cm}`, Syntax: mathextract.SyntaxLatex},
		{Tex: `#box(width: 1cm, height: 80cm)`, Syntax: mathextract.SyntaxTypst},
	} {
		data, err := newTestRenderer().Render(context.Background(), formula)
		assertPNG(t, data, err)
		config, _ := png.DecodeConfig(bytes.NewReader(data))
		if max(config.Width, config.Height) > targetSidePx+10 {
			t.Errorf("%s: image %dx%d not scaled down", formula.Tex, config.Width, config.Height)
		}
	}
}

func TestScaledDPI(t *testing.T) {
	cases := []struct {
		width, height float64
		want          string
		wantErr       bool
	}{
		{100, 50, "300", false},
		{72.27 * 20, 10, "200", false},
		{72.27 * 200, 10, "", true},
	}
	for _, tc := range cases {
		got, err := scaledDPI(tc.width, tc.height)
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("scaledDPI(%v, %v) = %q, %v", tc.width, tc.height, got, err)
		}
	}
}

func TestRenderMessageKeepsWideDisplayMath(t *testing.T) {
	for _, text := range []string{`$a$ and $$\rule{40cm}{1pt}$$`, `t$a$ and t$$#box(width: 40cm, height: 1pt)$$`} {
		data, err := newTestRenderer().RenderMessage(context.Background(), mathextract.Parse(text))
		assertPNG(t, data, err)
		config, _ := png.DecodeConfig(bytes.NewReader(data))
		if config.Width < 3000 {
			t.Errorf("%s: wide display math was clipped to %dpx", text, config.Width)
		}
	}
}

func TestCandidateSyntaxesPreferTypstGuesses(t *testing.T) {
	formulas := mathextract.Extract("This is a vector: $arrow(a)$ and this is another one: $arrow(b)$")
	got := candidateSyntaxes(formulas)
	if len(got) == 0 || got[0] != mathextract.SyntaxTypst {
		t.Errorf("candidateSyntaxes = %v, want typst first", got)
	}
	if got := candidateSyntaxes(mathextract.Extract(`$x$ and $\frac{a}{b}$`)); len(got) != 1 || got[0] != mathextract.SyntaxLatex {
		t.Errorf("candidateSyntaxes = %v, want only latex", got)
	}
}

func TestRenderScienceExtras(t *testing.T) {
	for _, formula := range []mathextract.Formula{
		{Tex: `\ce{2H2 + O2 -> 2H2O}`},
		{Tex: `\qty{9.81}{\metre\per\second\squared}`},
		{Tex: `\chemfig{*6(=-=-=-)}`, Display: true},
		{Tex: `\tikz \draw[->] (0,0) -- (1,0.5);`},
		{Tex: `\coloneqq \mathbb{R}`},
		{Tex: `ce("2H2 + O2 -> 2H2O")`},
		{Tex: `ce("H2O")`, Syntax: mathextract.SyntaxTypst},
	} {
		t.Run(formula.Tex, func(t *testing.T) {
			data, err := newTestRenderer().Render(context.Background(), formula)
			assertPNG(t, data, err)
		})
	}
}
