// Transactions page: deleting with undo, swiping rows on touch screens and
// keyboard shortcuts. Every action answers the whole list (HX-Retarget), so
// requests just need some target.

const UNDO_MS = 5000;
const SWIPE_PX = 90;

function txURL(id, action) {
  return action ? `/transactions/${id}/${action}` : `/transactions/${id}`;
}

function txRequest(verb, url, el) {
  return htmx.ajax(verb, url, { target: el, swap: "outerHTML" });
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
  // txs holds the deletes waiting out their undo delay. Their rows stay in
  // the DOM, hidden, so undoing is instant.
  Alpine.store("txs", {
    pending: {},
    toast: null,

    isPending(id) {
      return id in this.pending;
    },

    remove(el) {
      const { id, label } = el?.dataset ?? {};
      if (!id || this.isPending(id)) return;
      this.pending[id] = {
        url: txURL(id),
        timer: setTimeout(() => this.commit(id), UNDO_MS),
      };
      this.toast = { id, label };
    },

    undo() {
      const id = this.toast?.id;
      if (!id) return;
      clearTimeout(this.pending[id]?.timer);
      delete this.pending[id];
      this.toast = null;
    },

    commit(id) {
      const p = this.pending[id];
      if (!p) return;
      clearTimeout(p.timer);
      if (this.toast?.id === id) this.toast = null;
      // On failure the row shows up again, as it was never swapped out.
      txRequest("DELETE", p.url, "#tx-list").finally(() => delete this.pending[id]);
    },

    // flush sends the pending deletes right away, when leaving the page.
    flush() {
      for (const [id, p] of Object.entries(this.pending)) {
        clearTimeout(p.timer);
        fetch(p.url, { method: "DELETE", keepalive: true });
        delete this.pending[id];
      }
      this.toast = null;
    },
  });

  Alpine.data("txPage", () => ({
    // reviewOnly shows the review strips, when filtering by status=review.
    reviewOnly: false,
    filtersOpen: false,
    activeFilters: 0,

    // syncFilters counts the filters set in the form (the search apart, the
    // date and amount ranges once each).
    syncFilters(form) {
      const set = (name) => !!form.elements[name]?.value.trim();
      const keys = ["kind", "account", "category", "counterparty", "goal", "status", "source"];
      this.activeFilters =
        keys.filter(set).length + (set("from") || set("to")) + (set("min") || set("max"));
      this.reviewOnly = form.elements.status?.value === "review";
    },

    // filterBy narrows the list to what got clicked in it (a category, an
    // account, a month…), keeping the other filters set.
    filterBy(set) {
      closeTxDetails();
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

    // onClick opens the details of the row clicked, unless on one of its
    // controls, selecting its text or ending a swipe.
    onClick(e) {
      const row = e.target.closest?.("[data-tx-row][data-details]");
      if (!row || e.target.closest(TX_ROW_CONTROLS) || getSelection()?.toString()) return;
      if (Date.now() - Number(row.parentElement.dataset.swipedAt ?? 0) < 500) return;
      this.openDetails(row.dataset.details);
    },

    onKey(e) {
      if (e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return;
      const t = e.target;
      if (this.openCreateForm || document.querySelector("[data-tx-details]")) return;
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

      switch (e.key) {
        case "Enter":
          if (!t.matches?.("[data-tx-row][data-details]")) return;
          this.openDetails(t.dataset.details);
          break;
        case "n":
          this.openCreateForm = true;
          break;
        case "j":
          this.focusRow(t, 1);
          break;
        case "k":
          this.focusRow(t, -1);
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
        case "Delete":
        case "Backspace":
          if (!row) return;
          this.focusRow(t, 1) || this.focusRow(t, -1);
          Alpine.store("txs").remove(row);
          break;
        default:
          return;
      }
      e.preventDefault();
    },

    // focusRow moves the focus step rows away from the one holding from (or
    // to the first row), skipping hidden ones. Reports if it moved.
    focusRow(from, step) {
      const rows = [...this.$root.querySelectorAll("[data-tx-row]")].filter(
        (r) => r.offsetParent !== null,
      );
      const cur = rows.indexOf(from.closest?.("[data-tx-row]"));
      const next = cur === -1 ? rows[0] : rows[cur + step];
      if (!next) return false;
      next.focus({ preventScroll: true });
      next.scrollIntoView({ block: "center", behavior: "smooth" });
      return true;
    },
  }));

  // txDetails is the details drawer of a transaction (views.txDrawer). Its
  // content (#txd-panel) is the transaction's details or its edit form, each
  // calling show() as it's swapped in. Deleting acts on the transaction's
  // row in the list, for its undo.
  Alpine.data("txDetails", () => ({
    id: null,
    // mode is "view" or "edit"; from is where the edit form was opened from:
    // "details" goes back to them, "menu" (a row) closes the drawer.
    mode: "view",
    from: null,
    // pristine is the edit form's values when it opened.
    pristine: null,
    // opener is what had the focus before opening, given back on closing.
    opener: null,

    init() {
      if (!this.$root.contains(document.activeElement)) this.opener = document.activeElement;
    },

    show(id, mode, from = null) {
      // A form answered with its errors keeps the values it was opened with.
      const opening = mode === "edit" && this.mode !== "edit";
      this.id = id;
      this.mode = mode;
      this.from = from;
      setTxParam(id);
      this.$nextTick(() => {
        if (opening) this.pristine = this.formState();
        const panel = this.$root.querySelector(".txd-panel");
        const first =
          mode === "edit" &&
          panel?.querySelector("input:not([type=hidden]):not([disabled]), select, textarea, [role=combobox]");
        (first || this.$refs.close)?.focus({ preventScroll: true });
      });
    },

    row() {
      return this.id && document.getElementById(`tx-${this.id}`);
    },

    panel(url) {
      return htmx.ajax("GET", url, { target: "#txd-panel", swap: "innerHTML" });
    },

    close() {
      if (this.$root.hasAttribute("data-closing")) return;
      setTxParam(null);
      const back = this.row()?.querySelector("[data-tx-row]") ?? this.opener;
      this.$root.setAttribute("data-closing", "");
      let done = false;
      const remove = () => {
        if (done) return;
        done = true;
        this.$root.remove();
        back?.focus?.({ preventScroll: true });
      };
      this.$root.querySelector(".txd-drawer")?.addEventListener("animationend", remove, { once: true });
      setTimeout(remove, 250);
    },

    cancel() {
      if (this.from === "menu") this.close();
      else this.panel(txDetailsURL(this.id));
    },

    // escape (on keyup, after an open combobox stopped it for closing itself)
    // cancels editing or closes, unless the datepicker is open.
    escape(e) {
      if (e.defaultPrevented || confirmJustClosed()) return;
      if ([...document.querySelectorAll(".litepicker")].some((p) => p.offsetParent !== null)) return;
      if (this.mode === "edit") this.cancel();
      else this.close();
    },

    // formState is the edit form's values, to tell if they changed.
    formState() {
      const form = this.$root.querySelector("form.txd-panel");
      return form ? new URLSearchParams(new FormData(form)).toString() : null;
    },

    // Clicking outside closes, asking first when the form has changes.
    async backdrop() {
      if (this.mode === "edit" && this.formState() !== this.pristine) {
        const ok = await askConfirm("Discard your changes? The edits to this transaction won't be saved.", {
          label: "Discard",
          danger: true,
        });
        if (!ok) return;
      }
      this.close();
    },

    // saved is the edit form's answer: back to the details, or closing when
    // opened from the row.
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
        this.close();
        Alpine.store("txs").remove(row);
        return;
      }
      // Not in the loaded list, so no undo: ask first.
      if (!(await askConfirm("Delete this transaction? This can't be undone.", { label: "Delete", del: true }))) return;
      this.close();
      txRequest("DELETE", txURL(id), "#tx-list");
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

    // shown hides rows waiting to be deleted.
    get shown() {
      return !Alpine.store("txs").isPending(this.$root.dataset.id);
    },

    // swipe binds the row to the pointer, with x-bind to keep each row's
    // markup short.
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
        layer.innerHTML = `<span class="text-emerald-300">✓ ${right}</span><span class="text-red-300">Delete ✕</span>`;
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
  // Keep the URL the filters go into short.
  if (e.detail.elt.id === "tx-filters") {
    for (const [k, v] of Object.entries(e.detail.parameters)) {
      if (typeof v === "string" && v.trim() === "") delete e.detail.parameters[k];
    }
  }
});

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
