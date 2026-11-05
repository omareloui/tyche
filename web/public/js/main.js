// entryMenuMove moves the focus to the next (dir 1) or previous (-1) item of
// a components.EntryMenu, wrapping around.
function entryMenuMove(menu, dir) {
  const items = [...menu.querySelectorAll("[role=menuitem]")];
  if (!items.length) return;
  const i = items.indexOf(document.activeElement);
  items[(i + dir + items.length) % items.length].focus();
}

// askConfirm asks question through components.ConfirmDialog instead of the
// browser's confirm(), resolving to whether it got confirmed. The question
// is split at its first "? ", the rest shown as details. label names the
// confirm button, danger makes it red, del shows a trash icon.
function askConfirm(question, { label = "Confirm", danger = false, del = false } = {}) {
  const dialog = document.getElementById("confirm-dialog");
  if (!dialog?.showModal) return Promise.resolve(confirm(question));

  const i = question.indexOf("? ");
  dialog.querySelector("#confirm-title").textContent = i === -1 ? question : question.slice(0, i + 1);
  dialog.querySelector("#confirm-details").textContent = i === -1 ? "" : question.slice(i + 2);
  dialog.querySelector("#confirm-ok").textContent = label;
  dialog.toggleAttribute("data-danger", danger || del);
  dialog.toggleAttribute("data-delete", del);

  dialog.returnValue = "";
  return new Promise((resolve) => {
    dialog.addEventListener(
      "close",
      () => {
        // Escape closing it shouldn't reach what's under it on keyup (e.g.
        // the transaction drawer, cancelling its edit form).
        dialog.dataset.closedAt = Date.now();
        resolve(dialog.returnValue === "ok");
      },
      { once: true },
    );
    dialog.showModal();
    dialog.querySelector("#confirm-ok").focus();
  });
}

// confirmJustClosed reports if the confirm dialog closed a moment ago.
function confirmJustClosed() {
  const at = Number(document.getElementById("confirm-dialog")?.dataset.closedAt ?? 0);
  return Date.now() - at < 300;
}

// hx-confirm asks through askConfirm. On the confirming element,
// data-confirm-label names the confirm button (else "Delete" for hx-delete,
// "Confirm" otherwise) and data-confirm-danger makes it red.
document.addEventListener("htmx:confirm", (e) => {
  const question = e.detail.question;
  if (!question || !document.getElementById("confirm-dialog")?.showModal) return;
  e.preventDefault();

  const elt = e.detail.elt;
  const del = e.detail.verb === "delete";
  askConfirm(question, {
    label: elt.dataset.confirmLabel || (del ? "Delete" : "Confirm"),
    danger: "confirmDanger" in elt.dataset,
    del,
  }).then((ok) => ok && e.detail.issueRequest(true));
});

// Clicking the backdrop (outside the dialog's box) cancels.
document.addEventListener("click", (e) => {
  const dialog = e.target;
  if (dialog.id !== "confirm-dialog" || !dialog.open) return;
  const r = dialog.getBoundingClientRect();
  const inside = e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
  if (!inside) dialog.close("cancel");
});

// Page switches (the navbar's boosted links) show a top bar at once, then
// blur the page with a spinner if slow. The bar and spinner live on <html>,
// out of the swapped <body>.
(() => {
  const root = document.documentElement;
  const bar = document.createElement("div");
  bar.id = "nav-progress";
  bar.setAttribute("aria-hidden", "true");
  const loading = document.createElement("div");
  loading.id = "nav-loading";
  loading.setAttribute("role", "status");
  loading.setAttribute("aria-live", "polite");
  loading.innerHTML = '<span class="nav-spinner" aria-hidden="true"></span><span class="nav-loading-text">Loading</span>';
  root.append(bar, loading);

  let slow;
  const done = () => {
    clearTimeout(slow);
    delete root.dataset.navigating;
    document.querySelectorAll(".nav-pending").forEach((a) => a.classList.remove("nav-pending"));
  };

  document.addEventListener("htmx:beforeRequest", (e) => {
    if (!e.detail.boosted) return;
    done();
    root.dataset.navigating = "";
    slow = setTimeout(() => (root.dataset.navigating = "slow"), 150);
    if (e.detail.elt.tagName === "A") e.detail.elt.classList.add("nav-pending");
  });
  // The request's element is detached by the swap, so its afterRequest
  // never reaches the document: the swap (on <body>) ends it instead.
  for (const ev of ["htmx:afterSwap", "htmx:afterRequest", "htmx:sendError", "htmx:timeout", "htmx:historyRestore"]) {
    document.addEventListener(ev, (e) => {
      if (ev === "htmx:historyRestore" || e.detail.boosted) done();
    });
  }
  addEventListener("pageshow", done);

  // Actions (any request but a GET) slower than a moment show the bar too,
  // without the blur. Counted by their xhr, as their element may be swapped
  // out before afterRequest.
  let busy = 0;
  let busyTimer;
  document.addEventListener("htmx:beforeSend", (e) => {
    const xhr = e.detail.xhr;
    if (e.detail.boosted || e.detail.requestConfig?.verb === "get" || !xhr) return;
    if (busy++ === 0) busyTimer = setTimeout(() => (root.dataset.busy = ""), 150);
    xhr.addEventListener("loadend", () => {
      if (--busy > 0) return;
      clearTimeout(busyTimer);
      delete root.dataset.busy;
    });
  });
})();

// components.HoverPopup hangs right-aligned under its group (group/popup), so on narrow
// screens it can overflow the left edge (or the right one): shift it back in
// when its group is hovered or focused.
(() => {
  const margin = 8;
  const fit = (e) => {
    const group = e.target.closest?.(".group\\/popup");
    const popup = group?.querySelector(":scope [data-hover-popup]");
    if (!popup || popup.closest(".group\\/popup") !== group) return;
    popup.style.translate = "";
    // Measured unscaled: it may be mid scale-in, pinned at its top-right.
    const right = popup.getBoundingClientRect().right;
    const left = right - popup.offsetWidth;
    const vw = document.documentElement.clientWidth;
    let dx = 0;
    if (left < margin) dx = margin - left;
    else if (right > vw - margin) dx = vw - margin - right;
    if (dx) popup.style.translate = `${dx}px 0`;
  };
  document.addEventListener("mouseover", fit);
  document.addEventListener("focusin", fit);
})();

// The budget summary (categories page) hides categories per browser: their
// ids (or "none" for uncategorized) are kept in localStorage and sent as
// `hide` whenever it renders.
const budgetHiddenKey = "tyche:budget-hidden";

function budgetHidden() {
  try {
    const ids = JSON.parse(localStorage.getItem(budgetHiddenKey));
    return Array.isArray(ids) ? ids : [];
  } catch {
    return [];
  }
}

// The budget summary's sections (the burn-up + bars, the categories without
// a budget) start collapsed, then stay as last left.
const budgetOpenKey = "tyche:budget-open";

function budgetOpenSections() {
  try {
    const keys = JSON.parse(localStorage.getItem(budgetOpenKey));
    return Array.isArray(keys) ? keys : [];
  } catch {
    return [];
  }
}

function budgetSectionOpen(key) {
  return budgetOpenSections().includes(key);
}

function setBudgetSectionOpen(key, open) {
  const keys = budgetOpenSections().filter((k) => k !== key);
  if (open) keys.push(key);
  try {
    localStorage.setItem(budgetOpenKey, JSON.stringify(keys));
  } catch {}
}

function loadBudgetSummary(month) {
  const params = new URLSearchParams({ month, hide: budgetHidden().join(",") });
  htmx.ajax("GET", `/categories/budget?${params}`, {
    target: "#categories-budget",
    swap: "outerHTML",
  });
}

document.addEventListener("click", (e) => {
  const summary = e.target.closest("[data-budget-summary]");
  if (!summary) return;

  const nav = e.target.closest("[data-budget-month]");
  if (nav) {
    loadBudgetSummary(nav.dataset.budgetMonth);
    return;
  }

  const toggle = e.target.closest("[data-budget-toggle]");
  if (!toggle) return;
  const id = toggle.dataset.budgetToggle;
  const ids = budgetHidden();
  const i = ids.indexOf(id);
  if (i === -1) ids.push(id);
  else ids.splice(i, 1);
  try {
    localStorage.setItem(budgetHiddenKey, JSON.stringify(ids));
  } catch {}
  loadBudgetSummary(summary.dataset.month);
});

// The counterparties summary hides merchants per browser the same way, their
// ids kept in localStorage and sent as `hide`.
const cpSummaryHiddenKey = "tyche:cp-summary-hidden";

function cpSummaryHidden() {
  try {
    const ids = JSON.parse(localStorage.getItem(cpSummaryHiddenKey));
    return Array.isArray(ids) ? ids : [];
  } catch {
    return [];
  }
}

function loadCpSummary(month) {
  const params = new URLSearchParams({ month, hide: cpSummaryHidden().join(",") });
  htmx.ajax("GET", `/counterparties/summary?${params}`, {
    target: "#counterparties-summary",
    swap: "outerHTML",
  });
}

document.addEventListener("click", (e) => {
  const summary = e.target.closest("[data-cp-summary]");
  if (!summary) return;

  const nav = e.target.closest("[data-cp-summary-month]");
  if (nav) {
    loadCpSummary(nav.dataset.cpSummaryMonth);
    return;
  }

  const toggle = e.target.closest("[data-cp-summary-toggle]");
  if (!toggle) return;
  const id = toggle.dataset.cpSummaryToggle;
  const ids = cpSummaryHidden();
  const i = ids.indexOf(id);
  if (i === -1) ids.push(id);
  else ids.splice(i, 1);
  try {
    localStorage.setItem(cpSummaryHiddenKey, JSON.stringify(ids));
  } catch {}
  loadCpSummary(summary.dataset.month);
});

// The accounts summary's options per browser: counting the accounts out of
// the balance ("excluded", "goals"), leaving out the budgets' left
// ("budget"), kept in localStorage and sent as `with`.
const accSummaryWithKey = "tyche:acc-summary-with";

function accSummaryWith() {
  try {
    const keys = JSON.parse(localStorage.getItem(accSummaryWithKey));
    return Array.isArray(keys) ? keys : [];
  } catch {
    return [];
  }
}

document.addEventListener("click", (e) => {
  const toggle = e.target.closest("[data-acc-summary] [data-acc-summary-with]");
  if (!toggle) return;
  const key = toggle.dataset.accSummaryWith;
  const keys = accSummaryWith().filter((k) => k !== key);
  if (toggle.getAttribute("aria-pressed") !== "true") keys.push(key);
  try {
    localStorage.setItem(accSummaryWithKey, JSON.stringify(keys));
  } catch {}
  htmx.ajax("GET", `/accounts/summary?${new URLSearchParams({ with: keys.join(",") })}`, {
    target: "#accounts-summary",
    swap: "outerHTML",
  });
});

// Tooltips holding amounts (title) swap to their data-hidden-title while the
// navbar's eye toggle hides the amounts, set on hover before they show.
document.addEventListener(
  "mouseover",
  (e) => {
    const el = e.target.closest?.("[data-hidden-title]");
    if (!el) return;
    el.dataset.shownTitle ??= el.title;
    el.title = document.documentElement.hasAttribute("data-hide-amounts")
      ? el.dataset.hiddenTitle
      : el.dataset.shownTitle;
  },
  true,
);

// createFormFromURL opens a page's create form (layouts.WithCreateForm) when
// its url has ?new, e.g. from the PWA's shortcuts or reloading with it open
// (drawerHistory keeps it in the url meanwhile).
function createFormFromURL(data) {
  if (new URL(location.href).searchParams.has("new")) data.openCreateForm = true;
}

// The service worker (sw.js) only caches static files and the offline page.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => navigator.serviceWorker.register("/sw.js").catch(() => {}));
}

// installPrompt is the browser's deferred install prompt, shown by the user
// menu's "Install app" (components.Navbar) while the browser offers one.
// Browsers without it (iOS, already installed) never show the item.
let installPrompt = null;
window.addEventListener("beforeinstallprompt", (e) => {
  e.preventDefault();
  installPrompt = e;
  window.dispatchEvent(new CustomEvent("install-available", { detail: true }));
});
window.addEventListener("appinstalled", () => {
  installPrompt = null;
  window.dispatchEvent(new CustomEvent("install-available", { detail: false }));
});

function canInstall() {
  return installPrompt !== null;
}

async function installApp() {
  if (!installPrompt) return;
  const p = installPrompt;
  installPrompt = null;
  window.dispatchEvent(new CustomEvent("install-available", { detail: false }));
  p.prompt();
  await p.userChoice;
}

// settingsForm keeps a settings card's Save button disabled until its form
// differs from how the card loaded (settings.templ).
document.addEventListener("alpine:init", () => {
  Alpine.data("settingsForm", () => ({
    pristine: null,
    dirty: false,

    init() {
      // After the nested inputs' init, e.g. x-mask formatting the amount.
      setTimeout(() => {
        this.pristine = this.state();
      });
    },

    state() {
      const form = this.$root.querySelector("form");
      return form ? new URLSearchParams(new FormData(form)).toString() : "";
    },

    check() {
      this.dirty = this.pristine !== null && this.state() !== this.pristine;
    },
  }));
});

// chartScrub shows a chart's tooltips under a finger on touch screens, where
// group-hover never applies (Tailwind's hover needs a hover-capable pointer):
// touching or dragging across the chart marks the target under the finger
// `is-active` (its tooltip shown by group-[.is-active]), kept a moment after
// lifting it. Its children are the targets, side by side; touch-pan-y on it
// leaves vertical drags to scrolling (cancelling the scrub).
document.addEventListener("alpine:init", () => {
  Alpine.data("chartScrub", () => ({
    timer: null,

    scrub: {
      "@pointerdown"(e) {
        this.track(e);
      },
      "@pointermove"(e) {
        this.track(e);
      },
      "@pointerup"() {
        this.release();
      },
      "@pointercancel"() {
        this.release();
      },
    },

    track(e) {
      if (e.pointerType === "mouse") return;
      clearTimeout(this.timer);
      // A tapped target focused (group-focus) would keep its tooltip too.
      if (this.$root.contains(document.activeElement)) document.activeElement.blur();
      const targets = this.$root.children;
      const r = this.$root.getBoundingClientRect();
      const i = Math.floor(((e.clientX - r.left) / r.width) * targets.length);
      this.show(Math.min(Math.max(i, 0), targets.length - 1));
    },

    release() {
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.show(-1), 1500);
    },

    show(i) {
      [...this.$root.children].forEach((t, k) => t.classList.toggle("is-active", k === i));
    },
  }));
});

// balanceFix backs a balance typed as the bank shows it (views.balanceStep):
// the amount input's mask (signed, prefixed with the symbol, the metal
// unit's digits) and its difference with the current one. cur is in troy
// ounces for metals; owed (credit cards) colors a rise red.
document.addEventListener("alpine:init", () => {
  Alpine.data("balanceFix", (id, cur, sym, unit = "", owed = false) => ({
    id,
    cur,
    sym,
    unit,
    owed,

    init() {
      if (this.changes) {
        this.$watch("unit", () => this.track());
        this.$el.addEventListener("input", () => this.track());
      }
    },

    // track tells the stepper (fixBalances) around it this one's change.
    track() {
      this.changes[this.id] = this.diff() === 0 ? "" : this.fmtDiff();
    },

    digits() {
      return this.unit === "oz" ? 4 : 2;
    },

    mask(input, money) {
      const prefix = this.sym + " ";
      const s = input.startsWith(prefix) ? input.slice(prefix.length) : input;
      return prefix + money(s.trimStart(), ".", ",", this.digits());
    },

    remask() {
      this.$nextTick(() => {
        const i = this.$root.querySelector("input[inputmode=decimal]");
        if (i?.value) i.dispatchEvent(new Event("input", { bubbles: true }));
      });
    },

    typed() {
      const i = this.$root.querySelector("input[inputmode=decimal]");
      const s = (i?.value ?? "").replace(this.sym, "");
      const n = parseFloat(s.replace(/[^0-9.]/g, ""));
      if (isNaN(n)) return null;
      return /-|−/.test(s) ? -n : n;
    },

    diff() {
      const n = this.typed();
      if (n === null) return 0;
      const cur = this.unit === "g" ? this.cur * 31.1034768 : this.cur;
      const pow = 10 ** this.digits();
      return Math.round((n - cur) * pow) / pow;
    },

    fmtDiff() {
      const d = this.diff();
      const digits = this.digits();
      return (d > 0 ? "+" : "−") + Math.abs(d).toLocaleString(undefined, { minimumFractionDigits: digits, maximumFractionDigits: digits });
    },

    up() {
      return (this.diff() > 0) !== this.owed;
    },
  }));

  // fixBalances steps through the accounts of the fix balances drawer one at
  // a time (views.FixBalancesForm), the list under it showing every change
  // (balanceFix fills changes) and jumping to one.
  Alpine.data("fixBalances", (n) => ({
    k: 0,
    n,
    changes: {},

    changed() {
      return Object.values(this.changes).filter(Boolean).length;
    },

    go(k) {
      this.k = Math.max(0, Math.min(this.n - 1, k));
      this.$nextTick(() => {
        const i = this.$root.querySelector(`[data-step="${this.k}"] input[inputmode=decimal]`);
        i?.focus();
        i?.select();
      });
    },

    // onEnter goes to the next account, submitting on the last one.
    onEnter(e) {
      if (this.k < this.n - 1) {
        e.preventDefault();
        this.go(this.k + 1);
      }
    },
  }));
});
