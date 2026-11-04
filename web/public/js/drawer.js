// Drawers (components.Drawer): the add and edit forms, and the transaction
// details, in a panel from the right (a bottom sheet on phones). A drawer
// kept in the page (data-drawer-static, e.g. a create form) is shown by the
// page's variable bound to its `shown` (x-modelable); one swapped into
// #drawer is removed on closing. Closing with changes in its form asks
// first; a drawer-saved event from its form (the answer's HX-Trigger)
// closes it right away. On phones the sheet also closes by swiping it down.

// datepickerOpen reports if a Litepicker is showing, which Escape leaves
// open.
function datepickerOpen() {
  return [...document.querySelectorAll(".litepicker")].some((p) => p.offsetParent !== null);
}

// standIn is el, or what replaced it when it got re-rendered meanwhile (e.g.
// the list a drawer was opened from, re-rendered on saving): the element of
// its id, or the same kind of element in its closest one with an id.
function standIn(el) {
  if (!el || el.isConnected) return el;
  if (el.id) return document.getElementById(el.id);
  const holder = el.closest("[id]");
  const fresh = holder && document.getElementById(holder.id);
  if (!fresh) return null;
  const attrs = [...el.attributes].filter((a) => a.name.startsWith("data-")).map((a) => `[${a.name}]`);
  return (attrs.length && fresh.querySelector(el.tagName.toLowerCase() + attrs.join(""))) || fresh;
}

// DRAWER_URL_PARAMS are the query params of the details drawers
// (components.DrawerProps.URLParam), kept in the url while one is open so
// reloading the page opens it again; closing a fetched drawer drops them (an
// edit form opened from the details keeps them meanwhile). Each gives the url
// of its details, where Back goes from a form opened from them.
const DRAWER_URL_PARAMS = {
  cp: (id) => `/counterparties/${encodeURIComponent(id)}/details`,
  goal: (id) => `/goals/${encodeURIComponent(id)}/details`,
  acc: (id) => `/accounts/${encodeURIComponent(id)}/details`,
  rtx: (id) => `/recurrent-tx/${encodeURIComponent(id)}/details`,
};

// DETAILS_URL_PARAMS also has the transaction details' ?tx= (transactions.js).
const DETAILS_URL_PARAMS = [...Object.keys(DRAWER_URL_PARAMS), "tx"];

// FORM_PARAM is the url (a path) an open form drawer was fetched from, kept
// in the page's url so reloading opens it again; NEW_PARAM is the page's
// create form open (layouts.WithCreateForm, createFormFromURL).
const FORM_PARAM = "form";
const NEW_PARAM = "new";
const DRAWER_STATE_PARAMS = [...DETAILS_URL_PARAMS, FORM_PARAM, NEW_PARAM];

// formPath is a FORM_PARAM's value if it's a path of this site, else null.
function formPath(v) {
  return v && /^\/(?![/\\])/.test(v) ? v : null;
}

function setDrawerParams(name, value) {
  const u = new URL(location.href);
  for (const p of Object.keys(DRAWER_URL_PARAMS)) u.searchParams.delete(p);
  if (name) u.searchParams.set(name, value);
  if (u.href !== location.href) history.replaceState(history.state, "", u);
}

// drawerHistory gives every open drawer a history entry, so the browser's
// Back closes it instead of leaving the page, or goes back to the details a
// form was opened from. Its stack has an entry per pushed one: {el, kind,
// back, param, value, parent, form}, kind "plain" (a form), "details" (param
// and value are its url param) or "child" (a form that replaced details in
// #drawer, parent being their url), back(entry) what Back does, form its
// FORM_PARAM (or NEW_PARAM). Closing a drawer otherwise goes back past its
// entries itself.
const drawerHistory = {
  stack: [],
  // expect is the depth our own history.go lands on, whose popstate isn't a
  // Back; queue holds the pushes waiting for it.
  expect: null,
  queue: [],
  // page tells this page's entries from the ones of a page left since
  // (boosted navigations), which htmx restores.
  page: Math.random().toString(36).slice(2),
  // resume is the depth of the entry the page got reloaded on, which the
  // drawers opening again from its url take over instead of pushing theirs.
  resume: 0,
  // pendingForm is the form to open again on load, once the details it was
  // opened from (if any) are.
  pendingForm: null,
  // fetched is the url of the drawer being swapped into #drawer.
  fetched: null,

  top() {
    return this.stack[this.stack.length - 1];
  },

  // load reopens the form the url has, and makes the drawers opening from it
  // take over the entries the page got reloaded on.
  load() {
    const u = new URL(location.href);
    const form = formPath(u.searchParams.get(FORM_PARAM));
    const details = DETAILS_URL_PARAMS.some((p) => u.searchParams.has(p));
    const d = history.state?.drawer ?? 0;
    if (d > 0) {
      const n = Number(details) + Number(!!form || u.searchParams.has(NEW_PARAM));
      for (let i = n; i < d; i++) this.stack.push({ el: null, kind: "plain" });
      this.resume = d;
    }
    if (!form) return;
    if (details) this.pendingForm = form;
    else htmx.ajax("GET", form, { target: "#" + "drawer", swap: "innerHTML" });
  },

  entryURL(entry) {
    const u = new URL(location.href);
    if (entry.param || this.stack.length === 1) for (const p of DRAWER_STATE_PARAMS) u.searchParams.delete(p);
    if (entry.param) u.searchParams.set(entry.param, entry.value);
    if (entry.form === NEW_PARAM) u.searchParams.set(NEW_PARAM, "");
    else if (entry.form) u.searchParams.set(FORM_PARAM, entry.form);
    return u;
  },

  push(entry) {
    this.stack.push(entry);
    const state = { htmx: true, drawer: this.stack.length, page: this.page };
    if (this.resume) {
      // Reloaded on this entry (or one over it): it's there already.
      if (this.stack.length >= this.resume) {
        this.resume = 0;
        history.replaceState(state, "", this.entryURL(entry));
      }
      return;
    }
    const go = () => {
      const u = this.entryURL(entry);
      if (entry.param || this.stack.length === 1) {
        // Going back lands on the url without the drawers.
        const base = new URL(location.href);
        for (const p of DRAWER_STATE_PARAMS) base.searchParams.delete(p);
        history.replaceState(history.state, "", base);
      }
      history.pushState(state, "", u);
    };
    if (this.expect === null) go();
    else this.queue.push(go);
  },

  // drop goes back past the top n entries.
  drop(n) {
    n = Math.min(n, this.stack.length);
    if (n <= 0) return;
    this.stack.length -= n;
    this.expect = this.stack.length;
    history.go(-n);
  },

  // opened registers drawer el opening, param and value being its url param
  // if it's details, form its FORM_PARAM if it's a form. One replacing a
  // drawer in place (in #drawer, or the transaction details) takes over its
  // entry; a form replacing details gets its own (Back goes back to them);
  // details replacing that form (canceling or saving it) go back to the
  // details' entry.
  opened(el, back, param = null, value = null, form = null) {
    this.register(el, back, param, value, form);
    if (param && this.pendingForm) {
      // The transaction details swap their edit form in their panel.
      const target = param === "tx" ? "#txd-panel" : "#drawer";
      htmx.ajax("GET", this.pendingForm, { target, swap: "innerHTML" });
      this.pendingForm = null;
    }
  },

  register(el, back, param, value, form) {
    const kind = param ? "details" : "plain";
    let top = this.top();
    if (top?.el && !top.el.isConnected) {
      if (kind === "details" && top.kind === "child") {
        this.drop(1);
        top = this.top();
      }
      if (top) {
        const parent = top.kind === "details" && DRAWER_URL_PARAMS[top.param];
        if (kind === "plain" && parent) return this.push({ el, kind: "child", back, parent: parent(top.value), form });
        if (top.kind === "child") Object.assign(top, { el, back, form });
        else Object.assign(top, { el, kind, back, param, value, form });
        if (form && this.expect === null) history.replaceState(history.state, "", this.entryURL(top));
        return;
      }
    }
    this.push({ el, kind, back, param, value, form });
  },

  // closed goes back past the entries of drawer el, closed, and the ones it
  // replaced.
  closed(el) {
    if (this.top()?.el !== el) return;
    let i = this.stack.length;
    while (i > 0 && (this.stack[i - 1].el === el || !this.stack[i - 1].el?.isConnected)) i--;
    this.drop(this.stack.length - i);
  },

  reset() {
    this.stack = [];
    this.expect = null;
    this.queue = [];
    this.resume = 0;
    this.pendingForm = null;
    this.page = Math.random().toString(36).slice(2);
  },

  // onPopState runs before htmx's window.onpopstate (capturing), stopping it
  // when the entry is a drawer's of this page.
  onPopState(e) {
    const d = e.state?.drawer ?? 0;
    this.resume = 0;
    if (this.expect !== null) {
      const ours = d === this.expect;
      this.expect = null;
      const queue = this.queue;
      this.queue = [];
      if (ours) {
        e.stopImmediatePropagation();
        for (const go of queue) go();
        return;
      }
    }
    if (d < this.stack.length) {
      e.stopImmediatePropagation();
      const top = this.top();
      this.stack.length = d;
      top.back?.(top);
    } else if (d > this.stack.length && e.state?.page === this.page) {
      // Forward to a drawer closed since: back where it was.
      e.stopImmediatePropagation();
      this.expect = this.stack.length;
      history.go(this.stack.length - d);
    }
  },
};

window.addEventListener("popstate", (e) => drawerHistory.onPopState(e), true);
// Another page (its drawers gone with the old body).
for (const ev of ["htmx:pushedIntoHistory", "htmx:historyRestore", "htmx:historyCacheMissLoad"])
  document.addEventListener(ev, () => drawerHistory.reset());
// The url a drawer swapped into #drawer was fetched from, for its FORM_PARAM.
document.addEventListener("htmx:beforeSwap", (e) => {
  if (e.detail.target?.id !== "drawer") return;
  const get = e.detail.requestConfig?.verb === "get";
  drawerHistory.fetched = get ? e.detail.pathInfo?.finalRequestPath || e.detail.pathInfo?.requestPath : null;
});
// Before Alpine starts (deferred, after this), as its drawers register.
drawerHistory.load();

// drawerBase is the "drawer" Alpine component, to spread into ones extending
// it (txDetails). It has no getters, as spreading would evaluate them.
function drawerBase() {
  return {
    shown: false,
    // pristine is the form's values when it opened, to tell if they changed.
    pristine: null,
    // opener is what had the focus before opening, given back on closing.
    opener: null,
    // drag is the swipe on the sheet on phones, while it lasts: down closes
    // it, up fills the screen with it (data-expanded).
    drag: null,

    init() {
      if (!this.isStatic()) return this.opened();
      // Hidden by its variable rather than closing.
      this.$watch("shown", (v) => (v ? this.opened() : drawerHistory.closed(this.drawerRoot())));
      if (this.shown) this.opened();
    },

    // drawerRoot is the drawer's element. Not $root, which is the closest
    // x-data: called from a form with its own (e.g. a category's accent
    // color), that's the form.
    drawerRoot() {
      return this.$el.closest?.("[data-drawer]") ?? this.$root;
    },

    isStatic() {
      return this.drawerRoot().hasAttribute("data-drawer-static");
    },

    isOpen() {
      return !this.isStatic() || this.shown;
    },

    opened() {
      const root = this.drawerRoot();
      const { urlParam, urlValue } = root.dataset;
      if (urlParam) setDrawerParams(urlParam, urlValue);
      const fetched = drawerHistory.fetched;
      drawerHistory.fetched = null;
      let form = null;
      if (this.isStatic()) form = root.getAttribute("x-model") === "openCreateForm" ? NEW_PARAM : null;
      else if (!urlParam) form = formPath(fetched);
      drawerHistory.opened(root, (e) => this.back(e), urlParam || null, urlValue, form);
      if (!root.contains(document.activeElement)) this.opener = document.activeElement;
      this.$nextTick(() => {
        this.pristine = this.formState();
        this.focusFirst();
      });
    },

    // focusFirst focuses the form's first field (not on touch screens, where
    // it'd pop the keyboard up), else the close button.
    focusFirst() {
      const field =
        matchMedia("(pointer: fine)").matches &&
        this.form()?.querySelector(
          "input:not([type=hidden]):not([disabled]), select, textarea, [aria-haspopup=listbox]",
        );
      (field || this.drawerRoot().querySelector("[x-ref=close]"))?.focus({ preventScroll: true });
    },

    form() {
      return this.drawerRoot().querySelector("form[data-drawer-form]");
    },

    formState() {
      const form = this.form();
      return form ? new URLSearchParams(new FormData(form)).toString() : null;
    },

    dirty() {
      const s = this.formState();
      return s !== null && s !== this.pristine;
    },

    // confirmDiscard resolves to whether the form's changes may be lost.
    async confirmDiscard() {
      if (!this.dirty()) return true;
      return askConfirm("Discard your changes? What you entered won't be saved.", {
        label: "Discard",
        danger: true,
      });
    },

    async requestClose() {
      if (await this.confirmDiscard()) this.close();
    },

    // back is the browser's Back (drawerHistory), its entry already gone:
    // back to the details the form was opened from, else closing; kept
    // open (its entry back) when its changes aren't to be discarded.
    async back(entry) {
      if (!(await this.confirmDiscard())) return drawerHistory.push(entry);
      if (entry.parent) htmx.ajax("GET", entry.parent, { target: "#drawer", swap: "innerHTML" });
      else this.close(true);
    },

    // escape (on keyup, after an open combobox stopped it for closing itself)
    // closes, unless it closed the confirm dialog or the datepicker is open.
    escape(e) {
      if (!this.isOpen() || e.defaultPrevented || confirmJustClosed() || datepickerOpen()) return;
      this.requestClose();
    },

    backdrop() {
      this.requestClose();
    },

    saved() {
      this.close();
    },

    // dragStart begins following a touch on the sheet, on phones (a bottom
    // sheet), unless it's on a scrolled area (pulling it down scrolls it
    // back), a textarea or an open listbox.
    dragStart(e) {
      this.drag = null;
      const root = this.drawerRoot();
      if (matchMedia("(min-width: 40rem)").matches || root.hasAttribute("data-closing")) return;
      if (e.touches.length !== 1 || e.target.closest("textarea, [role=listbox]")) return;
      const sheet = root.querySelector(".drawer-sheet");
      for (let el = e.target; el && el !== sheet; el = el.parentElement) if (el.scrollTop > 0) return;
      const t = e.touches[0];
      this.drag = {
        sheet,
        x0: t.clientX,
        y0: t.clientY,
        t0: e.timeStamp,
        h0: sheet.offsetHeight,
        expanded: root.hasAttribute("data-expanded"),
        dy: 0,
        dir: null,
      };
    },

    // dragMove moves the sheet with the finger once it goes down, or grows it
    // when it goes up (unless it fills the screen already); going sideways
    // first, or up on a full screen sheet, leaves the touch to scrolling.
    dragMove(e) {
      const d = this.drag;
      if (!d) return;
      const t = e.touches[0];
      const dx = t.clientX - d.x0;
      const dy = t.clientY - d.y0;
      if (!d.dir) {
        if (Math.abs(dx) > Math.abs(dy) || dy === 0 || (dy < 0 && d.expanded)) {
          this.drag = null;
          return;
        }
        d.dir = dy > 0 ? "down" : "up";
        d.sheet.style.transition = "none";
        this.drawerRoot().querySelector(".drawer-backdrop").style.transition = "none";
      }
      if (e.cancelable) e.preventDefault();
      d.dy = d.dir === "down" ? Math.max(0, dy) : Math.min(0, dy);
      d.speed = (d.dy - (d.lastDy ?? 0)) / Math.max(1, e.timeStamp - (d.lastT ?? d.t0));
      d.lastDy = d.dy;
      d.lastT = e.timeStamp;
      if (d.dir === "up") {
        d.sheet.style.maxHeight = "none";
        d.sheet.style.height = `${Math.min(innerHeight, d.h0 - d.dy)}px`;
        return;
      }
      d.sheet.style.transform = `translateY(${d.dy}px)`;
      const backdrop = this.drawerRoot().querySelector(".drawer-backdrop");
      backdrop.style.opacity = String(Math.max(0, 1 - d.dy / d.sheet.offsetHeight));
    },

    // dragEnd fills the screen with the sheet when dragged up far or fast
    // enough; down, closes it when dragged far or fast enough (asking first
    // if its form changed), shrinks a full screen one back, else puts it back.
    async dragEnd() {
      const d = this.drag;
      this.drag = null;
      if (!d?.dir) return;
      const root = this.drawerRoot();
      const backdrop = root.querySelector(".drawer-backdrop");
      if (d.dir === "up") {
        if (-d.dy > 40 || d.speed < -0.5) this.resize(d.sheet, innerHeight, () => root.setAttribute("data-expanded", ""));
        else this.resize(d.sheet, d.h0);
        return;
      }
      const h = d.sheet.offsetHeight;
      const fast = d.speed > 0.5 && d.dy > 40;
      const far = d.expanded ? d.dy > h / 2 || (fast && d.dy > h / 4) : d.dy > h / 4 || fast;
      if (far && !this.dirty()) {
        // Slides out from where it is instead of the closing animation's start.
        root.setAttribute("data-dragged", "");
        d.sheet.style.transition = "transform 180ms ease-in";
        d.sheet.style.transform = "translateY(100%)";
        this.close();
        return;
      }
      backdrop.style.transition = "opacity 200ms";
      backdrop.style.opacity = "";
      if (d.expanded && (d.dy > h / 4 || fast)) {
        this.collapse(d.sheet);
        if (far) this.requestClose();
        return;
      }
      d.sheet.style.transition = "transform 200ms cubic-bezier(0.32, 0.72, 0, 1)";
      d.sheet.style.transform = "";
      if (far) this.requestClose();
    },

    // resize animates the sheet's height from where it is to px, then leaves
    // it to its css again, after calling done (e.g. setting data-expanded).
    resize(sheet, px, done) {
      sheet.style.maxHeight = "none";
      sheet.style.height = `${sheet.offsetHeight}px`;
      sheet.offsetHeight; // Starts the transition from there.
      sheet.style.transition = "height 220ms cubic-bezier(0.32, 0.72, 0, 1), transform 220ms cubic-bezier(0.32, 0.72, 0, 1)";
      sheet.style.height = `${px}px`;
      sheet.style.transform = "";
      const end = (e) => {
        if (e && (e.target !== sheet || e.propertyName !== "height")) return;
        sheet.removeEventListener("transitionend", end);
        clearTimeout(timer);
        done?.();
        sheet.style.height = sheet.style.maxHeight = sheet.style.transition = "";
      };
      sheet.addEventListener("transitionend", end);
      const timer = setTimeout(end, 260);
    },

    // collapse shrinks a full screen sheet back to its own height, from where
    // it was dragged to.
    collapse(sheet) {
      const top = sheet.getBoundingClientRect().top;
      this.drawerRoot().removeAttribute("data-expanded");
      sheet.style.transition = "none";
      sheet.style.height = sheet.style.maxHeight = sheet.style.transform = "";
      const natural = sheet.offsetHeight;
      sheet.style.maxHeight = "none";
      sheet.style.height = `${innerHeight - top}px`;
      this.resize(sheet, natural);
    },

    // close slides the drawer out, then hides (kept in the page) or removes
    // it; fromHistory when Back closes it, its entry gone already.
    close(fromHistory = false) {
      const root = this.drawerRoot();
      if (!this.isOpen() || root.hasAttribute("data-closing")) return;
      if (!fromHistory) drawerHistory.closed(root);
      const back = this.opener;
      if (!root.hasAttribute("data-drawer-static")) setDrawerParams(null);
      root.setAttribute("data-closing", "");
      const sheet = root.querySelector(".drawer-sheet");
      let done = false;
      const finish = () => {
        if (done) return;
        done = true;
        if (root.hasAttribute("data-drawer-static")) {
          this.shown = false;
          root.removeAttribute("data-closing");
          root.removeAttribute("data-dragged");
          root.removeAttribute("data-expanded");
          if (sheet) sheet.style.transform = sheet.style.transition = sheet.style.height = sheet.style.maxHeight = "";
          const backdrop = root.querySelector(".drawer-backdrop");
          backdrop.style.opacity = backdrop.style.transition = "";
        } else {
          root.remove();
        }
        standIn(back)?.focus({ preventScroll: true });
      };
      sheet?.addEventListener("animationend", (e) => e.target === sheet && finish());
      setTimeout(finish, 250);
    },
  };
}

document.addEventListener("alpine:init", () => {
  Alpine.data("drawer", drawerBase);
});
