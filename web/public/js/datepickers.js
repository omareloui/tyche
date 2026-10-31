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
  const picker = new Litepicker({
    element: el,
    singleMode: false,
    numberOfMonths: 2,
    numberOfColumns: 2,
    delimiter: " → ",
    resetButton: true,
    startDate: from.value || undefined,
    endDate: to.value || undefined,
  });
  picker.on("selected", (a, b) => set(a.format("YYYY-MM-DD"), b.format("YYYY-MM-DD")));
  picker.on("clear:selection", () => set("", ""));
  el.dataset.datepickerReady = "";
}

loadDatePickers();
document.body.addEventListener("htmx:afterSettle", () => loadDatePickers());
