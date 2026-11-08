// Transactions page: deleting with undo, swiping rows on touch screens,
// selecting rows to edit or delete them together, and keyboard shortcuts.
// Every action answers the whole list (HX-Retarget), so requests just need
// some target.

const UNDO_MS = 5000;
// txDeleteSeq tells the deletes sent apart, for their answers.
let txDeleteSeq = 0;
const SWIPE_PX = 90;
// HOLD_MS is how long holding a row (touch) takes to select it.
const HOLD_MS = 450;

// txRowIds are the transactions of a row (its wrapper, [data-sel-ids]): one,
// or both legs of a transfer pair.
function txRowIds(el) {
  return (el?.dataset.selIds ?? el?.dataset.id ?? "").split(" ").filter(Boolean);
}

// txSend sends a request whose answer (the list, HX-Retarget) doesn't go
// to a row, resolving to whether it went through and the error message.
function txSend(verb, url, values) {
  const key = String(++txDeleteSeq);
  return new Promise((resolve) => {
    const done = (e) => {
      if (e.detail.requestConfig?.headers?.["X-Tx-Req"] !== key) return;
      document.removeEventListener("htmx:afterRequest", done);
      const xhr = e.detail.xhr;
      const ok = !!xhr && xhr.status > 0 && xhr.status < 400;
      resolve({ ok, error: ok ? "" : xhr?.responseText?.trim() || "Something went wrong, try again." });
    };
    document.addEventListener("htmx:afterRequest", done);
    htmx.ajax(verb, url, { target: "#tx-sel-sink", swap: "innerHTML", values, headers: { "X-Tx-Req": key } });
  });
}

// txSelectionValues is the selection (txSel) as sent with a bulk action:
// its rows' ids, or every row the page's filter matches.
function txSelectionValues() {
  return Alpine.store("txSel").values();
}

function txURL(id, action) {
  return action ? `/transactions/${id}/${action}` : `/transactions/${id}`;
}

// txRequest sends an action of a row, from it so it shows busy meanwhile
// (its hx-indicator).
function txRequest(verb, url, el) {
  return htmx.ajax(verb, url, { source: el, target: el, swap: "outerHTML" });
}

function txDetailsURL(id) {
  return `/transactions/${encodeURIComponent(id)}/details`;
}

// setTxParam keeps the open details in the url (?tx=), so reloading the page
// opens them again.
function setTxParam(id) {
  const u = new URL(location.href);
  if (id) u.searchParams.set("tx", id);
  else u.searchParams.delete("tx");
  if (u.href !== location.href) history.replaceState(history.state, "", u);
}

// closeTxDetails closes the open details drawer, if any.
function closeTxDetails() {
  const el = document.querySelector("[data-tx-details]");
  if (el) Alpine.$data(el).close();
}

// Clicks on these, in a row, do their own thing instead of opening the
// row's details.
const TX_ROW_CONTROLS = "button, a, input, select, textarea, label, [role=menu], [tabindex]:not([data-tx-row])";

document.addEventListener("alpine:init", () => {
  // txs holds the deletes waiting out their undo delay, all sent at once
  // when it ends (each delete restarts it), or undone together. Their rows
  // stay in the DOM, hidden by a stylesheet out of the swapped list, so
  // undoing is instant and the list re-rendering (other actions, the delete's
  // answer) never shows them back.
  Alpine.store("txs", {
    // pending is the deletes the toast offers to undo, as {ids, el, label}:
    // the row's transactions and its element's id.
    pending: [],
    timer: null,
    // toast is the shown toast: its text, and a key restarting its countdown.
    toast: null,
    // left is the seconds left before the pending deletes are sent.
    left: 0,
    deadline: 0,
    ticker: null,
    // hidden is every row hidden, by element id: pending ones and sent ones
    // (gone from the list once answered; shown again if the delete fails).
    hidden: new Set(),

    remove(el) {
      this.removeMany([el]);
    },

    // removeMany deletes rows (their wrappers) with one undo.
    removeMany(els) {
      const rows = els.filter((el) => el?.id && txRowIds(el).length && !this.hidden.has(el.id));
      if (!rows.length) return;
      // The focus in a row (e.g. its menu's Delete) goes on to the next
      // row, as deleting with the key does, rather than to the page.
      const focused = rows.find((el) => el.contains(document.activeElement));
      if (focused) txNeighbor(focused.querySelector("[data-tx-row]"))?.focus({ preventScroll: true });
      for (const el of rows) this.pending.push({ ids: txRowIds(el), el: el.id, label: el.dataset.label });
      this.hide(rows.map((el) => el.id));
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.commit(), UNDO_MS);
      this.countDown();
      const n = this.pending.length;
      this.toast = {
        text: n === 1 ? `Deleted “${this.pending[0].label}”` : `Deleted ${n} transactions`,
        key: (this.toast?.key ?? 0) + 1,
      };
    },

    // countDown (re)starts the seconds the toast shows.
    countDown() {
      this.deadline = Date.now() + UNDO_MS;
      const tick = () => (this.left = Math.max(0, Math.ceil((this.deadline - Date.now()) / 1000)));
      tick();
      this.ticker ??= setInterval(tick, 200);
    },

    stopCountDown() {
      clearInterval(this.ticker);
      this.ticker = null;
    },

    undo() {
      clearTimeout(this.timer);
      this.stopCountDown();
      this.show(this.pending.map((p) => p.el));
      this.pending = [];
      this.toast = null;
    },

    commit() {
      clearTimeout(this.timer);
      this.stopCountDown();
      const ids = this.pending.flatMap((p) => p.ids);
      const els = this.pending.map((p) => p.el);
      this.pending = [];
      this.toast = null;
      if (!ids.length) return;
      // Shown back if it fails (htmx shows the error).
      const key = String(++txDeleteSeq);
      const done = (e) => {
        if (e.detail.requestConfig?.headers?.["X-Tx-Delete"] !== key) return;
        document.removeEventListener("htmx:afterRequest", done);
        if (!e.detail.successful) this.show(els);
      };
      document.addEventListener("htmx:afterRequest", done);
      htmx.ajax("POST", "/transactions/delete", {
        target: "#tx-list",
        swap: "outerHTML",
        values: { id: ids },
        headers: { "X-Tx-Delete": key },
      });
    },

    // flush sends the pending deletes right away, when leaving the page.
    flush() {
      clearTimeout(this.timer);
      this.stopCountDown();
      if (this.pending.length) {
        const body = new URLSearchParams(this.pending.flatMap((p) => p.ids.map((id) => ["id", id])));
        fetch("/transactions/delete", { method: "POST", body, keepalive: true });
      }
      this.pending = [];
      this.toast = null;
      this.show([...this.hidden]);
    },

    hide(ids) {
      for (const id of ids) this.hidden.add(id);
      this.paint();
    },

    show(ids) {
      for (const id of ids) this.hidden.delete(id);
      this.paint();
    },

    paint() {
      let style = document.getElementById("tx-hidden");
      if (!style) {
        style = document.createElement("style");
        style.id = "tx-hidden";
        document.head.append(style);
      }
      style.textContent = [...this.hidden].map((id) => `#${CSS.escape(id)}{display:none!important}`).join("");
    },
  });

  // txSel is the rows selected to edit or delete together: some rows (by
  // their wrapper's id, [data-sel-ids]), or all of the rows the filter
  // matches, loaded or not (the summary's count, sent as the filter and
  // checked by the server). Rows show it as data-selected (paint), kept
  // across the list's re-renders; changing the filters clears it.
  Alpine.store("txSel", {
    on: false,
    // sel maps the selected rows' element ids to their transactions' ids.
    sel: {},
    all: false,
    // total is how many rows the filter matches, loaded is how many (top
    // level) rows are; both read from the list (refresh).
    total: 0,
    loaded: 0,
    // last is the row toggled last, where a shift-click range starts.
    last: null,
    notice: "",
    noticeTimer: null,

    get count() {
      return this.all ? this.total : Object.keys(this.sel).length;
    },

    clear() {
      this.on = false;
      this.sel = {};
      this.all = false;
      this.last = null;
      this.paint();
    },

    // rows are the selectable rows loaded, nested ones (transfer sides,
    // reimbursements) included; top only the list's own.
    rows(top = false) {
      const hidden = Alpine.store("txs").hidden;
      const sel = top ? "#tx-list [data-sel-top]" : "#tx-list [data-sel-ids]";
      return [...document.querySelectorAll(sel)].filter((el) => !hidden.has(el.id));
    },

    // toggle selects or unselects a row; range selects the rows from the one
    // toggled last to it.
    toggle(el, range = false) {
      if (!el?.id) return;
      this.on = true;
      if (this.all) {
        // Unselecting one of all: the loaded ones less it.
        this.all = false;
        this.selectLoaded();
      }
      const rows = this.rows().filter((r) => txShown(r.querySelector("[data-tx-row]")));
      const from = range && this.last ? rows.findIndex((r) => r.id === this.last) : -1;
      const to = rows.indexOf(el);
      if (from !== -1 && to !== -1) {
        for (const r of rows.slice(Math.min(from, to), Math.max(from, to) + 1)) this.sel[r.id] = txRowIds(r);
      } else if (this.sel[el.id]) {
        delete this.sel[el.id];
      } else {
        this.sel[el.id] = txRowIds(el);
      }
      this.last = el.id;
      this.refresh();
    },

    selectLoaded() {
      for (const r of this.rows(true)) this.sel[r.id] = txRowIds(r);
      this.refresh();
    },

    hasMore() {
      return (document.getElementById("tx-until")?.dataset.until ?? "all") !== "all";
    },

    // canSelectMore offers the loaded rows not selected yet, then all of the
    // filter's when there are more to load.
    canSelectMore() {
      if (this.all) return false;
      return this.count < this.loaded || (this.hasMore() && this.total > this.count);
    },

    moreText() {
      if (this.count < this.loaded) return this.hasMore() ? `Select the ${this.loaded} loaded` : `Select all ${this.loaded}`;
      return `Select all ${this.total} matching`;
    },

    selectMore() {
      if (this.count < this.loaded) this.selectLoaded();
      else {
        this.all = true;
        this.paint();
      }
    },

    countText() {
      if (this.all) return `All ${this.total} selected`;
      return this.count ? `${this.count} selected` : "Tap transactions to select them";
    },

    // refresh reads the list's counts again and paints the rows.
    refresh() {
      this.loaded = this.rows(true).length;
      this.total = Number(document.getElementById("tx-summary")?.dataset.total ?? 0);
      this.paint();
    },

    paint() {
      for (const el of document.querySelectorAll("#tx-list [data-sel-ids]")) {
        const on = this.on && (!!this.sel[el.id] || (this.all && el.hasAttribute("data-sel-top")));
        el.toggleAttribute("data-selected", on);
      }
    },

    values() {
      if (this.all) {
        const v = Object.fromEntries(new URLSearchParams(location.search));
        return { ...v, all: "1", expect: String(this.total) };
      }
      return { id: [...new Set(Object.values(this.sel).flat())] };
    },

    // edit opens the bulk edit drawer, which first drops the rows that can't
    // be edited (tx-sel-drop).
    edit() {
      if (!this.count) return;
      htmx.ajax("POST", "/transactions/bulk-edit/form", { target: "#drawer", swap: "innerHTML", values: this.values() });
    },

    // drop unselects the rows holding any of ids (e.g. the ones that can't
    // be edited); all of the filter's stay so, the server leaving them out.
    drop(ids) {
      if (this.all || !ids?.length) return;
      const out = new Set(ids);
      for (const [key, rowIds] of Object.entries(this.sel)) {
        if (rowIds.some((id) => out.has(id))) delete this.sel[key];
      }
      this.paint();
    },

    // remove deletes the selected rows with an undo; all of the filter's
    // (not all loaded) once confirmed.
    async remove() {
      if (!this.count) return;
      if (!this.all) {
        const els = Object.keys(this.sel)
          .map((id) => document.getElementById(id))
          .filter(Boolean);
        this.clear();
        Alpine.store("txs").removeMany(els);
        return;
      }
      const n = this.total;
      if (!(await askConfirm(`Delete all ${n} transactions matching the filters? This can't be undone.`, { label: "Delete", del: true })))
        return;
      const { ok, error } = await txSend("POST", "/transactions/delete", this.values());
      if (ok) this.clear();
      this.notify(ok ? `Deleted ${n} transactions` : error);
    },

    notify(text) {
      this.notice = text;
      clearTimeout(this.noticeTimer);
      this.noticeTimer = setTimeout(() => (this.notice = ""), 5000);
    },
  });

  Alpine.data("txPage", () => ({
    // reviewOnly shows the review strips, when filtering by status=review.
    reviewOnly: false,
    filtersOpen: false,
    activeFilters: 0,
    // lastG is when g was pressed, for gg.
    lastG: 0,

    // syncFilters counts the filters set in the form (the search apart, the
    // date and amount ranges once each).
    syncFilters(form) {
      const set = (name) => !!form.elements[name]?.value.trim();
      const keys = ["kind", "account", "category", "counterparty", "goal", "status", "source", "receipt"];
      this.activeFilters =
        keys.filter(set).length + (set("from") || set("to")) + (set("min") || set("max"));
      this.reviewOnly = form.elements.status?.value === "review";
    },

    // filterBy narrows the list to what got clicked in it (a category, an
    // account, a month…), keeping the other filters set.
    filterBy(set) {
      closeTxDetails();
      Alpine.store("txSel").clear();
      const params = new URLSearchParams(location.search);
      params.delete("tx");
      for (const [k, v] of Object.entries(set)) params.set(k, v);
      const url = `/transactions?${params}`;
      htmx
        .ajax("GET", url, { target: "#tx-body", swap: "innerHTML show:window:top" })
        .then(() => history.replaceState({ htmx: true }, "", url));
    },

    openDetails(id) {
      htmx.ajax("GET", txDetailsURL(id), { target: "#tx-details", swap: "innerHTML" });
    },

    // openEdit opens the drawer right in the transaction's edit form.
    openEdit(id) {
      htmx.ajax("GET", txURL(id, "edit"), { target: "#tx-details", swap: "innerHTML" });
    },

    // onSelectClick, before anything in the row gets the click: while
    // selecting, a click anywhere on a row toggles it; out of it, on a row's
    // tile turned check (txSelTile) starts selecting.
    onSelectClick(e) {
      const sel = Alpine.store("txSel");
      const box = e.target.closest?.("[data-sel-box]");
      const row = e.target.closest?.("[data-tx-row]");
      if (!row || (!sel.on && !box)) return;
      // A menu opened before selecting still works.
      if (!box && e.target.closest("[role=menu]")) return;
      e.preventDefault();
      e.stopPropagation();
      const wrap = row.closest("[data-sel-ids]");
      // The click ending a hold already selected it (txRow).
      if (Date.now() - Number(wrap?.dataset.heldAt ?? 0) < 600) return;
      sel.toggle(wrap, e.shiftKey);
    },

    // onClick opens the details of the row clicked, unless on one of its
    // controls, selecting its text or ending a swipe.
    onClick(e) {
      const row = e.target.closest?.("[data-tx-row][data-details]");
      if (!row || e.target.closest(TX_ROW_CONTROLS) || getSelection()?.toString()) return;
      if (Date.now() - Number(row.parentElement.dataset.swipedAt ?? 0) < 500) return;
      this.openDetails(row.dataset.details);
    },

    // toggleShortcuts opens or closes the keyboard shortcuts' dialog.
    toggleShortcuts() {
      const d = document.getElementById("tx-shortcuts");
      if (!d) return;
      if (d.open) d.close();
      else d.showModal();
    },

    onKey(e) {
      if (e.metaKey || e.altKey || e.defaultPrevented) return;
      const t = e.target;
      // The shortcuts' dialog takes no other key while open (Escape closes it).
      if (document.getElementById("tx-shortcuts")?.open) {
        if (e.key === "?") {
          e.preventDefault();
          this.toggleShortcuts();
        }
        return;
      }
      if (this.openCreateForm || document.querySelector("[data-tx-details], #drawer [data-drawer]")) return;
      if (e.ctrlKey) {
        if (e.shiftKey || t.closest?.("input, textarea, select, [contenteditable], form, [role=listbox], [role=menu]")) return;
        if (!this.vimScroll(e.key, t)) return;
        e.preventDefault();
        return;
      }
      if (e.key === "Escape" && Alpine.store("txSel").on && !t.closest?.("input, textarea, select, [role=listbox], [role=menu]")) {
        e.preventDefault();
        Alpine.store("txSel").clear();
        return;
      }
      if (e.key === "/" && !t.closest?.("input, textarea, select, [contenteditable]")) {
        e.preventDefault();
        this.$refs.q?.focus();
        this.$refs.q?.select();
        return;
      }
      if (t.closest?.("input, textarea, select, [contenteditable], form, [role=listbox], [role=menu]")) return;

      // Edit forms are data-tx too, without an id.
      const row = t.closest?.("[data-tx][data-id]");
      const d = row?.dataset ?? {};

      // gg is the first row (a g, then another within a moment).
      const gg = e.key === "g" && Date.now() - this.lastG < 600;
      this.lastG = e.key === "g" && !gg ? Date.now() : 0;

      switch (e.key) {
        case "?":
          this.toggleShortcuts();
          break;
        case "Enter":
          if (!t.matches?.("[data-tx-row][data-details]")) return;
          this.openDetails(t.dataset.details);
          break;
        case "n":
          this.openCreateForm = true;
          break;
        case "s": {
          const scan = document.querySelector("[data-receipt-quick]");
          if (!scan) return;
          scan.click();
          break;
        }
        case "j":
          this.focusRow(t, 1);
          break;
        case "k":
          this.focusRow(t, -1);
          break;
        case "g":
          if (!gg) return;
          this.focusEdge(true);
          break;
        case "G":
          this.focusEdge(false);
          break;
        case "e":
          if (!row) return;
          this.openEdit(d.id);
          break;
        case "f":
          if (!("scheduled" in d)) return;
          txRequest("PUT", txURL(d.id, "fulfill"), row);
          break;
        case "r":
          if (!("review" in d)) return;
          txRequest("PUT", txURL(d.id, "reviewed"), row);
          break;
        case " ":
          // Space only selects while selecting, else it scrolls the page.
          if (!Alpine.store("txSel").on) return;
        // falls through
        case "x":
        case "X": {
          const wrap = t.closest?.("[data-sel-ids]");
          if (!wrap) return;
          Alpine.store("txSel").toggle(wrap, e.shiftKey);
          break;
        }
        case "Delete":
        case "Backspace":
          if (Alpine.store("txSel").on) {
            Alpine.store("txSel").remove();
            break;
          }
          if (!row) return;
          this.focusRow(t, 1) || this.focusRow(t, -1);
          Alpine.store("txs").remove(row);
          break;
        default:
          return;
      }
      e.preventDefault();
    },

    // vimScroll scrolls the page as vim does for Ctrl+key: e/y a row, d/u
    // half a screen, f/b a screen (less the sticky headers, so no row is
    // skipped). A focused row scrolled out of view hands the focus on to the
    // nearest one in view. Reports if key is one of them.
    vimScroll(key, from) {
      const sticky = txStickyHeight();
      const page = window.innerHeight - sticky;
      const steps = { e: 64, y: -64, d: page / 2, u: -page / 2, f: page, b: -page };
      const by = steps[key];
      if (by === undefined) return false;
      window.scrollBy({ top: by, behavior: key === "e" || key === "y" ? "instant" : "smooth" });

      const cur = from.closest?.(TX_STOPS);
      if (!cur || !this.$root.contains(cur)) return true;
      // Where the page ends up once scrolled (a smooth scroll is still going).
      const top = Math.max(0, Math.min(window.scrollY + by, document.documentElement.scrollHeight - window.innerHeight));
      const shift = top - window.scrollY;
      const inView = (el) => {
        const r = el.getBoundingClientRect();
        return r.top - shift >= sticky && r.bottom - shift <= window.innerHeight;
      };
      if (inView(cur)) return true;
      const rows = [...this.$root.querySelectorAll(TX_STOPS)].filter(txShown).filter(inView);
      const next = by > 0 ? rows[0] : rows.at(-1);
      next?.focus({ preventScroll: true });
      return true;
    },

    // focusEdge focuses the first row (the upcoming section's header when
    // there's one), or the last one loaded.
    focusEdge(first) {
      const rows = [...this.$root.querySelectorAll(TX_STOPS)].filter(txShown);
      const el = first ? rows[0] : rows.at(-1);
      if (!el) return;
      el.focus({ preventScroll: true });
      el.scrollIntoView({ block: "center", behavior: "smooth" });
    },

    // focusRow moves the focus step rows away from the one holding from (or
    // to the first row), skipping hidden ones. The upcoming section's header
    // (data-tx-stop) is one too, to open or fold it (Enter).
    // Reports if it moved.
    focusRow(from, step) {
      const stops = TX_STOPS;
      const rows = [...this.$root.querySelectorAll(stops)].filter(txShown);
      let cur = rows.indexOf(from.closest?.(stops));
      if (cur === -1 && from instanceof Node && this.$root.contains(from) && from !== this.$root) {
        // Off the rows (e.g. a month's header): from the nearest one in the
        // direction, going on from where the focus is.
        const after = rows.findIndex((r) => from.compareDocumentPosition(r) & Node.DOCUMENT_POSITION_FOLLOWING);
        cur = step > 0 ? (after === -1 ? rows.length : after) - 1 : after === -1 ? rows.length : after;
      }
      const next = cur === -1 ? rows[0] : rows[cur + step];
      if (!next) return false;
      next.focus({ preventScroll: true });
      next.scrollIntoView({ block: "center", behavior: "smooth" });
      return true;
    },
  }));

  // txDetails is the details drawer of a transaction (views.txDrawer), a
  // drawer (drawerBase, drawer.js) whose content (#txd-panel) is the
  // transaction's details or its edit form, each calling show() as it's
  // swapped in. Deleting acts on the transaction's row in the list, for its
  // undo.
  Alpine.data("txDetails", () => {
    const base = drawerBase();
    return {
      ...base,
      id: null,
      // mode is "view", "edit" or "split" (views.TxSplitPanel, saved on
      // every change); from is where the form was opened from: "details"
      // goes back to them, "menu" (a row) closes the drawer.
      mode: "view",
      from: null,

      // The panels' show() does what base.init would.
      init() {
        if (!this.drawerRoot().contains(document.activeElement)) this.opener = document.activeElement;
      },

      // form is the url the form was fetched from, the edit form's when
      // not set, kept in the url to open it again on reloading.
      show(id, mode, from = null, form = null) {
        // A form answered with its errors keeps the values it was opened with;
        // the split's are saved on every change.
        const opening = (mode === "edit" && this.mode !== "edit") || mode === "split";
        // Back closes the drawer, or leaves the form opened from the details
        // (its own entry) for them.
        const root = this.drawerRoot();
        const top = drawerHistory.top();
        if (top?.el !== root) drawerHistory.opened(root, (e) => this.back(e), "tx", id);
        else if (mode !== "view" && from === "details" && top.kind !== "child")
          drawerHistory.push({ el: root, kind: "child", back: (e) => this.back(e), form: form ?? txURL(id, "edit") });
        else if (mode === "view" && top.kind === "child") drawerHistory.drop(1);
        this.id = id;
        this.mode = mode;
        this.from = from;
        setTxParam(id);
        this.$nextTick(() => {
          if (opening) this.pristine = this.formState();
          if (mode === "edit") this.focusFirst();
          else this.drawerRoot().querySelector("[x-ref=close]")?.focus({ preventScroll: true });
        });
      },

      row() {
        return this.id && document.getElementById(`tx-${this.id}`);
      },

      panel(url) {
        return htmx.ajax("GET", url, { target: "#txd-panel", swap: "innerHTML" });
      },

      close(fromHistory = false) {
        setTxParam(null);
        this.opener = this.row()?.querySelector("[data-tx-row]") ?? this.opener;
        base.close.call(this, fromHistory);
      },

      // back is the browser's Back (drawerHistory): the edit form opened from
      // the details goes back to them, else closing.
      async back(entry) {
        if (!(await this.confirmDiscard())) return drawerHistory.push(entry);
        if (entry.kind === "child") this.panel(txDetailsURL(this.id));
        else this.close(true);
      },

      // cancel leaves the edit form: back to the details, or closing when
      // opened from the row.
      async cancel() {
        if (!(await this.confirmDiscard())) return;
        if (this.from === "menu") this.close();
        else this.panel(txDetailsURL(this.id));
      },

      escape(e) {
        if (e.defaultPrevented || confirmJustClosed() || datepickerOpen()) return;
        if (this.mode !== "view") this.cancel();
        else this.close();
      },

      saved() {
        if (this.from === "menu") this.close();
      },

      // reload gets the details again after an action changed them.
      reload(e) {
        if (e.detail.successful) this.panel(txDetailsURL(this.id));
      },

      // remove deletes with undo when the row is loaded, else right away.
      async remove() {
        const id = this.id;
        const row = this.row();
        if (row) {
          // The focus goes on to the next row, as deleting with the key.
          this.opener = txNeighbor(row.querySelector("[data-tx-row]")) ?? this.opener;
          base.close.call(this);
          setTxParam(null);
          Alpine.store("txs").remove(row);
          return;
        }
        // Not in the loaded list, so no undo: ask first.
        if (!(await askConfirm("Delete this transaction? This can't be undone.", { label: "Delete", del: true })))
          return;
        this.close();
        txRequest("DELETE", txURL(id), "#tx-list");
      },
    };
  });

  // splitChips are who had an item of a split bill (views.splitItem):
  // picking everyone unpicks the others, picking someone unpicks everyone.
  Alpine.data("splitChips", () => ({
    pick(e) {
      const box = e.target;
      if (!box.checked) return;
      for (const other of this.$root.querySelectorAll("input[type=checkbox]")) {
        if (other !== box && (box.value === "all" || other.value === "all")) other.checked = false;
      }
    },
  }));

  // splitUnits are how many of an item's qty units each one had
  // (views.splitItemUnits): a step changes someone's, within what's left
  // (the user's), and saves the split.
  Alpine.data("splitUnits", (qty) => ({
    inputs() {
      return [...this.$root.querySelectorAll("input[data-id]")];
    },
    count(input) {
      return Number(input.value.split(":")[1]) || 0;
    },
    mine() {
      return qty - this.inputs().reduce((sum, i) => sum + this.count(i), 0);
    },
    step(btn, d) {
      const input = btn.closest("[data-units]").querySelector("input[data-id]");
      const n = this.count(input) + d;
      if (n < 0 || (d > 0 && this.mine() <= 0)) return;
      input.value = `${input.dataset.id}:${n}`;
      btn.closest("[data-units]").querySelector("[data-count]").textContent = n;
      this.refresh();
      input.dispatchEvent(new Event("change", { bubbles: true }));
    },
    // refresh dims the steps that can't be taken: − at 0, + with nothing
    // left (before the answer re-renders them).
    refresh() {
      const left = this.mine();
      this.$root.querySelector("[data-mine]").textContent = left;
      for (const row of this.$root.querySelectorAll("[data-units]")) {
        const n = this.count(row.querySelector("input[data-id]"));
        row.querySelector("[data-step='-1']").disabled = n <= 0;
        row.querySelector("[data-step='1']").disabled = left <= 0;
      }
    },
  }));

  // txRow is a transaction row; swiping it right fulfills it (or marks it
  // reviewed), left deletes it.
  Alpine.data("txRow", () => ({
    openMenu: false,
    openReimbursements: false,
    openTransferPair: false,
    dx: 0,
    x0: null,
    y0: null,
    swiping: false,
    holdTimer: null,

    // swipe binds the row to the pointer, with x-bind to keep each row's
    // markup short. Holding it still selects it (txSel).
    swipe: {
      "@pointerdown"(e) {
        this.down(e);
      },
      "@pointermove"(e) {
        this.move(e);
      },
      "@pointerup"() {
        this.up();
      },
      "@pointercancel"() {
        this.cancel();
      },
      // Holding a row is selecting it, not the browser's menu.
      "@contextmenu"(e) {
        if (this.holdTimer || Date.now() - Number(this.$root.dataset.heldAt ?? 0) < 1000) e.preventDefault();
      },
      ":style"() {
        return this.dx ? `transform: translateX(${this.dx}px)` : "";
      },
      ":class"() {
        return this.x0 === null ? "transition-transform" : "";
      },
    },

    get rightUrl() {
      const d = this.$root.dataset;
      if ("scheduled" in d) return txURL(d.id, "fulfill");
      if ("review" in d) return txURL(d.id, "reviewed");
      return null;
    },

    down(e) {
      if (e.pointerType !== "touch") return;
      this.x0 = e.clientX;
      this.y0 = e.clientY;
      this.swiping = false;
      clearTimeout(this.holdTimer);
      this.holdTimer = setTimeout(() => this.hold(), HOLD_MS);
    },

    // hold selects the row held, without the click ending it toggling it
    // back (txPage.onSelectClick) nor opening its details.
    hold() {
      this.holdTimer = null;
      if (this.swiping) return;
      const wrap = this.$root;
      wrap.dataset.heldAt = Date.now();
      wrap.dataset.swipedAt = Date.now();
      const sel = Alpine.store("txSel");
      if (!sel.sel[wrap.id] || !sel.on) sel.toggle(wrap);
      navigator.vibrate?.(10);
      // Moving the finger on, before lifting it, selects the rows it goes
      // over.
      txDragSelect(wrap, this.x0, this.y0);
      this.cancel();
    },

    // swipeLayer is revealed under the row while swiping, created on the
    // first swipe instead of rendered with every row.
    swipeLayer() {
      let layer = this.$root.querySelector(":scope > [data-swipe-layer]");
      if (!layer) {
        const right = "scheduled" in this.$root.dataset ? "Fulfill" : "Reviewed";
        layer = document.createElement("div");
        layer.dataset.swipeLayer = "";
        layer.className =
          "absolute inset-x-0 top-0 flex items-center justify-between px-5 rounded-2xl text-sm font-semibold transition-colors";
        layer.innerHTML =
          `<span class="inline-flex items-center gap-1.5 text-emerald-300"><span class="icon-check"></span>${right}</span>` +
          `<span class="inline-flex items-center gap-1.5 text-red-300">Delete<span class="icon-x"></span></span>`;
        this.$root.prepend(layer);
      }
      return layer;
    },

    paintSwipe() {
      const layer = this.swipeLayer();
      const [right, left] = layer.children;
      const past = Math.abs(this.dx) >= SWIPE_PX;
      layer.hidden = this.dx === 0;
      layer.style.height = `${this.$root.querySelector("[data-tx-row]").offsetHeight}px`;
      right.hidden = this.dx <= 0;
      left.hidden = this.dx >= 0;
      layer.style.backgroundColor =
        this.dx > 0
          ? `rgb(16 185 129 / ${past ? 0.4 : 0.15})`
          : `rgb(239 68 68 / ${past ? 0.4 : 0.15})`;
      left.style.marginLeft = "auto";
    },

    move(e) {
      if (this.x0 === null) return;
      const dx = e.clientX - this.x0;
      const dy = e.clientY - this.y0;
      if (Math.abs(dx) > 10 || Math.abs(dy) > 10) {
        clearTimeout(this.holdTimer);
        this.holdTimer = null;
      }
      // Selecting, rows don't swipe.
      if (Alpine.store("txSel").on) return;
      if (!this.swiping) {
        // Scrolling, leave it to the page.
        if (Math.abs(dy) > 10 && Math.abs(dy) > Math.abs(dx)) return this.cancel();
        if (Math.abs(dx) < 10) return;
        this.swiping = true;
      }
      const limit = SWIPE_PX * 1.5;
      this.dx = dx > 0 && !this.rightUrl ? 0 : Math.max(-limit, Math.min(limit, dx));
      this.paintSwipe();
    },

    up() {
      clearTimeout(this.holdTimer);
      this.holdTimer = null;
      if (this.x0 === null) return;
      // The click ending a swipe doesn't open the details (txPage.onClick).
      if (this.swiping) this.$root.dataset.swipedAt = Date.now();
      const dx = this.dx;
      this.cancel();
      if (dx <= -SWIPE_PX) {
        Alpine.store("txs").remove(this.$root);
      } else if (dx >= SWIPE_PX && this.rightUrl) {
        txRequest("PUT", this.rightUrl, this.$root);
      }
    },

    cancel() {
      clearTimeout(this.holdTimer);
      this.holdTimer = null;
      if (this.swiping) {
        this.dx = 0;
        this.paintSwipe();
      }
      this.x0 = null;
      this.dx = 0;
      this.swiping = false;
    },
  }));
});

window.addEventListener("pagehide", () => Alpine.store("txs")?.flush());
// Boosted navigation swaps the page without unloading it.
document.addEventListener("htmx:beforeHistorySave", () => Alpine.store("txs")?.flush());

// Re-rendering the list after changing a transaction keeps the older months
// loaded by scrolling (see handler.loadedTxListData).
document.addEventListener("htmx:configRequest", (e) => {
  const until = document.getElementById("tx-until")?.dataset.until;
  if (until) e.detail.headers["X-Tx-Until"] = until;
  // Re-rendered lists keep the upcoming groups open or folded.
  const upcoming = document.getElementById("tx-upcoming");
  if (upcoming) e.detail.headers["X-Tx-Upcoming"] = upcoming.open ? "open" : "closed";
  const later = document.getElementById("tx-upcoming-later");
  if (later) e.detail.headers["X-Tx-Upcoming-Later"] = later.open ? "open" : "closed";
  // Keep the URL the filters go into short.
  if (e.detail.elt.id === "tx-filters") {
    for (const [k, v] of Object.entries(e.detail.parameters)) {
      if (typeof v === "string" && v.trim() === "") delete e.detail.parameters[k];
    }
  }
});

// The selection follows the list's re-renders (and chunks loaded), cleared
// when the filters change, as the rows it had may be gone.
document.addEventListener("htmx:afterSettle", () => {
  const sel = window.Alpine?.store("txSel");
  if (sel?.on) sel.refresh();
});
document.addEventListener("htmx:afterRequest", (e) => {
  const cfg = e.detail.requestConfig;
  if (cfg?.verb !== "get" || cfg.path?.split("?")[0] !== "/transactions") return;
  window.Alpine?.store("txSel")?.clear();
});
// Opening the bulk edit dropped rows that can't be edited
// (handler.BulkEditTransactionsForm), telling why when none could.
document.addEventListener("tx-sel-drop", (e) => {
  const sel = Alpine.store("txSel");
  sel.drop(e.detail?.ids);
  if (e.detail?.message) sel.notify(e.detail.message);
});
// A bulk edit got saved (handler.BulkEditTransactions).
document.addEventListener("tx-bulk-done", (e) => {
  const sel = Alpine.store("txSel");
  const { updated = 0, skipped = 0 } = e.detail ?? {};
  sel.clear();
  let text = `Updated ${updated} ${updated === 1 ? "transaction" : "transactions"}`;
  if (skipped) text += ` · ${skipped} skipped where it didn't fit`;
  sel.notify(text);
});

// TX_DRAG_EDGE is how close (px) to the list's visible edges (under the
// header, above the selection bar) a drag selecting rows scrolls the page,
// faster the closer, up to TX_DRAG_SPEED px a frame.
const TX_DRAG_EDGE = 80;
const TX_DRAG_SPEED = 18;

// txDragSelect selects, from a row held (txRow.hold) until the finger
// lifts, the rows from it to the one under the finger; the ones it leaves
// go back to how they were. Near the top or bottom it scrolls the page,
// selecting on. It's touch only: it stops the page from scrolling under
// the finger meanwhile (a non-passive touchmove, as the rows' touch-action
// lets them pan).
function txDragSelect(anchor, x, y) {
  const sel = Alpine.store("txSel");
  const before = { ...sel.sel };
  let last = anchor;
  let raf = 0;
  let pending = false;
  // moved tells the finger left where it held, before which it doesn't
  // scroll (e.g. holding a row by the bottom edge).
  const x0 = x;
  const y0 = y;
  let moved = false;

  const rows = () => sel.rows().filter((r) => txShown(r.querySelector("[data-tx-row]")));

  // update selects the range from the anchor to the row at (x, y).
  const update = () => {
    pending = false;
    const hit = document.elementFromPoint(x, y)?.closest("#tx-list [data-sel-ids]");
    if (hit) last = hit;
    const list = rows();
    const a = list.indexOf(anchor);
    const b = list.indexOf(last);
    if (a === -1 || b === -1) return;
    const [from, to] = a < b ? [a, b] : [b, a];
    list.forEach((r, i) => {
      if (i >= from && i <= to) sel.sel[r.id] = txRowIds(r);
      else if (before[r.id]) sel.sel[r.id] = before[r.id];
      else delete sel.sel[r.id];
    });
    sel.refresh();
  };
  const schedule = () => {
    if (!pending) {
      pending = true;
      requestAnimationFrame(update);
    }
  };

  // scroll moves the page while the finger is near an edge.
  const scroll = () => {
    raf = requestAnimationFrame(scroll);
    if (!moved) return;
    const header = document.querySelector("header")?.getBoundingClientRect();
    const bar = document.getElementById("tx-sel-bar")?.getBoundingClientRect();
    const top = header?.height ? Math.max(0, header.bottom) : 0;
    const bottom = bar?.height ? bar.top : innerHeight;
    let dy = 0;
    if (y < top + TX_DRAG_EDGE) dy = -TX_DRAG_SPEED * Math.min(1, (top + TX_DRAG_EDGE - y) / TX_DRAG_EDGE);
    else if (y > bottom - TX_DRAG_EDGE) dy = TX_DRAG_SPEED * Math.min(1, (y - bottom + TX_DRAG_EDGE) / TX_DRAG_EDGE);
    if (dy) {
      const was = scrollY;
      scrollBy(0, dy);
      if (scrollY !== was) update();
    }
  };

  const move = (e) => {
    const t = e.touches[0];
    if (!t) return;
    if (e.cancelable) e.preventDefault();
    x = t.clientX;
    y = t.clientY;
    moved ||= Math.hypot(x - x0, y - y0) > 10;
    schedule();
  };
  const end = () => {
    cancelAnimationFrame(raf);
    document.removeEventListener("touchmove", move);
    document.removeEventListener("touchend", end);
    document.removeEventListener("touchcancel", end);
    // The click lifting the finger doesn't toggle the row it's on
    // (txPage.onSelectClick).
    last.dataset.heldAt = Date.now();
    anchor.dataset.heldAt = Date.now();
  };
  document.addEventListener("touchmove", move, { passive: false });
  document.addEventListener("touchend", end);
  document.addEventListener("touchcancel", end);
  raf = requestAnimationFrame(scroll);
}

// TX_STOPS are where j/k move the focus: the rows and the upcoming
// section's header.
const TX_STOPS = "[data-tx-row], [data-tx-stop]";

// txNeighbor is the shown row (or stop) after the one of el, else the one
// before, to give the focus to when el's goes.
function txNeighbor(el) {
  const rows = [...document.querySelectorAll(TX_STOPS)].filter(txShown);
  const i = rows.indexOf(el?.closest?.(TX_STOPS));
  if (i === -1) return null;
  return rows[i + 1] ?? rows[i - 1] ?? null;
}

// txShown tells if a row (or the upcoming section's header) shows: not in a
// hidden parent, nor in a folded details, whose content browsers may only
// hide with content-visibility (so it still has an offsetParent).
function txShown(el) {
  if (el.offsetParent === null) return false;
  const folded = el.closest("details:not([open])");
  return !folded || el.closest("summary")?.parentElement === folded;
}

// txStickyHeight is how much of the top of the screen the sticky nav and
// month header cover.
function txStickyHeight() {
  const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
  const month = document.querySelector("[data-tx-month-head]")?.offsetHeight ?? 0;
  return 4 * rem + month;
}

// --tx-month-h is the height of a month's header, which the days' headers
// stick under (it differs between phones and wider screens).
const txMonthHeadObserver = new ResizeObserver((entries) => {
  const h = entries.at(-1)?.target.offsetHeight;
  if (h) document.getElementById("tx-list")?.style.setProperty("--tx-month-h", `${h}px`);
});
function txObserveMonthHead() {
  txMonthHeadObserver.disconnect();
  const head = document.querySelector("#tx-list [data-tx-month-head]");
  if (head) txMonthHeadObserver.observe(head);
}
document.addEventListener("htmx:afterSettle", txObserveMonthHead);
txObserveMonthHead();

// Keep the keyboard focus on the same transaction across list re-renders.
let txRefocusId = null;
document.addEventListener("htmx:beforeSwap", () => {
  txRefocusId = document.activeElement?.closest?.("[data-tx][data-id]")?.dataset.id ?? null;
});
document.addEventListener("htmx:afterSettle", () => {
  if (!txRefocusId) return;
  document.querySelector(`#tx-${txRefocusId} [data-tx-row]`)?.focus({ preventScroll: true });
  txRefocusId = null;
});
