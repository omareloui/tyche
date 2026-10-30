// Attaches a picker to every date input without one yet, so inputs swapped
// in by HTMX (e.g. inline edit forms) get one too.
function loadDatePickers(root = document) {
  const els = root.querySelectorAll(
    "input[data-datepicker]:not([data-datepicker-ready])",
  );
  for (const el of els) {
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

loadDatePickers();
document.body.addEventListener("htmx:afterSettle", () => loadDatePickers());
