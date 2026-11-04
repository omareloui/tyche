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
// edit form opened from the details keeps them meanwhile).
const DRAWER_URL_PARAMS = ["cp", "goal"];

function setDrawerParams(name, value) {
  const u = new URL(location.href);
  for (const p of DRAWER_URL_PARAMS) u.searchParams.delete(p);
  if (name) u.searchParams.set(name, value);
  if (u.href !== location.href) history.replaceState(history.state, "", u);
}

// drawerBase is the "drawer" Alpine component, to spread into ones extending
// it (txDetails). It has no getters, as spreading would evaluate them.
function drawerBase() {
  return {
    shown: false,
    // pristine is the form's values when it opened, to tell if they changed.
    pristine: null,
    // opener is what had the focus before opening, given back on closing.
    opener: null,
    // drag is the swipe down closing the sheet on phones, while it lasts.
    drag: null,

    init() {
      if (!this.isStatic()) return this.opened();
      this.$watch("shown", (v) => v && this.opened());
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
      if (root.dataset.urlParam) setDrawerParams(root.dataset.urlParam, root.dataset.urlValue);
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
      this.drag = { sheet, x0: t.clientX, y0: t.clientY, t0: e.timeStamp, dy: 0, on: false };
    },

    // dragMove moves the sheet with the finger once it goes down; going up or
    // sideways first leaves the touch to scrolling.
    dragMove(e) {
      const d = this.drag;
      if (!d) return;
      const t = e.touches[0];
      const dx = t.clientX - d.x0;
      const dy = t.clientY - d.y0;
      if (!d.on) {
        if (dy <= 0 || Math.abs(dx) > dy) {
          this.drag = null;
          return;
        }
        d.on = true;
        d.sheet.style.transition = "none";
        this.drawerRoot().querySelector(".drawer-backdrop").style.transition = "none";
      }
      if (e.cancelable) e.preventDefault();
      d.dy = Math.max(0, dy);
      d.speed = (d.dy - (d.lastDy ?? 0)) / Math.max(1, e.timeStamp - (d.lastT ?? d.t0));
      d.lastDy = d.dy;
      d.lastT = e.timeStamp;
      d.sheet.style.transform = `translateY(${d.dy}px)`;
      const backdrop = this.drawerRoot().querySelector(".drawer-backdrop");
      backdrop.style.opacity = String(Math.max(0, 1 - d.dy / d.sheet.offsetHeight));
    },

    // dragEnd closes the sheet when dragged far or fast enough (asking first
    // if its form changed), else puts it back.
    async dragEnd() {
      const d = this.drag;
      this.drag = null;
      if (!d?.on) return;
      const root = this.drawerRoot();
      const backdrop = root.querySelector(".drawer-backdrop");
      const far = d.dy > d.sheet.offsetHeight / 4 || (d.speed > 0.5 && d.dy > 40);
      if (far && !this.dirty()) {
        // Slides out from where it is instead of the closing animation's start.
        root.setAttribute("data-dragged", "");
        d.sheet.style.transition = "transform 180ms ease-in";
        d.sheet.style.transform = "translateY(100%)";
        this.close();
        return;
      }
      d.sheet.style.transition = "transform 200ms cubic-bezier(0.32, 0.72, 0, 1)";
      d.sheet.style.transform = "";
      backdrop.style.transition = "opacity 200ms";
      backdrop.style.opacity = "";
      if (far) this.requestClose();
    },

    // close slides the drawer out, then hides (kept in the page) or removes
    // it.
    close() {
      const root = this.drawerRoot();
      if (!this.isOpen() || root.hasAttribute("data-closing")) return;
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
          if (sheet) sheet.style.transform = sheet.style.transition = "";
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
