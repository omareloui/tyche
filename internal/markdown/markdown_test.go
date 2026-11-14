package markdown

import (
	"strings"
	"testing"
)

func mustRender(t *testing.T, src string) string {
	t.Helper()
	out, err := render(src)
	if err != nil {
		t.Fatalf("render(%q): %v", src, err)
	}
	return out
}

func TestRenderSanitizes(t *testing.T) {
	for _, src := range []string{
		"<script>alert(1)</script>",
		"hi <script>alert(1)</script>",
		`<img src=x onerror="alert(1)">`,
		"[x](javascript:alert(1))",
		"[x](JaVaScRiPt:alert(1))",
		"[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)",
		"[x](vbscript:msgbox(1))",
		`<a href="https://example.com" onclick="alert(1)">x</a>`,
		`<iframe src="https://example.com"></iframe>`,
		"![i](https://example.com/t.png)",
		"<style>body{display:none}</style>",
		`<div style="position:fixed">x</div>`,
		"<svg onload=alert(1)>",
		"`<script>alert(1)</script>`",
	} {
		out := strings.ToLower(mustRender(t, src))
		for _, bad := range []string{"<script", "onerror", "onclick", "onload", "javascript:", "vbscript:", "data:", "<img", "<iframe", "<style", "style=", "<svg", "<div"} {
			if strings.Contains(out, bad) {
				t.Errorf("render(%q) = %q, has %q", src, out, bad)
			}
		}
	}
}

func TestRenderFormats(t *testing.T) {
	for src, want := range map[string]string{
		"**b** and *i*":                   "<strong>b</strong> and <em>i</em>",
		"~~gone~~":                        "<del>gone</del>",
		"- one\n- two":                    "<li>one</li>",
		"1. one\n2. two":                  "<ol>",
		"a\nb":                            "a<br",
		"> quoted":                        "<blockquote>",
		"`code`":                          "<code>code</code>",
		"`<b>`":                           "<code>&lt;b&gt;</code>",
		"- [x] done":                      `checked`,
		"| a | b |\n|---|---|\n| 1 | 2 |": "<td>1</td>",
	} {
		if out := mustRender(t, src); !strings.Contains(out, want) {
			t.Errorf("render(%q) = %q, want %q in it", src, out, want)
		}
	}
}

func TestRenderLinks(t *testing.T) {
	out := mustRender(t, "[site](https://example.com) and https://example.org")
	for _, want := range []string{`href="https://example.com"`, `target="_blank"`, "noopener", "noreferrer", "nofollow", `href="https://example.org"`} {
		if !strings.Contains(out, want) {
			t.Errorf("links rendered as %q, want %q in it", out, want)
		}
	}
	if out := mustRender(t, "- [ ] todo"); !strings.Contains(out, "disabled") {
		t.Errorf("task list rendered as %q, want its checkbox disabled", out)
	}
}

func TestPlain(t *testing.T) {
	for src, want := range map[string]string{
		"**Hi** there\nmore":             "Hi there\nmore",
		"# Title\n\n- a\n- b":            "Title\na\nb",
		"[site](https://example.com)":    "site",
		"Tom & Jerry <script>x</script>": "Tom & Jerry x",
		"":                               "",
	} {
		if got := Plain(src); got != want {
			t.Errorf("Plain(%q) = %q, want %q", src, got, want)
		}
	}
	if got := FirstLine("\n\n**Hi** there\nmore"); got != "Hi there" {
		t.Errorf("FirstLine = %q, want %q", got, "Hi there")
	}
}
