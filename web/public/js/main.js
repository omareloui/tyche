// entryMenuMove moves the focus to the next (dir 1) or previous (-1) item of
// a components.EntryMenu, wrapping around.
function entryMenuMove(menu, dir) {
  const items = [...menu.querySelectorAll("[role=menuitem]")];
  if (!items.length) return;
  const i = items.indexOf(document.activeElement);
  items[(i + dir + items.length) % items.length].focus();
}
