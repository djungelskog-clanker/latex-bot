package mathextract

import "regexp"

var (
	latexCommand    = regexp.MustCompile(`\\[a-zA-Z]`)
	typstWord       = regexp.MustCompile(`[a-zA-Z]{2,}`)
	typstStringOrFn = regexp.MustCompile(`"|#[a-zA-Z]|=>|->|<==>|(?m)(^|[^\\])\\[ \t]*$`)
)

func GuessSyntax(tex string) (syntax Syntax, certain bool) {
	switch {
	case latexCommand.MatchString(tex):
		return SyntaxLatex, true
	case typstWord.MatchString(tex), typstStringOrFn.MatchString(tex):
		return SyntaxTypst, false
	default:
		return SyntaxLatex, false
	}
}
