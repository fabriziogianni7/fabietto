package gateway

import (
	"strings"
	"testing"
)

func TestFormatForTelegramReply_TableAndBold(t *testing.T) {
	in := `| City | Why May |
|------|----------|
| **Rome** | ≈ 22 °C |
| Lisbon | mild |`

	out := FormatForTelegramReply(in)
	if strings.Contains(out, "<pre>") {
		t.Fatalf("pipe tables should use normal HTML, not pre:\n%s", out)
	}
	if !strings.Contains(out, "\n") || !strings.Contains(out, "Rome") {
		t.Fatalf("expected br and content, got:\n%s", out)
	}
	if strings.Contains(out, "| **Rome** |") {
		t.Fatalf("raw markdown table leaked:\n%s", out)
	}
	if !strings.Contains(out, "Rome") {
		t.Fatalf("expected city name in output:\n%s", out)
	}
}

func TestFormatForTelegramReply_BoldOutsideTable(t *testing.T) {
	in := "Hello **world**"
	out := FormatForTelegramReply(in)
	if out != "Hello <b>world</b>" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatForTelegramReply_EscapesHTML(t *testing.T) {
	in := "a <script>x</script> b"
	out := FormatForTelegramReply(in)
	if strings.Contains(out, "<script>") {
		t.Fatalf("unescaped script:\n%s", out)
	}
}

func TestFormatForTelegramReply_FencedCode(t *testing.T) {
	in := "Before:\n```go\nfmt.Println(`x`)\n```\nAfter"
	out := FormatForTelegramReply(in)
	if !strings.Contains(out, "<pre>") {
		t.Fatalf("expected fenced pre:\n%s", out)
	}
	if strings.Contains(out, "```") {
		t.Fatalf("fence leaked:\n%s", out)
	}
}

func TestFormatForTelegramReply_InlineCode(t *testing.T) {
	in := "use `x++` carefully"
	out := FormatForTelegramReply(in)
	if !strings.Contains(out, "<code>") || !strings.Contains(out, "x++") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestFormatForTelegramReply_FencedSpaceTableNotCodeBubble(t *testing.T) {
	in := "```\n#  Capital City   Country\n1  London         United Kingdom\n2  Paris          France\n```"
	out := FormatForTelegramReply(in)
	if strings.Contains(out, "<pre>") {
		t.Fatalf("tabular fence should not become pre/code bubble:\n%s", out)
	}
	if !strings.Contains(out, "London") || !strings.Contains(out, "Paris") {
		t.Fatalf("missing rows:\n%s", out)
	}
	if !strings.Contains(out, "<b>") {
		t.Fatalf("expected bold headers or numbers:\n%s", out)
	}
}
