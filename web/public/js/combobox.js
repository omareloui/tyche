// combobox is a custom select with optional search (see form.Combobox). The
// value lives in a hidden input; picking an option dispatches "change" on it
// so hx-trigger="change" keeps working. The panel is position: fixed so it
// isn't clipped by scrolling containers like the create form's body.
//
// With data-create-url, a "Create" row offers adding the searched name when
// no option has that exact label (see ComboboxProps.CreateURL).
//
// Filtering and the active option are applied to the DOM directly (hidden,
// data-active) instead of through Alpine reactivity, so long lists stay fast.
document.addEventListener("alpine:init", () => {
  Alpine.data("combobox", () => {
    let visible = [];
    let active = -1;

    return {
      open: false,
      q: "",
      noMatches: false,
      canCreate: false,
      creating: false,
      panelStyle: "",

      options() {
        return this.$refs.list.querySelectorAll("[data-option]");
      },

      filter() {
        const q = this.q.trim().toLowerCase();
        visible = [];
        let exact = false;
        for (const o of this.options()) {
          const ok = q === "" || o.dataset.search.includes(q);
          o.hidden = !ok;
          if (ok) visible.push(o);
          if (o.dataset.value !== "" && o.dataset.label === q) exact = true;
        }
        const create = this.$refs.list.querySelector("[data-create]");
        this.canCreate = !!create && q !== "" && !exact;
        if (this.canCreate) visible.push(create);
        this.noMatches = visible.length === 0;
        this.setActive(0);
      },

      setActive(i) {
        visible[active]?.removeAttribute("data-active");
        active = i;
        const o = visible[active];
        if (!o) return;
        o.setAttribute("data-active", "");
        o.scrollIntoView({ block: "nearest" });
      },

      move(step) {
        const n = visible.length;
        if (n > 0) this.setActive((active + step + n) % n);
      },

      hover(o) {
        const i = visible.indexOf(o);
        if (i !== active) this.setActive(i);
      },

      toggle() {
        this.open ? this.close() : this.show();
      },

      show() {
        this.q = "";
        this.filter();
        this.place();
        this.open = true;
        const selected = visible.findIndex(
          (o) => o.dataset.value === this.$refs.input.value,
        );
        this.$nextTick(() => {
          (this.$refs.search ?? this.$refs.list).focus();
          this.setActive(Math.max(0, selected));
        });
      },

      close(refocus = false) {
        if (!this.open) return;
        this.open = false;
        if (refocus) this.$refs.button.focus();
      },

      // place anchors the panel under the button, or above it when there's
      // no room below.
      place() {
        const r = this.$refs.button.getBoundingClientRect();
        const width = Math.max(r.width, 240);
        const left = Math.max(8, Math.min(r.left, window.innerWidth - width - 8));
        const below = window.innerHeight - r.bottom;
        const vertical =
          below < 320 && r.top > below
            ? `bottom: ${window.innerHeight - r.top + 6}px`
            : `top: ${r.bottom + 6}px`;
        this.panelStyle = `position: fixed; left: ${left}px; width: ${width}px; ${vertical}`;
      },

      pickActive() {
        if (visible[active]) this.pick(visible[active]);
      },

      pick(o) {
        if ("create" in o.dataset) return this.create();
        const input = this.$refs.input;
        for (const opt of this.options()) {
          opt.setAttribute("aria-selected", opt === o ? "true" : "false");
        }
        this.$refs.display.innerHTML =
          o.querySelector("[data-display]").innerHTML;
        this.$refs.button.dataset.empty = o.dataset.value === "" ? "true" : "false";
        this.close(true);
        if (input.value === o.dataset.value) return;
        input.value = o.dataset.value;
        input.dispatchEvent(new Event("change", { bubbles: true }));
      },

      // create POSTs the search to data-create-url, adds the returned option
      // (or reuses it if the server answered with an existing one) and picks it.
      async create() {
        const name = this.q.trim();
        if (this.creating || name === "") return;
        this.creating = true;
        try {
          const res = await fetch(this.$root.dataset.createUrl, {
            method: "POST",
            body: new URLSearchParams({ name }),
          });
          if (!res.ok) throw new Error(await res.text());
          const tpl = document.createElement("template");
          tpl.innerHTML = (await res.text()).trim();
          const o = tpl.content.firstElementChild;
          const existing = [...this.options()].find(
            (x) => x.dataset.value === o.dataset.value,
          );
          if (existing) return this.pick(existing);
          const next = [...this.options()].find(
            (x) => x.dataset.value !== "" && x.dataset.label > o.dataset.label,
          );
          this.$refs.list.insertBefore(
            o,
            next ?? this.$refs.list.querySelector("[data-create]"),
          );
          this.pick(o);
        } catch (err) {
          console.error("combobox create", err);
          alert(`Couldn't create "${name}": ${err.message}`);
        } finally {
          this.creating = false;
        }
      },

      onEscape(ev) {
        // Only swallow Escape while open, so it still closes the form
        // otherwise.
        if (!this.open) return;
        ev.stopPropagation();
        this.close(true);
      },

      init() {
        this.onViewportChange = (ev) => {
          if (this.open && !this.$refs.panel.contains(ev.target)) this.close();
        };
        window.addEventListener("scroll", this.onViewportChange, true);
        window.addEventListener("resize", this.onViewportChange);
      },

      destroy() {
        window.removeEventListener("scroll", this.onViewportChange, true);
        window.removeEventListener("resize", this.onViewportChange);
      },
    };
  });
});

// Picking a counterparty with a default category (data-default-category on
// its option) in a form whose category is still empty selects that category,
// when the form's category combobox offers it (it allows the tx kind).
document.addEventListener("change", (ev) => {
  const input = ev.target;
  if (!input.matches?.("input[data-fills-category]") || !input.value) return;
  const form = input.form ?? input.closest("form");
  const cat = form?.querySelector('input[name="category_id"]');
  if (!cat || cat.value) return;
  const cp = input
    .closest('[x-data="combobox"]')
    ?.querySelector(`[data-option][data-value="${CSS.escape(input.value)}"]`);
  const catID = cp?.dataset.defaultCategory;
  if (!catID) return;
  const root = cat.closest('[x-data="combobox"]');
  const opt = root?.querySelector(
    `[data-option][data-value="${CSS.escape(catID)}"]`,
  );
  if (opt) Alpine.$data(root).pick(opt);
});
