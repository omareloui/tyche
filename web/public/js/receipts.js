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

  async function resize(file, maxSide = RECEIPT_MAX, quality = QUALITY) {
    let bitmap;
    try {
      bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
    } catch {
      // Not decodable here (e.g. HEIC outside Safari): sent as is.
      return file;
    }
    const scale = Math.min(1, maxSide / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.type === "image/webp") {
      bitmap.close();
      return file;
    }
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    canvas.getContext("2d").drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close();
    let blob = await encode(canvas, "image/webp", quality);
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

  // An item's picture (input[data-item-image-input], its details): resized
  // before the upload (its short side still over receiptimg.ThumbSide for
  // most photos), the server cropping the small square WebP stored
  // (receiptimg.Thumbnail), then put by htmx on "item-image-ready".
  const ITEM_IMAGE_MAX = 768;

  document.addEventListener("change", async (e) => {
    const input = e.target;
    if (!(input instanceof HTMLInputElement) || !input.matches("[data-item-image-input]")) return;
    const file = input.files?.[0];
    if (!file) return;
    const resized = await resize(file, ITEM_IMAGE_MAX, 0.85);
    if (resized !== file) {
      const dt = new DataTransfer();
      dt.items.add(resized);
      input.files = dt.files;
    }
    htmx.trigger(input, "item-image-ready");
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
        if (row.querySelector("[name=item_kind]")?.value === "6") v = -Math.abs(v);
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
    // A price typed by hand stays when an item is picked (itemSuggest).
    if (e.isTrusted && el instanceof Element && el.matches("[name=item_unit_price], [name=unit_price]")) {
      el.removeAttribute("data-price-picked");
    }
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

  // The details' "Add a line" form (form[data-item-add]): its total follows
  // quantity × unit price (an item) or the rate on the items already there
  // (data-subtotal, a charge) until typed by hand, like the form's rows.
  function recomputeAddForm(f) {
    const charge = f.querySelector("input[name=kind]")?.value !== "0";
    // Alpine disables the other kind's total.
    const total = f.querySelector("[name=total]:not([disabled])");
    if (!total || total.hasAttribute("data-total-manual")) return;
    if (charge) {
      const rate = f.querySelector("[name=rate]")?.value.trim();
      if (!rate) return;
      let v = round2((num(f.dataset.subtotal) * num(rate)) / 100);
      if (f.querySelector("input[name=kind]").value === "6") v = -Math.abs(v);
      total.value = v ? String(v) : "";
      return;
    }
    const price = f.querySelector("[name=unit_price]")?.value.trim();
    if (!price) return;
    const qty = f.querySelector("[name=quantity]")?.value.trim();
    total.value = String(round2((qty ? num(qty) : 1) * num(price)));
  }
  document.addEventListener("input", (e) => {
    const el = e.target;
    const f = el instanceof Element && el.closest("form[data-item-add]");
    if (!f) return;
    if (el.matches("[name=total]")) {
      el.toggleAttribute("data-total-manual", el.value.trim() !== "");
      return;
    }
    recomputeAddForm(f);
  });
  document.addEventListener("change", (e) => {
    const f = e.target instanceof Element && e.target.closest("form[data-item-add]");
    // After Alpine swapped which total is enabled.
    if (f && e.target.matches("input[name=kind]")) setTimeout(() => recomputeAddForm(f));
  });

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

// itemSuggest offers the catalog's items while an item line's name is typed
// (views.itemNameInput): the ones bought at the form's counterparty (its
// counterparty_id field, or the details' add form's data-counterparty)
// first. Picking one puts its id in the hidden input (x-ref id) and the
// unit price it shows (its last, at that counterparty when bought there)
// when none is typed;
// typing clears the id, the server then matching the name (or creating an
// item) when saved.
document.addEventListener("alpine:init", () => {
  Alpine.data("itemSuggest", () => ({
    open: false,
    list: [],
    active: -1,
    timer: null,
    ctrl: null,

    input: {
      ["@input"]() {
        this.$refs.id.value = "";
        this.search();
      },
      ["@keydown.down.prevent"]() {
        if (!this.open) return this.search();
        this.active = (this.active + 1) % this.list.length;
      },
      ["@keydown.up.prevent"]() {
        if (!this.open) return;
        this.active = (this.active - 1 + this.list.length) % this.list.length;
      },
      ["@keydown.enter"](e) {
        if (!this.open || this.active < 0) return;
        e.preventDefault();
        this.pick(this.list[this.active]);
      },
      ["@keydown.escape"](e) {
        if (!this.open) return;
        e.preventDefault();
        e.stopPropagation();
        this.close();
      },
      ["@keydown.tab"]() {
        this.close();
      },
      ["@blur"]() {
        setTimeout(() => this.close(), 150);
      },
    },

    counterparty() {
      const f = this.$el.closest("form");
      return f?.dataset.counterparty || f?.querySelector("[name=counterparty_id]")?.value || "";
    },

    search() {
      clearTimeout(this.timer);
      const q = this.$refs.name.value.trim();
      if (!q) return this.close();
      this.timer = setTimeout(async () => {
        this.ctrl?.abort();
        this.ctrl = new AbortController();
        try {
          const params = new URLSearchParams({ q, cp: this.counterparty() });
          const res = await fetch(`/items/suggest?${params}`, {
            signal: this.ctrl.signal,
            headers: { Accept: "application/json" },
          });
          if (!res.ok) return;
          const list = await res.json();
          if (this.$refs.name.value.trim() !== q) return;
          this.list = Array.isArray(list) ? list : [];
          this.active = -1;
          // Nothing to offer but what's typed already.
          const same = this.list.length === 1 && this.list[0].name.toLowerCase() === q.toLowerCase();
          this.open = this.list.length > 0 && !same;
        } catch {}
      }, 180);
    },

    pick(s) {
      this.$refs.name.value = s.name;
      this.$refs.id.value = s.id;
      const row = this.$el.closest("[data-tx-item], form[data-item-add]");
      const price = row?.querySelector("[name=item_unit_price]:not([type=hidden]), [name=unit_price]");
      // The shown price goes in, unless one was typed by hand (a price an
      // earlier pick put there is replaced).
      if (price && s.last_price && (!price.value.trim() || price.hasAttribute("data-price-picked"))) {
        price.value = String(Math.round(s.last_price * 100) / 100);
        price.setAttribute("data-price-picked", "");
        price.dispatchEvent(new Event("input", { bubbles: true }));
      }
      this.close();
      this.$refs.name.focus();
    },

    close() {
      this.open = false;
      this.active = -1;
    },

    priceLabel(s) {
      const n = s.last_price.toLocaleString(undefined, { maximumFractionDigits: 2 });
      return `${n} ${s.currency}`;
    },
  }));
});
