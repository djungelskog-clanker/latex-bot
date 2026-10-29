package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"latex-bot/internal/mathextract"
)

const (
	maxDPI          = 300
	minDPI          = 40
	targetSidePx    = 4000
	maxImageSidePx  = 6000
	stackGapPx      = 12
	addressSpaceCap = "--as=1073741824"
	cpuSecondsCap   = "--cpu=10"
	fileSizeCap     = "--fsize=33554432"
)

type InvalidFormulaError struct {
	Reason string
}

func (e *InvalidFormulaError) Error() string {
	return "invalid formula: " + e.Reason
}

func invalid(format string, args ...any) error {
	return &InvalidFormulaError{Reason: fmt.Sprintf(format, args...)}
}

const outputFile = "formula.png"

type Renderer struct {
	slots   chan struct{}
	timeout time.Duration
}

func NewRenderer(concurrency int, timeout time.Duration) *Renderer {
	return &Renderer{slots: make(chan struct{}, concurrency), timeout: timeout}
}

func (r *Renderer) Render(ctx context.Context, formula mathextract.Formula) ([]byte, error) {
	return r.renderWithCandidates(ctx, []mathextract.Part{{Formula: &formula}}, candidateSyntaxes([]mathextract.Formula{formula}))
}

func (r *Renderer) RenderMessage(ctx context.Context, parts []mathextract.Part) ([]byte, error) {
	return r.renderWithCandidates(ctx, parts, candidateSyntaxes(mathextract.Formulas(parts)))
}

func candidateSyntaxes(formulas []mathextract.Formula) []mathextract.Syntax {
	required := map[mathextract.Syntax]bool{}
	preferred := mathextract.SyntaxLatex
	for _, formula := range formulas {
		if formula.Syntax != mathextract.SyntaxAuto {
			required[formula.Syntax] = true
			continue
		}
		guess, certain := mathextract.GuessSyntax(formula.Tex)
		switch {
		case certain:
			required[guess] = true
		case guess == mathextract.SyntaxTypst:
			preferred = mathextract.SyntaxTypst
		}
	}
	switch len(required) {
	case 0:
		return []mathextract.Syntax{preferred, otherSyntax(preferred)}
	case 1:
		for syntax := range required {
			return []mathextract.Syntax{syntax}
		}
	}
	return nil
}

func (r *Renderer) renderWithCandidates(ctx context.Context, parts []mathextract.Part, syntaxes []mathextract.Syntax) ([]byte, error) {
	if len(syntaxes) == 0 {
		return nil, invalid("formulas mix latex and typst")
	}
	var firstErr error
	for _, syntax := range syntaxes {
		data, err := r.renderAs(ctx, parts, syntax)
		var invalidErr *InvalidFormulaError
		if err == nil || !errors.As(err, &invalidErr) {
			return data, err
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

func otherSyntax(syntax mathextract.Syntax) mathextract.Syntax {
	if syntax == mathextract.SyntaxTypst {
		return mathextract.SyntaxLatex
	}
	return mathextract.SyntaxTypst
}

func (r *Renderer) renderAs(ctx context.Context, parts []mathextract.Part, syntax mathextract.Syntax) ([]byte, error) {
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	dir, err := os.MkdirTemp("", "latex-bot-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	compile := renderLatex
	if syntax == mathextract.SyntaxTypst {
		compile = renderTypst
	}
	if err := compile(ctx, dir, parts); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, outputFile))
	if err != nil {
		return nil, invalid("no image produced")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, invalid("unreadable image: %v", err)
	}
	if config.Width > maxImageSidePx || config.Height > maxImageSidePx {
		return nil, invalid("image too large")
	}
	return data, nil
}

func scaledDPI(widthPt, heightPt float64) (string, error) {
	const pointsPerInch = 72.27
	longestInches := max(widthPt, heightPt) / pointsPerInch
	dpi := maxDPI
	if longestInches > 0 {
		dpi = min(maxDPI, int(math.Floor(targetSidePx/longestInches+1e-6)))
	}
	if dpi < minDPI {
		return "", invalid("formula too large")
	}
	return strconv.Itoa(dpi), nil
}

func writeSource(dir, name, source string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600)
}

func Stack(images [][]byte) ([]byte, error) {
	decoded := make([]image.Image, 0, len(images))
	width, height := 0, 0
	for _, data := range images {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, img)
		width = max(width, img.Bounds().Dx())
		height += img.Bounds().Dy()
	}
	if len(decoded) == 0 {
		return nil, errors.New("nothing to stack")
	}
	if len(decoded) == 1 {
		return images[0], nil
	}
	height += stackGapPx * (len(decoded) - 1)

	canvas := image.NewGray(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	y := 0
	for _, img := range decoded {
		bounds := img.Bounds()
		draw.Draw(canvas, image.Rect(0, y, bounds.Dx(), y+bounds.Dy()), img, bounds.Min, draw.Src)
		y += bounds.Dy() + stackGapPx
	}

	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
