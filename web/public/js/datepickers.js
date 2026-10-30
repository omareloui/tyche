// Attaches a picker to every date input without one yet, so inputs swapped
// in by HTMX (e.g. inline edit forms) get one too.
function loadDatePickers(root = document) {
  const els = root.querySelectorAll(
    "input[data-datepicker]:not([data-datepicker-ready])",
  );
  for (const el of els) {
    new Litepicker({ element: el });
    el.dataset.datepickerReady = "";
  }
}

loadDatePickers();
document.body.addEventListener("htmx:afterSettle", () => loadDatePickers());
