# Extra brand icons

Brands missing from [simple-icons](https://github.com/simple-icons/simple-icons)
(`third_party/simple-icons`), e.g. the ones on its
[forbidden list](https://github.com/simple-icons/simple-icons/blob/develop/CONTRIBUTING.md#forbidden-brands)
like Amazon, Disney and Microsoft. Entries here take precedence over
simple-icons for both detection and serving (`/brand-icons/{slug}.svg`).

To add a brand:

1. Add `icons/{slug}.svg`: a single-color svg on a `0 0 24 24` viewBox (the
   fill is ignored, it's tinted with `hex`).
2. Add an entry to `brands.json`, same format as simple-icons'
   `data/simple-icons.json`:

```json
{
	"title": "DiDi",
	"hex": "FC9153",
	"source": "https://www.didiglobal.com",
	"aliases": { "aka": ["Didi Mobility"] }
}
```

`slug` is optional, it defaults to the title lowercased with only `a-z0-9`
kept (`+` → `plus`, `.` → `dot`, `&` → `and`).

Each entry's `source` records where its svg came from (old simple-icons
releases, Wikimedia Commons, Iconify or the brand's own site).
