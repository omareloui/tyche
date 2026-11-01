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
