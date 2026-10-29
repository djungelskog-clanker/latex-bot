package mathextract

import (
	"reflect"
	"strings"
	"testing"
)

func texts(text string) []string {
	result := []string{}
	for _, formula := range Extract(text) {
		result = append(result, formula.Tex)
	}
	return result
}

func TestExtract(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"inline", "so $x^2$ is cool", []string{"x^2"}},
		{"display across lines", "look $$\na+b\n$$ ok", []string{"a+b"}},
		{"several in order", "$a$ and $$b$$ and $c$", []string{"a", "b", "c"}},
		{"dollar amounts", "it costs $5 and $10 total", []string{}},
		{"dollar range", "between $5 and $10", []string{}},
		{"closing dollar before digit", "$20$30", []string{}},
		{"space inside delimiters", "$ x$ and $x $", []string{}},
		{"escaped dollars", `\$x$ and \$y\$`, []string{}},
		{"escaped dollar inside math", `$\$5 + x$`, []string{`\$5 + x`}},
		{"inline does not span lines", "$a\nb$", []string{}},
		{"code is ignored", "`$x$` and ```\n$y$\n``` but $z$", []string{"z"}},
		{"slack entities", "$a &lt; b &amp;&amp; c &gt; d$", []string{"a < b && c > d"}},
		{"empty display", "$$ $$", []string{}},
		{"unclosed display", "$$x", []string{}},
		{"unclosed inline", "$x", []string{}},
		{"overly long", "$" + strings.Repeat("x", MaxTexLength+1) + "$", []string{}},
		{"empty text", "", []string{}},
		{"block math", "look\n$\nx &= y^2 \\\\\nz &= 1\n$\nnice", []string{"x &= y^2 \\\\\nz &= 1"}},
		{"block math with indented closer", "$\n  a + b\n  $", []string{"a + b"}},
		{"block math needs closer on its own line", "$\na + b\nc$ d", []string{}},
		{"trailing dollar amount", "it is 5$\nand also 3$", []string{}},
		{"dollar amounts on separate lines", "costs 5$\n$10 more", []string{}},
		{"slack links are not math", "<https://x.com/a$b$c|link> and $y$", []string{"y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := texts(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Extract(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestExtractMarksDisplay(t *testing.T) {
	want := []Formula{{Tex: "a", Display: false}, {Tex: "b", Display: true}}
	if got := Extract("$a$ $$b$$"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestExtractCapsFormulas(t *testing.T) {
	if got := len(Extract(strings.Repeat("$a$ ", 50))); got != MaxFormulas {
		t.Errorf("got %d formulas, want %d", got, MaxFormulas)
	}
}

func TestExtractSyntaxPrefix(t *testing.T) {
	cases := []struct {
		text string
		want Syntax
	}{
		{"t$a/b$", SyntaxTypst},
		{"look t$$mat(1, 2)$$", SyntaxTypst},
		{"(t$x$)", SyntaxTypst},
		{"$x$", SyntaxAuto},
		{"at$x$", SyntaxAuto},
		{"st$x$", SyntaxAuto},
	}
	for _, tc := range cases {
		formulas := Extract(tc.text)
		if len(formulas) != 1 || formulas[0].Syntax != tc.want {
			t.Errorf("Extract(%q) = %+v, want syntax %v", tc.text, formulas, tc.want)
		}
	}
}

func TestExtractBlockMathIsDisplay(t *testing.T) {
	formulas := Extract("$\nx\n$")
	if len(formulas) != 1 || !formulas[0].Display {
		t.Errorf("got %+v, want one display formula", formulas)
	}
}

func TestParseKeepsContext(t *testing.T) {
	parts := Parse("so <@U123> said `$x$` is $a^2$ and t$b$, see <https://example.com|this> &amp; $$c$$")
	var got []string
	for _, part := range parts {
		if part.Formula != nil {
			got = append(got, "math:"+part.Formula.Tex)
		} else {
			got = append(got, "text:"+part.Text)
		}
	}
	want := []string{"text:so @someone said $x$ is ", "math:a^2", "text: and ", "math:b", "text:, see this & ", "math:c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestParseWithoutMathReturnsNothing(t *testing.T) {
	if parts := Parse("no math, just $5"); parts != nil {
		t.Errorf("expected nil, got %+v", parts)
	}
}
