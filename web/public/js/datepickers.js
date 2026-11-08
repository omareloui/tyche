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
  const set = (a, b) => {
    from.value = a;
    to.value = b;
    from.dispatchEvent(new Event("change", { bubbles: true }));
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

// txDateFilter switches the transactions' date filter between a range and
// one open-ended bound, carrying the day picked over.
document.addEventListener("alpine:init", () => {
  Alpine.data("txDateFilter", (mode = "range") => ({
    mode,

    switchMode() {
      const el = this.$refs.picker;
      const box = el.parentElement;
      const from = box.querySelector("input[name=from]");
      const to = box.querySelector("input[name=to]");
      const was = [from.value, to.value];
      const one = from.value || to.value;
      if (this.mode === "from") [from.value, to.value] = [one, ""];
      else if (this.mode === "to") [from.value, to.value] = ["", one];
      el._picker?.destroy();
      delete el.dataset.datepickerReady;
      el.dataset.datepickerMode = this.mode;
      el.value =
        this.mode === "from" ? from.value
        : this.mode === "to" ? to.value
        : from.value || to.value ? `${from.value} → ${to.value}`
        : "";
      loadDatePickers(box);
      if (from.value !== was[0] || to.value !== was[1]) {
        from.dispatchEvent(new Event("change", { bubbles: true }));
      }
    },
  }));
});

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
