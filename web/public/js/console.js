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

// The console's schema diagram (/console/diagram): the tables' cards are laid
// out in columns by rank (data-rank: referenced tables left), stacked by their
// height, or where they were dragged to (kept in localStorage); the foreign
// keys (data-fks) are curves between their columns' rows. Scrolling pans,
// Ctrl + scrolling (a trackpad's pinch) or two fingers zoom, dragging the
// background pans and a card's header moves it; clicking a card lights up
// its links.
const DIAGRAM_KEY = "consoleDiagram";
const DIAGRAM_GAP_X = 120;
const DIAGRAM_GAP_Y = 32;
const DIAGRAM_PAD = 48;
const SVG_NS = "http://www.w3.org/2000/svg";

function loadDiagramState() {
  try {
    return JSON.parse(localStorage.getItem(DIAGRAM_KEY)) || {};
  } catch {
    return {};
  }
}

document.addEventListener("alpine:init", () => {
  Alpine.data("schemaDiagram", () => ({
    x: 0,
    y: 0,
    k: 1,
    keysOnly: false,
    picked: null,
    // pos is every card's top left, in the world's coordinates; saved the
    // ones dragged.
    pos: {},
    saved: {},
    fks: [],
    cards: {},
    pointers: new Map(),
    gesture: null,

    init() {
      const state = loadDiagramState();
      this.saved = state.pos || {};
      this.keysOnly = !!state.keysOnly;
      try {
        this.fks = JSON.parse(this.$el.dataset.fks || "[]");
      } catch {}
      for (const el of this.$el.querySelectorAll("[data-table]")) this.cards[el.dataset.table] = el;

      this.$el.addEventListener("wheel", (e) => this.onWheel(e), { passive: false });
      this.$el.addEventListener("pointerdown", (e) => this.onDown(e));
      this.$el.addEventListener("pointermove", (e) => this.onMove(e));
      this.$el.addEventListener("pointerup", (e) => this.onUp(e));
      this.$el.addEventListener("pointercancel", (e) => this.onUp(e));
      this.$el.addEventListener("fullscreenchange", () => this.$nextTick(() => this.fit()));

      this.$nextTick(() => {
        this.layout();
        this.fit();
      });
    },

    save() {
      try {
        localStorage.setItem(DIAGRAM_KEY, JSON.stringify({ pos: this.saved, keysOnly: this.keysOnly }));
      } catch {}
    },

    // layout stacks the cards in their rank's column, then puts the dragged
    // ones where they were left.
    layout() {
      const ranks = [];
      for (const [name, el] of Object.entries(this.cards)) {
        const r = +el.dataset.rank || 0;
        (ranks[r] ||= []).push(name);
      }
      let x = 0;
      for (const names of ranks) {
        if (!names) continue;
        let y = 0;
        let width = 0;
        for (const name of names) {
          const el = this.cards[name];
          this.pos[name] = { x, y };
          y += el.offsetHeight + DIAGRAM_GAP_Y;
          width = Math.max(width, el.offsetWidth);
        }
        x += width + DIAGRAM_GAP_X;
      }
      for (const [name, p] of Object.entries(this.saved)) {
        if (this.pos[name]) this.pos[name] = { ...p };
      }
      for (const name of Object.keys(this.cards)) this.place(name);
      this.draw();
    },

    place(name) {
      const p = this.pos[name];
      this.cards[name].style.transform = `translate(${p.x}px, ${p.y}px)`;
    },

    bounds() {
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      for (const [name, el] of Object.entries(this.cards)) {
        const p = this.pos[name];
        minX = Math.min(minX, p.x);
        minY = Math.min(minY, p.y);
        maxX = Math.max(maxX, p.x + el.offsetWidth);
        maxY = Math.max(maxY, p.y + el.offsetHeight);
      }
      return { minX, minY, maxX, maxY };
    },

    fit() {
      if (!Object.keys(this.cards).length) return;
      const b = this.bounds();
      const w = this.$el.clientWidth;
      const h = this.$el.clientHeight;
      this.k = Math.min(1, Math.max(0.15, Math.min((w - 2 * DIAGRAM_PAD) / (b.maxX - b.minX), (h - 2 * DIAGRAM_PAD) / (b.maxY - b.minY))));
      this.x = (w - (b.maxX - b.minX) * this.k) / 2 - b.minX * this.k;
      this.y = Math.max(DIAGRAM_PAD, (h - (b.maxY - b.minY) * this.k) / 2) - b.minY * this.k;
    },

    // zoomAt zooms by factor keeping the point (px, py) of the canvas still.
    zoomAt(px, py, factor) {
      const k = Math.min(2, Math.max(0.15, this.k * factor));
      this.x = px - ((px - this.x) * k) / this.k;
      this.y = py - ((py - this.y) * k) / this.k;
      this.k = k;
    },

    zoomBy(factor) {
      this.zoomAt(this.$el.clientWidth / 2, this.$el.clientHeight / 2, factor);
    },

    toggleKeys() {
      this.keysOnly = !this.keysOnly;
      this.save();
      this.$nextTick(() => this.layout());
    },

    resetLayout() {
      this.saved = {};
      this.save();
      this.layout();
      this.fit();
    },

    fullscreen() {
      if (document.fullscreenElement) document.exitFullscreen();
      else this.$el.requestFullscreen?.();
    },

    onWheel(e) {
      e.preventDefault();
      const r = this.$el.getBoundingClientRect();
      if (e.ctrlKey || e.metaKey) {
        this.zoomAt(e.clientX - r.left, e.clientY - r.top, Math.exp(-e.deltaY * 0.01));
      } else {
        this.x -= e.deltaX;
        this.y -= e.deltaY;
      }
    },

    point(e) {
      const r = this.$el.getBoundingClientRect();
      return { x: e.clientX - r.left, y: e.clientY - r.top };
    },

    onDown(e) {
      if (e.button > 0 || e.target.closest("a, button, [data-diagram-ui]")) return;
      this.$el.setPointerCapture(e.pointerId);
      this.pointers.set(e.pointerId, this.point(e));

      if (this.pointers.size === 2) {
        const [a, b] = [...this.pointers.values()];
        this.gesture = { type: "pinch", dist: Math.hypot(a.x - b.x, a.y - b.y), mid: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } };
        return;
      }
      const card = e.target.closest("[data-table]");
      const start = this.point(e);
      if (card && e.target.closest("[data-drag]")) {
        const name = card.dataset.table;
        this.gesture = { type: "card", name, start, from: { ...this.pos[name] }, moved: false };
      } else {
        this.gesture = { type: "pan", start, from: { x: this.x, y: this.y }, card: card?.dataset.table ?? null, moved: false };
      }
    },

    onMove(e) {
      if (!this.pointers.has(e.pointerId) || !this.gesture) return;
      const p = this.point(e);
      this.pointers.set(e.pointerId, p);
      const g = this.gesture;

      if (g.type === "pinch") {
        if (this.pointers.size < 2) return;
        const [a, b] = [...this.pointers.values()];
        const dist = Math.hypot(a.x - b.x, a.y - b.y);
        const mid = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
        this.x += mid.x - g.mid.x;
        this.y += mid.y - g.mid.y;
        if (g.dist > 0) this.zoomAt(mid.x, mid.y, dist / g.dist);
        g.dist = dist;
        g.mid = mid;
        return;
      }

      const dx = p.x - g.start.x;
      const dy = p.y - g.start.y;
      if (!g.moved && Math.hypot(dx, dy) < 4) return;
      g.moved = true;
      if (g.type === "card") {
        this.pos[g.name] = { x: g.from.x + dx / this.k, y: g.from.y + dy / this.k };
        this.place(g.name);
        this.draw();
      } else {
        this.x = g.from.x + dx;
        this.y = g.from.y + dy;
      }
    },

    onUp(e) {
      if (!this.pointers.delete(e.pointerId)) return;
      const g = this.gesture;
      if (g?.type === "pinch") {
        // The finger left keeps panning from where it is.
        this.gesture = null;
        return;
      }
      this.gesture = null;
      if (!g) return;
      if (g.type === "card" && g.moved) {
        this.saved[g.name] = this.pos[g.name];
        this.save();
      } else if (!g.moved) {
        // A tap: picks the card (its header or body), or none on the
        // background.
        const name = g.type === "card" ? g.name : g.card;
        this.pick(name === this.picked ? null : name);
      }
    },

    pick(name) {
      this.picked = name;
      const linked = new Set();
      if (name) {
        for (const fk of this.fks) {
          if (fk.from === name) linked.add(fk.to);
          if (fk.to === name) linked.add(fk.from);
        }
      }
      for (const [n, el] of Object.entries(this.cards)) {
        el.toggleAttribute("data-picked", n === name);
        el.toggleAttribute("data-linked", !!name && n !== name && linked.has(n));
        el.toggleAttribute("data-dim", !!name && n !== name && !linked.has(n));
      }
      this.draw();
    },

    // anchor is the middle of a column's row on the card (its header when
    // the row is hidden), in the world's coordinates.
    anchor(table, col) {
      const el = this.cards[table];
      const p = this.pos[table];
      const row = el.querySelector(`[data-col="${CSS.escape(col)}"]`);
      const y = row && row.offsetParent ? row.offsetTop + row.offsetHeight / 2 : 18;
      return { left: p.x, right: p.x + el.offsetWidth, y: p.y + y };
    },

    draw() {
      const svg = this.$refs.edges;
      svg.replaceChildren();
      for (const fk of this.fks) {
        if (!this.cards[fk.from] || !this.cards[fk.to]) continue;
        const a = this.anchor(fk.from, fk.fromCol);
        const b = this.anchor(fk.to, fk.toCol);
        let sx, ex, sdir, edir;
        if (fk.from === fk.to || (a.left < b.right && b.left < a.right)) {
          // Itself, or a card over the other: loop out of their right sides.
          sx = a.right;
          ex = b.right;
          sdir = 1;
          edir = 1;
        } else if (b.left >= a.right) {
          sx = a.right;
          ex = b.left;
          sdir = 1;
          edir = -1;
        } else {
          sx = a.left;
          ex = b.right;
          sdir = -1;
          edir = 1;
        }
        const reach = Math.max(40, Math.abs(ex - sx) / 2);
        const path = document.createElementNS(SVG_NS, "path");
        path.setAttribute("d", `M${sx},${a.y} C${sx + sdir * reach},${a.y} ${ex + edir * reach},${b.y} ${ex},${b.y}`);
        path.setAttribute("class", "diagram-edge");
        const end = document.createElementNS(SVG_NS, "circle");
        end.setAttribute("cx", ex);
        end.setAttribute("cy", b.y);
        end.setAttribute("r", 3.5);
        end.setAttribute("class", "diagram-edge-end");
        if (this.picked) {
          const hot = fk.from === this.picked || fk.to === this.picked;
          for (const el of [path, end]) el.toggleAttribute(hot ? "data-hot" : "data-cold", true);
        }
        svg.append(path, end);
      }
    },
  }));
});
