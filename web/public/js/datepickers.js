function loadDatePickers() {
  const datePickers = [];
  const els = document.querySelectorAll("input[data-datepicker]");
  for (const el of els) {
    const picker = new Litepicker({ element: el });
    datePickers.push(picker);
  }
  return datePickers;
}

function datePickerObserverCallback(_mutationList, observer) {
  initDatePickers();
  observer.disconnect();
}

function datePickersObservers(datePickers) {
  const elements = datePickers.reduce((prev, curr) => {
    const el = curr.options.element.parentElement.parentElement;
    if (prev.find((x) => x === el)) {
      return prev;
    }
    return [...prev, el];
  }, []);

  const config = { childList: true, subtree: true };

  const observer = new MutationObserver(datePickerObserverCallback);
  for (const element of elements) {
    observer.observe(element, config);
  }
}

function initDatePickers() {
  const pickers = loadDatePickers();
  datePickersObservers(pickers);
}

initDatePickers();
