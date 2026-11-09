// The SQL console's editor (/console): the textarea's text is transparent,
// under a <pre> holding it highlighted, redrawn on input. The
// tables of the database (data-tables, JSON) are highlighted as such.

const SQL_KEYWORDS = new Set(
  `abort action add after all alter always analyze and as asc attach autoincrement before begin between by cascade
case cast check collate column commit conflict constraint create cross current current_date current_time
current_timestamp database default deferrable deferred delete desc detach distinct do drop each else end escape
except exclude exclusive exists explain fail filter first following for foreign from full generated glob group
groups having if ignore immediate in index indexed initially inner insert instead intersect into is isnull join key
last left like limit match materialized natural no not nothing notnull null nulls of offset on or order others
outer over partition plan pragma preceding primary query raise range recursive references regexp reindex release
rename replace restrict returning right rollback row rows savepoint select set table temp temporary then ties to
transaction trigger unbounded union unique update using vacuum values view virtual when where window with without
true false`.split(/\s+/),
);

// SQL_TOKEN matches a token at a time: comments, strings, quoted names,
// blobs, numbers, words, then anything else one character at a time.
const SQL_TOKEN =
  /(--[^\n]*|\/\*[\s\S]*?(?:\*\/|$))|('(?:[^']|'')*'?)|("(?:[^"]|"")*"?|`[^`]*`?|\[[^\]]*\]?)|([xX]'[0-9a-fA-F]*'?|\b\d+(?:\.\d*)?(?:[eE][+-]?\d+)?\b|\.\d+\b)|([A-Za-z_][\w$]*)|([\s\S])/g;

function escapeHTML(s) {
  return s.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]);
}

function unquoteName(s) {
  const q = s[0];
  const close = q === "[" ? "]" : q;
  let name = s.slice(1);
  if (name.endsWith(close)) name = name.slice(0, -1);
  return name.replaceAll(close + close, close).toLowerCase();
}

function highlightSQL(sql, tables) {
  let html = "";
  for (const m of sql.matchAll(SQL_TOKEN)) {
    const [tok, comment, str, quoted, num, word] = m;
    let cls = "";
    if (comment) cls = "sql-comment";
    else if (str) cls = "sql-string";
    else if (quoted) cls = tables.has(unquoteName(quoted)) ? "sql-table" : "sql-name";
    else if (num) cls = "sql-number";
    else if (word) {
      const lower = word.toLowerCase();
      if (SQL_KEYWORDS.has(lower)) cls = "sql-keyword";
      else if (tables.has(lower)) cls = "sql-table";
      else if (/^\s*\(/.test(sql.slice(m.index + tok.length))) cls = "sql-function";
    } else if (/[(),;.*=<>!+\-/%|]/.test(tok)) cls = "sql-punct";
    html += cls ? `<span class="${cls}">${escapeHTML(tok)}</span>` : escapeHTML(tok);
  }
  // A trailing newline needs a line after it to take its height.
  return html + "\n";
}

document.addEventListener("alpine:init", () => {
  Alpine.data("sqlEditor", () => ({
    tables: new Set(),

    init() {
      try {
        this.tables = new Set(JSON.parse(this.$el.dataset.tables || "[]").map((t) => t.toLowerCase()));
      } catch {}
      this.render();
    },

    // render highlights the SQL and grows the textarea to fit it, so
    // neither scrolls (a scrollbar would wrap their lines differently).
    render() {
      const ta = this.$refs.input;
      this.$refs.code.innerHTML = highlightSQL(ta.value, this.tables);
      ta.style.height = "auto";
      ta.style.height = ta.scrollHeight + 2 + "px";
    },
  }));
});
