// Receipt scanning: a picked or taken photo (input[data-receipt-input]) is
// resized to at most RECEIPT_MAX px on its long edge as a WebP (fewer tokens
// to read it, a smaller database; a JPEG where the browser can't make one,
// e.g. Safari), put back in its input, then posted by htmx on
// "receipt-ready": the transaction form's inputs post it with the form's
// fields (hx-include), the details' scan form on its own. The server
// converts what it gets the same way (receiptimg), e.g. a HEIC this browser
// couldn't read.
(() => {
  // Same as receiptimg's MaxSide and Quality.
  const RECEIPT_MAX = 1600;
  const QUALITY = 0.75;

  const encode = (canvas, type, quality) =>
    new Promise((done) => canvas.toBlob(done, type, quality));

  async function resize(file) {
    let bitmap;
    try {
      bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
    } catch {
      // Not decodable here (e.g. HEIC outside Safari): sent as is.
      return file;
    }
    const scale = Math.min(1, RECEIPT_MAX / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.type === "image/webp") {
      bitmap.close();
      return file;
    }
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    canvas.getContext("2d").drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close();
    let blob = await encode(canvas, "image/webp", QUALITY);
    // Browsers without a WebP encoder hand a PNG instead.
    if (blob?.type !== "image/webp") blob = await encode(canvas, "image/jpeg", 0.8);
    if (!blob) return file;
    const ext = blob.type === "image/webp" ? "webp" : "jpg";
    return new File([blob], `receipt.${ext}`, { type: blob.type });
  }

  // Lines typed in the form, which a scan replaces.
  function typedLines(input) {
    const items = input.closest("form")?.querySelector("[data-tx-items]");
    return items ? items.querySelectorAll("[data-tx-item]").length : 0;
  }

  document.addEventListener("change", async (e) => {
    const input = e.target;
    if (!(input instanceof HTMLInputElement) || !input.matches("[data-receipt-input]")) return;
    const file = input.files?.[0];
    if (!file) return;

    const scan = input.closest("[data-receipt-scan]");
    const ask = scan?.dataset.receiptConfirm || (typedLines(input) > 0 ? "Replace the lines with the receipt's?" : "");
    if (ask && !window.confirm(ask)) {
      input.value = "";
      return;
    }

    const resized = await resize(file);
    if (resized !== file) {
      const dt = new DataTransfer();
      dt.items.add(resized);
      input.files = dt.files;
    }
    // The other input (camera / gallery) goes empty, so one file is sent.
    scan?.querySelectorAll("[data-receipt-input]").forEach((i) => {
      if (i !== input) i.value = "";
    });
    // The details' form posts itself, the transaction form's input itself.
    htmx.trigger(input.closest("form[hx-trigger='receipt-ready']") ?? input, "receipt-ready");
  });

  // scanInCreateForm hands a file to the transactions page's create form's
  // scan (its gallery input), when scanning is on.
  function scanInCreateForm(file) {
    const input = document.querySelector("#tx-create [data-receipt-input]:not([capture])");
    if (!file || !input) return;
    const dt = new DataTransfer();
    dt.items.add(file);
    input.files = dt.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  // The transactions page's "Scan receipt" (input[data-receipt-quick], which
  // opens the create form) hands its file to the create form's scan.
  document.addEventListener("change", (e) => {
    const quick = e.target;
    if (!(quick instanceof HTMLInputElement) || !quick.matches("[data-receipt-quick]")) return;
    const file = quick.files?.[0];
    quick.value = "";
    scanInCreateForm(file);
  });

  // An image shared to the installed app: the service worker (sw.js) keeps
  // it and opens /transactions?new&shared=receipt, the create form opening
  // by ?new; the image is scanned there, then dropped (a reload doesn't scan
  // it again).
  const SHARED = "tyche-shared";
  const SHARED_URL = "/__shared-receipt";

  async function takeShared() {
    let res;
    try {
      const c = await caches.open(SHARED);
      res = await c.match(SHARED_URL);
      await c.delete(SHARED_URL);
    } catch {
      return;
    }
    if (!res) return;
    const name = decodeURIComponent(res.headers.get("X-File-Name") || "receipt");
    const blob = await res.blob();
    scanInCreateForm(new File([blob], name, { type: blob.type }));
  }

  const url = new URL(location.href);
  if (url.searchParams.get("shared") === "receipt") {
    url.searchParams.delete("shared");
    history.replaceState(history.state, "", url.pathname + url.search.replace(/=(?=&|$)/g, "") + url.hash);
    // After htmx and Alpine set the page up (the form's inputs post by htmx).
    if (document.readyState === "complete") takeShared();
    else window.addEventListener("load", takeShared, { once: true });
  }

  // Once sent, the file isn't posted again with the form.
  document.addEventListener("htmx:afterRequest", (e) => {
    const el = e.detail.elt;
    if (!(el instanceof Element)) return;
    el.querySelectorAll?.("[data-receipt-input]").forEach((i) => (i.value = ""));
    if (el.matches?.("[data-receipt-input]")) el.value = "";
  });

  // Live line totals in the transaction form's items ([data-tx-items]): an
  // item's total follows quantity × unit price, a charge's amount its rate on
  // the items' subtotal, until typed by hand (data-total-manual; cleared, it
  // follows again), like NormalizeTxItems does on saving. The lines' total
  // shows under the rows ([data-items-total]).
  const num = (v) => {
    const n = parseFloat(String(v ?? "").replace(/[^0-9.\-]/g, ""));
    return Number.isFinite(n) ? n : 0;
  };
  const round2 = (v) => Math.round(v * 100) / 100;
  const field = (row, name) => row.querySelector(`[name=${name}]:not([type=hidden])`);

  function recomputeItems(box) {
    let sub = 0;
    const rows = [...box.querySelectorAll("[data-tx-item]")];
    for (const row of rows) {
      if (field(row, "item_rate")) continue;
      const total = field(row, "item_total");
      const price = field(row, "item_unit_price");
      if (total && price && !total.hasAttribute("data-total-manual") && price.value.trim() !== "") {
        const qty = field(row, "item_quantity")?.value.trim();
        total.value = String(round2((qty ? num(qty) : 1) * num(price.value)));
      }
      sub += num(total?.value);
    }
    let sum = sub;
    for (const row of rows) {
      const rate = field(row, "item_rate");
      if (!rate) continue;
      const total = field(row, "item_total");
      if (!total.hasAttribute("data-total-manual") && rate.value.trim() !== "") {
        let v = round2((sub * num(rate.value)) / 100);
        if (row.querySelector("select[name=item_kind]")?.value === "6") v = -Math.abs(v);
        total.value = v ? String(v) : "";
      }
      const included = row.querySelector("input[type=hidden][name=item_included]")?.value === "on";
      if (!included) sum += num(total.value);
    }
    const out = box.querySelector("[data-items-total]");
    if (out) out.textContent = rows.length ? round2(sum).toLocaleString(undefined, { maximumFractionDigits: 2 }) : "–";
  }

  document.addEventListener("input", (e) => {
    const el = e.target;
    const box = el instanceof Element && el.closest("[data-tx-items]");
    if (!box) return;
    if (el.matches("[name=item_total]")) {
      el.toggleAttribute("data-total-manual", el.value.trim() !== "");
    }
    recomputeItems(box);
  });
  // Rows added or removed, the included toggle, a charge's kind.
  document.addEventListener("change", (e) => {
    const box = e.target instanceof Element && e.target.closest("[data-tx-items]");
    if (box) recomputeItems(box);
  });
  document.addEventListener("click", (e) => {
    const box = e.target instanceof Element && e.target.closest("[data-tx-items]");
    if (box) queueMicrotask(() => recomputeItems(box));
  });
  const initItems = (root) => root.querySelectorAll?.("[data-tx-items]").forEach(recomputeItems);
  document.addEventListener("htmx:afterSettle", (e) => initItems(e.detail.elt));
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", () => initItems(document));
  else initItems(document);

  // "Use as the amount": the answer's data-items-amount goes in the form's
  // amount (its mask formats it).
  document.addEventListener("htmx:afterSwap", (e) => {
    const p = e.detail.elt?.querySelector?.("[data-items-amount]");
    const v = p?.dataset.itemsAmount;
    if (!v) return;
    const amount = p.closest("form")?.querySelector("input[name=amount]");
    if (!amount) return;
    amount.value = v;
    amount.dispatchEvent(new Event("input", { bubbles: true }));
  });
})();
