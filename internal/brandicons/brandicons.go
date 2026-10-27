// Package brandicons indexes the simple-icons dataset to detect brand icons
// for counterparties by their name or aliases.
package brandicons

import (
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/logger"
	"go.uber.org/zap"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Minimum slug lengths to avoid false positives: short exact names like "HP"
// are fine, but partial matches on tokens like "eg" or "bv" are not.
const (
	minExactLen     = 2
	minCandidateLen = 3
)

type Icon struct {
	Slug  string
	Title string
	Hex   string
}

type Index struct {
	bySlug map[string]Icon
	byKey  map[string]string
}

type rawIcon struct {
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	Hex     string `json:"hex"`
	Aliases struct {
		Aka []string                 `json:"aka"`
		Dup []struct{ Title string } `json:"dup"`
		Loc map[string]string        `json:"loc"`
	} `json:"aliases"`
}

func emptyIndex() *Index {
	return &Index{bySlug: map[string]Icon{}, byKey: map[string]string{}}
}

// Source is an icons dataset: a simple-icons style json and its svgs dir.
type Source struct {
	JSON  string
	Icons string
}

// SimpleIcons is the source of a simple-icons checkout.
func SimpleIcons(dir string) Source {
	return Source{JSON: filepath.Join(dir, "data", "simple-icons.json"), Icons: filepath.Join(dir, "icons")}
}

// Extra is the source of the local brands simple-icons lacks, laid out as
// `brands.json` next to an `icons/` dir.
func Extra(dir string) Source {
	return Source{JSON: filepath.Join(dir, "brands.json"), Icons: filepath.Join(dir, "icons")}
}

// Load merges the sources into one index, where earlier sources take
// precedence for both slugs and detection keys.
func Load(sources ...Source) (*Index, error) {
	idx := emptyIndex()
	for _, src := range sources {
		srcIdx, err := loadSource(src)
		if err != nil {
			return nil, err
		}
		for slug, icon := range srcIdx.bySlug {
			if _, ok := idx.bySlug[slug]; !ok {
				idx.bySlug[slug] = icon
			}
		}
		for key, slug := range srcIdx.byKey {
			idx.addKey(key, slug)
		}
	}
	return idx, nil
}

// loadSource keeps only entries that have a matching svg.
func loadSource(src Source) (*Index, error) {
	data, err := os.ReadFile(src.JSON)
	if err != nil {
		return nil, err
	}

	var raws []rawIcon
	if err := json.Unmarshal(data, &raws); err != nil {
		// Legacy format: {"icons": [...]}
		var legacy struct {
			Icons []rawIcon `json:"icons"`
		}
		if lerr := json.Unmarshal(data, &legacy); lerr != nil {
			return nil, errors.Join(err, lerr)
		}
		raws = legacy.Icons
	}

	idx := emptyIndex()
	type iconAliases struct {
		slug    string
		aliases []string
	}
	pending := []iconAliases{}
	for _, raw := range raws {
		slug := raw.Slug
		if slug == "" {
			slug = TitleToSlug(raw.Title)
		}
		if slug == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(src.Icons, slug+".svg")); err != nil {
			continue
		}

		idx.bySlug[slug] = Icon{Slug: slug, Title: raw.Title, Hex: strings.ToUpper(raw.Hex)}

		idx.byKey[slug] = slug
		idx.addKey(TitleToSlug(raw.Title), slug)

		aliases := append([]string{}, raw.Aliases.Aka...)
		for _, d := range raw.Aliases.Dup {
			aliases = append(aliases, d.Title)
		}
		for _, l := range raw.Aliases.Loc {
			aliases = append(aliases, l)
		}
		pending = append(pending, iconAliases{slug, aliases})
	}

	// Aliases go last so they never shadow another icon's own slug or title.
	for _, p := range pending {
		for _, a := range p.aliases {
			idx.addKey(TitleToSlug(a), p.slug)
		}
	}
	return idx, nil
}

func (idx *Index) addKey(key, slug string) {
	if _, ok := idx.byKey[key]; key == "" || ok {
		return
	}
	idx.byKey[key] = slug
}

var (
	defaultIdx  *Index
	defaultOnce sync.Once
)

// Default lazily loads the index from the configured extra brands and
// simple-icons dirs, extra brands first. A missing dataset is skipped, so the
// app runs without (some) icons.
func Default() *Index {
	defaultOnce.Do(func() {
		sources := []Source{}
		for _, src := range defaultSources() {
			if _, err := os.Stat(src.JSON); err != nil {
				logger.Get().Warn("skipping brand icons source", zap.String("json", src.JSON), zap.Error(err))
				continue
			}
			sources = append(sources, src)
		}
		idx, err := Load(sources...)
		if err != nil {
			logger.Get().Warn("loading brand icons, brand icons disabled", zap.Error(err))
			idx = emptyIndex()
		}
		defaultIdx = idx
	})
	return defaultIdx
}

func defaultSources() []Source {
	return []Source{Extra(config.GetExtraBrandIconsDir()), SimpleIcons(config.GetSimpleIconsDir())}
}

// IconsFS serves the svgs of the default sources, extra brands first.
func IconsFS() fs.FS {
	srcs := defaultSources()
	fsys := make(overlayFS, len(srcs))
	for i, src := range srcs {
		fsys[i] = os.DirFS(src.Icons)
	}
	return fsys
}

// overlayFS opens a file from the first file system that has it.
type overlayFS []fs.FS

func (o overlayFS) Open(name string) (fs.File, error) {
	var err error
	for _, fsys := range o {
		var f fs.File
		if f, err = fsys.Open(name); err == nil {
			return f, nil
		}
	}
	return nil, err
}

func (idx *Index) Get(slug string) (Icon, bool) {
	i, ok := idx.bySlug[slug]
	return i, ok
}

// Detect returns the slug of the first icon matching the name or one of the
// aliases, or "" if none matches.
func (idx *Index) Detect(name string, aliases []string) string {
	for _, s := range append([]string{name}, aliases...) {
		if slug := idx.match(s); slug != "" {
			return slug
		}
	}
	return ""
}

func (idx *Index) match(s string) string {
	if key := TitleToSlug(s); len(key) >= minExactLen {
		if slug, ok := idx.byKey[key]; ok {
			return slug
		}
	}

	// Bank SMS merchants look like "UBER *TRIP" or "CARREFOUR MAADI", so try
	// the longest leading run of words first.
	tokens := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for n := len(tokens); n > 0; n-- {
		candidate := TitleToSlug(strings.Join(tokens[:n], ""))
		if len(candidate) < minCandidateLen {
			continue
		}
		if slug, ok := idx.byKey[candidate]; ok {
			return slug
		}
	}
	return ""
}

var slugReplacer = strings.NewReplacer(
	"+", "plus", ".", "dot", "&", "and",
	"đ", "d", "ħ", "h", "ı", "i", "ĸ", "k", "ŀ", "l",
	"ł", "l", "ß", "ss", "ŧ", "t", "ø", "o",
)

// TitleToSlug ports simple-icons' `titleToSlug`.
func TitleToSlug(title string) string {
	s := slugReplacer.Replace(strings.ToLower(title))
	s, _, _ = transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), s)

	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// placeholderColors are tailwind 600 shades, readable under white text.
var placeholderColors = []string{
	"DC2626", "EA580C", "D97706", "16A34A", "059669", "0D9488", "0891B2",
	"0284C7", "2563EB", "4F46E5", "7C3AED", "9333EA", "C026D3", "DB2777", "E11D48",
}

// RandomColor picks a placeholder color for counterparties without an icon.
func RandomColor() string {
	return placeholderColors[rand.IntN(len(placeholderColors))]
}

// ColorFor returns the brand color of a known icon. Without one it keeps the
// current color, falling back to a random placeholder color.
func (idx *Index) ColorFor(slug, current string) string {
	if icon, ok := idx.Get(slug); ok {
		return icon.Hex
	}
	if current != "" {
		return current
	}
	return RandomColor()
}

// IsDark reports whether a hex color is too dark to read on a dark
// background.
func IsDark(hex string) bool {
	l, ok := luminance(hex)
	return ok && l < 0.05
}

// IsLight reports whether a hex color is too light to hold white text.
func IsLight(hex string) bool {
	l, ok := luminance(hex)
	return ok && l > 0.4
}

// luminance returns the WCAG relative luminance of a 6 digit hex color.
func luminance(hex string) (float64, bool) {
	hex = strings.TrimPrefix(hex, "#")
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return 0, false
	}
	lin := func(c uint64) float64 {
		f := float64(c) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(v>>16&0xff) + 0.7152*lin(v>>8&0xff) + 0.0722*lin(v&0xff), true
}
