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
    // change event (e.g. for hx-trigger="change").
    if ("datepickerChange" in el.dataset) {
      picker.on("selected", () =>
        el.dispatchEvent(new Event("change", { bubbles: true })),
      );
    }
    el.dataset.datepickerReady = "";
  }
}

// loadRangePicker picks a range into the from and to hidden inputs next to
// el, dispatching change on them. Its reset button clears it.
function loadRangePicker(el) {
  const box = el.parentElement;
  const from = box.querySelector("input[name=from]");
  const to = box.querySelector("input[name=to]");
  const set = (a, b) => {
    from.value = a;
    to.value = b;
    from.dispatchEvent(new Event("change", { bubbles: true }));
  };
  // Two months side by side don't fit a phone.
  const months = window.matchMedia("(min-width: 640px)").matches ? 2 : 1;
  const picker = new Litepicker({
    element: el,
    singleMode: false,
    numberOfMonths: months,
    numberOfColumns: months,
    delimiter: " → ",
    resetButton: true,
    startDate: from.value || undefined,
    endDate: to.value || undefined,
  });
  keepInViewport(picker);
  picker.on("selected", (a, b) => set(a.format("YYYY-MM-DD"), b.format("YYYY-MM-DD")));
  picker.on("clear:selection", () => set("", ""));
  el.dataset.datepickerReady = "";
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
