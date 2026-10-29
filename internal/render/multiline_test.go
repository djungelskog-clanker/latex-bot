package render

import "testing"

func TestLatexRows(t *testing.T) {
	cases := map[string]string{
		"x &= y^2 \\\nz &= 1":                    `\begin{aligned}x &= y^2 \\ z &= 1\end{aligned}`,
		"x &= y^2 \\\\\nz &= 1":                  `\begin{aligned}x &= y^2 \\ z &= 1\end{aligned}`,
		"a + b\n\nc":                             `\begin{gathered}a + b \\ c\end{gathered}`,
		"\\frac{a\n}{b}":                         "\\frac{a\n}{b}",
		"\\begin{pmatrix}1\\\\\n2\\end{pmatrix}": "\\begin{pmatrix}1\\\\\n2\\end{pmatrix}",
		"single":                                 "single",
		"a &= b \\\\\nc\n&= d":                   `\begin{aligned}a &= b \\ c &= d\end{aligned}`,
	}
	for input, want := range cases {
		if got := latexRows(input); got != want {
			t.Errorf("latexRows(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTypstRows(t *testing.T) {
	cases := map[string]string{
		"x &= y^2 \\\nz &= 1": `x &= y^2 \ z &= 1`,
		"a\nb":                `a \ b`,
		"mat(1, 2;\n3, 4)":    "mat(1, 2;\n3, 4)",
		"x \"a\n(\" + y\nz":   "x \"a\n(\" + y \\ z",
		"single":              "single",
	}
	for input, want := range cases {
		if got := typstRows(input); got != want {
			t.Errorf("typstRows(%q) = %q, want %q", input, got, want)
		}
	}
}
