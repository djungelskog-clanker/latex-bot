package mathextract

import "testing"

func TestGuessSyntax(t *testing.T) {
	cases := []struct {
		tex         string
		want        Syntax
		wantCertain bool
	}{
		{`\frac{a}{b}`, SyntaxLatex, true},
		{`\alpha + \beta`, SyntaxLatex, true},
		{`x^{2} + \sqrt{y}`, SyntaxLatex, true},
		{`\text{if } x > 0`, SyntaxLatex, true},
		{`frac(a, b)`, SyntaxTypst, false},
		{`alpha + beta`, SyntaxTypst, false},
		{`sum_(i=0)^n i`, SyntaxTypst, false},
		{`mat(1, 2; 3, 4)`, SyntaxTypst, false},
		{`x "if" x > 0`, SyntaxTypst, false},
		{`x^2 + y`, SyntaxLatex, false},
		{`a/b = 2`, SyntaxLatex, false},
		{`arrow(a)`, SyntaxTypst, false},
		{"a = b \\\nc = d", SyntaxTypst, false},
		{"x => y", SyntaxTypst, false},
		{"a^2 \\\\\nb^2", SyntaxLatex, false},
	}
	for _, tc := range cases {
		got, certain := GuessSyntax(tc.tex)
		if got != tc.want || certain != tc.wantCertain {
			t.Errorf("GuessSyntax(%q) = %v, %v, want %v, %v", tc.tex, got, certain, tc.want, tc.wantCertain)
		}
	}
}
