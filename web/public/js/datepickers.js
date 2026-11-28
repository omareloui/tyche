// Attaches a picker to every date input without one yet, so inputs swapped
// in by HTMX (e.g. inline edit forms) get one too.
function loadDatePickers(root = document) {
  const els = root.querySelectorAll(
    "input[data-datepicker]:not([data-datepicker-ready])",
  );
  for (const el of els) {
    if ("datepickerRange" in el.dataset) {
      loadRangePicker(el);
      continue;
    }
    const picker = new Litepicker({ element: el });
    keepInViewport(picker);
    // Litepicker sets the value silently, data-datepicker-change opts into a
    // change event (e.g. for hx-trigger="change"). Litepicker reads the input
    // again on change and selects it, so the change it sends (or one sent by
    // a script setting the value) mustn't send another.
    if ("datepickerChange" in el.dataset) {
      let sent = el.value;
      el.addEventListener("change", () => (sent = el.value));
      picker.on("selected", () => {
        if (el.value === sent) return;
        sent = el.value;
        el.dispatchEvent(new Event("change", { bubbles: true }));
      });
    }
    el.dataset.datepickerReady = "";
  }
}

// loadRangePicker picks the days of the from and to hidden inputs next to
// el, dispatching change on them: a range, or by data-datepicker-mode ("from"
// / "to") one day into that input alone, the other one emptied. Its reset
// button clears it. The picker is kept as el._picker, to rebuild it in
// another mode (txDateFilter).
function loadRangePicker(el) {
  const box = el.parentElement;
  const from = box.querySelector("input[name=from]");
  const to = box.querySelector("input[name=to]");
  const mode = el.dataset.datepickerMode || "range";
  // A pick (not a mode switch) also tells the popover holding it to close
  // (txDateFilter's datepicker-picked).
  const set = (a, b) => {
    from.value = a;
    to.value = b;
    from.dispatchEvent(new Event("change", { bubbles: true }));
    el.dispatchEvent(new CustomEvent("datepicker-picked", { bubbles: true }));
  };
  const day = (d) => d.format("YYYY-MM-DD");
  let picker;
  if (mode === "range") {
    // Two months side by side don't fit a phone.
    const months = window.matchMedia("(min-width: 640px)").matches ? 2 : 1;
    picker = new Litepicker({
      element: el,
      singleMode: false,
      numberOfMonths: months,
      numberOfColumns: months,
      delimiter: " → ",
      resetButton: true,
      startDate: from.value || undefined,
      endDate: to.value || undefined,
    });
    picker.on("selected", (a, b) => set(day(a), day(b)));
  } else {
    const input = mode === "from" ? from : to;
    picker = new Litepicker({
      element: el,
      resetButton: true,
      startDate: input.value || undefined,
    });
    picker.on("selected", (d) => (mode === "from" ? set(day(d), "") : set("", day(d))));
  }
  keepInViewport(picker);
  picker.on("clear:selection", () => set("", ""));
  el._picker = picker;
  el.dataset.datepickerReady = "";
}

// txDateFilter is the transactions' dates pill (views.txDateFilter): a
// preset, or custom days, a range or one open-ended bound (switching carries
// the day picked over), named on the pill.
document.addEventListener("alpine:init", () => {
  Alpine.data("txDateFilter", (mode = "range") => ({
    mode,
    open: false,
    label: "",
    set: false,

    init() {
      this.syncLabel();
    },

    inputs() {
      const box = this.$refs.picker.parentElement;
      return [box.querySelector("input[name=from]"), box.querySelector("input[name=to]")];
    },

    isPreset(from, to) {
      const [f, t] = this.inputs();
      return f.value === from && t.value === to;
    },

    // syncLabel names the days: a preset's name, else the days themselves.
    syncLabel() {
      const [f, t] = this.inputs();
      this.set = !!(f.value || t.value);
      const preset = [...this.$root.querySelectorAll("[data-preset]")].find(
        (b) => b.dataset.from === f.value && b.dataset.to === t.value,
      );
      this.label = preset ? preset.dataset.preset : txRangeLabel(f.value, t.value);
    },

    pickPreset(btn) {
      const [f, t] = this.inputs();
      this.open = false;
      if (f.value === btn.dataset.from && t.value === btn.dataset.to) return;
      f.value = btn.dataset.from;
      t.value = btn.dataset.to;
      this.mode = "range";
      this.rebuild();
      f.dispatchEvent(new Event("change", { bubbles: true }));
    },

    switchMode() {
      const [from, to] = this.inputs();
      const was = [from.value, to.value];
      const one = from.value || to.value;
      if (this.mode === "from") [from.value, to.value] = [one, ""];
      else if (this.mode === "to") [from.value, to.value] = ["", one];
      this.rebuild();
      if (from.value !== was[0] || to.value !== was[1]) {
        from.dispatchEvent(new Event("change", { bubbles: true }));
      }
    },

    // rebuild makes the picker again for the mode and the days.
    rebuild() {
      const el = this.$refs.picker;
      const [from, to] = this.inputs();
      el._picker?.destroy();
      delete el.dataset.datepickerReady;
      el.dataset.datepickerMode = this.mode;
      el.value =
        this.mode === "from" ? from.value
        : this.mode === "to" ? to.value
        : from.value || to.value ? `${from.value} → ${to.value}`
        : "";
      loadDatePickers(el.parentElement);
    },
  }));
});

// txRangeLabel names a dates filter's days like views.FormatTxRange.
function txRangeLabel(from, to) {
  const day = (s) => {
    const d = new Date(s + "T00:00:00");
    const opts = { month: "short", day: "numeric" };
    if (d.getFullYear() !== new Date().getFullYear()) opts.year = "numeric";
    return d.toLocaleDateString("en-US", opts);
  };
  if (!from && !to) return "Any time";
  if (!to) return "From " + day(from);
  if (!from) return "Until " + day(to);
  if (from === to) return day(from);
  return day(from) + " → " + day(to);
}

// keepInViewport moves the shown picker back inside the viewport:
// Litepicker keeps it aligned with the input's left edge when it fits neither
// aligned left nor right, overflowing narrow screens.
function keepInViewport(picker) {
  picker.on("show", () => {
    const ui = picker.ui;
    const margin = 8;
    const rect = ui.getBoundingClientRect();
    const maxLeft = document.documentElement.clientWidth - rect.width - margin;
    const left = Math.max(margin, Math.min(rect.left, maxLeft));
    if (left !== rect.left) {
      ui.style.left = left + window.scrollX + "px";
    }
  });
}

loadDatePickers();
document.body.addEventListener("htmx:afterSettle", () => loadDatePickers());
