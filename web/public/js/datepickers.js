function loadDatePickers() {
  const datePickers = [];
  const els = document.querySelectorAll("input[data-datepicker]");
  for (const el of els) {
    const picker = new Litepicker({ element: el });
    datePickers.push(picker);
  }
  return datePickers;
}

loadDatePickers();
