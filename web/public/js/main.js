// entryMenuMove moves the focus to the next (dir 1) or previous (-1) item of
// a components.EntryMenu, wrapping around.
function entryMenuMove(menu, dir) {
  const items = [...menu.querySelectorAll("[role=menuitem]")];
  if (!items.length) return;
  const i = items.indexOf(document.activeElement);
  items[(i + dir + items.length) % items.length].focus();
}

// hx-confirm asks through components.ConfirmDialog instead of the browser's
// confirm().
document.addEventListener("htmx:confirm", (e) => {
  const question = e.detail.question;
  const dialog = document.getElementById("confirm-dialog");
  if (!question || !dialog?.showModal) return;
  e.preventDefault();

  const elt = e.detail.elt;
  const del = e.detail.verb === "delete";
  const i = question.indexOf("? ");
  dialog.querySelector("#confirm-title").textContent = i === -1 ? question : question.slice(0, i + 1);
  dialog.querySelector("#confirm-details").textContent = i === -1 ? "" : question.slice(i + 2);
  dialog.querySelector("#confirm-ok").textContent =
    elt.dataset.confirmLabel || (del ? "Delete" : "Confirm");
  dialog.toggleAttribute("data-danger", del || "confirmDanger" in elt.dataset);
  dialog.toggleAttribute("data-delete", del);

  dialog.returnValue = "";
  dialog.addEventListener(
    "close",
    () => {
      if (dialog.returnValue === "ok") e.detail.issueRequest(true);
    },
    { once: true },
  );
  dialog.showModal();
  dialog.querySelector("#confirm-ok").focus();
});

// Clicking the backdrop (outside the dialog's box) cancels.
document.addEventListener("click", (e) => {
  const dialog = e.target;
  if (dialog.id !== "confirm-dialog" || !dialog.open) return;
  const r = dialog.getBoundingClientRect();
  const inside = e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
  if (!inside) dialog.close("cancel");
});

// Page switches (the navbar's boosted links) show a top bar at once, then
// blur the page with a spinner if slow. The bar and spinner live on <html>,
// out of the swapped <body>.
(() => {
  const root = document.documentElement;
  const bar = document.createElement("div");
  bar.id = "nav-progress";
  bar.setAttribute("aria-hidden", "true");
  const loading = document.createElement("div");
  loading.id = "nav-loading";
  loading.setAttribute("role", "status");
  loading.setAttribute("aria-live", "polite");
  loading.innerHTML = '<span class="nav-spinner" aria-hidden="true"></span><span class="nav-loading-text">Loading</span>';
  root.append(bar, loading);

  let slow;
  const done = () => {
    clearTimeout(slow);
    delete root.dataset.navigating;
    document.querySelectorAll(".nav-pending").forEach((a) => a.classList.remove("nav-pending"));
  };

  document.addEventListener("htmx:beforeRequest", (e) => {
    if (!e.detail.boosted) return;
    done();
    root.dataset.navigating = "";
    slow = setTimeout(() => (root.dataset.navigating = "slow"), 150);
    if (e.detail.elt.tagName === "A") e.detail.elt.classList.add("nav-pending");
  });
  // The request's element is detached by the swap, so its afterRequest
  // never reaches the document: the swap (on <body>) ends it instead.
  for (const ev of ["htmx:afterSwap", "htmx:afterRequest", "htmx:sendError", "htmx:timeout", "htmx:historyRestore"]) {
    document.addEventListener(ev, (e) => {
      if (ev === "htmx:historyRestore" || e.detail.boosted) done();
    });
  }
  addEventListener("pageshow", done);
})();

// components.HoverPopup hangs right-aligned under its group, so on narrow
// screens it can overflow the left edge (or the right one): shift it back in
// when its group is hovered or focused.
(() => {
  const margin = 8;
  const fit = (e) => {
    const group = e.target.closest?.(".group");
    const popup = group?.querySelector(":scope [data-hover-popup]");
    if (!popup || popup.closest(".group") !== group) return;
    popup.style.translate = "";
    // Measured unscaled: it may be mid scale-in, pinned at its top-right.
    const right = popup.getBoundingClientRect().right;
    const left = right - popup.offsetWidth;
    const vw = document.documentElement.clientWidth;
    let dx = 0;
    if (left < margin) dx = margin - left;
    else if (right > vw - margin) dx = vw - margin - right;
    if (dx) popup.style.translate = `${dx}px 0`;
  };
  document.addEventListener("mouseover", fit);
  document.addEventListener("focusin", fit);
})();
