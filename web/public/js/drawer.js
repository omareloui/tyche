// Drawers (components.Drawer): the add and edit forms, and the transaction
// details, in a panel from the right (a bottom sheet on phones). A drawer
// kept in the page (data-drawer-static, e.g. a create form) is shown by the
// page's variable bound to its `shown` (x-modelable); one swapped into
// #drawer is removed on closing. Closing with changes in its form asks
// first; a drawer-saved event from its form (the answer's HX-Trigger)
// closes it right away.

// datepickerOpen reports if a Litepicker is showing, which Escape leaves
// open.
function datepickerOpen() {
  return [...document.querySelectorAll(".litepicker")].some((p) => p.offsetParent !== null);
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
      if (!this.drawerRoot().contains(document.activeElement)) this.opener = document.activeElement;
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

    // close slides the drawer out, then hides (kept in the page) or removes
    // it.
    close() {
      const root = this.drawerRoot();
      if (!this.isOpen() || root.hasAttribute("data-closing")) return;
      const back = this.opener;
      root.setAttribute("data-closing", "");
      let done = false;
      const finish = () => {
        if (done) return;
        done = true;
        if (root.hasAttribute("data-drawer-static")) {
          this.shown = false;
          root.removeAttribute("data-closing");
        } else {
          root.remove();
        }
        if (back?.isConnected) back.focus({ preventScroll: true });
      };
      const sheet = root.querySelector(".drawer-sheet");
      sheet?.addEventListener("animationend", (e) => e.target === sheet && finish());
      setTimeout(finish, 250);
    },
  };
}

document.addEventListener("alpine:init", () => {
  Alpine.data("drawer", drawerBase);
});
