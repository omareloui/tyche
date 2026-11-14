// Package markdown renders the users' notes, stored as they typed them, as
// Markdown. Rendered HTML can only reach a page through templ.Raw, which
// escapes nothing, so every render goes through goldmark (raw HTML left out,
// dangerous link schemes dropped) then a bluemonday allowlist.
package markdown

import (
	"bytes"
	"context"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/a-h/templ"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// md renders GitHub flavored Markdown, a newline staying a line break as
// notes were written before being Markdown. Without WithUnsafe raw HTML is
// left out.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
)

// policy is what a rendered note may hold: text formatting, lists, quotes,
// code, tables and links (http, https, mailto; opening in a new tab). No
// images (remote ones would load trackers), styles, classes or handlers.
var policy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "del", "code", "pre", "blockquote",
		"ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "hr",
		"table", "thead", "tbody", "tr", "th", "td")
	p.AllowAttrs("start").Matching(bluemonday.Integer).OnElements("ol")
	p.AllowAttrs("align").Matching(regexp.MustCompile(`^(left|center|right)$`)).OnElements("th", "td")
	p.AllowAttrs("href").OnElements("a")
	p.AllowStandardURLs()
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	// Task lists' checkboxes, read only.
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	return p
}()

var strict = bluemonday.StrictPolicy()

// render is src as sanitized HTML.
func render(src string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return policy.Sanitize(buf.String()), nil
}

// HTML is the note rendered from Markdown, sanitized; the note as escaped
// text if it can't be rendered.
func HTML(src string) templ.Component {
	out, err := render(src)
	if err != nil {
		out = "<p>" + html.EscapeString(src) + "</p>"
	}
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, out)
		return err
	})
}

var spaces = regexp.MustCompile(`[ \t]+`)

// Plain is the note as plain text, its Markdown left out, e.g. for a title
// attribute: one line per block or line break.
func Plain(src string) string {
	out, err := render(src)
	if err != nil {
		return src
	}
	// Line breaks between blocks and at <br>s before the tags go.
	out = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "</p>", "\n", "</li>", "\n", "</tr>", "\n",
		"</h1>", "\n", "</h2>", "\n", "</h3>", "\n", "</h4>", "\n", "</h5>", "\n", "</h6>", "\n").Replace(out)
	text := html.UnescapeString(strict.Sanitize(out))
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(spaces.ReplaceAllString(line, " ")); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// FirstLine is the note's first line as plain text, for one line spots like
// a card.
func FirstLine(src string) string {
	line, _, _ := strings.Cut(Plain(src), "\n")
	return line
}
