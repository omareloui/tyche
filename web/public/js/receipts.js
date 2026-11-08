// Receipt scanning: a picked or taken photo (input[data-receipt-input]) is
// resized to at most RECEIPT_MAX px on its long edge as a JPEG (fewer tokens
// to read it, a smaller database), put back in its input, then posted by
// htmx on "receipt-ready": the transaction form's inputs post it with the
// form's fields (hx-include), the details' scan form on its own.
(() => {
  const RECEIPT_MAX = 1600;
  const QUALITY = 0.8;

  async function resize(file) {
    let bitmap;
    try {
      bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
    } catch {
      // Not decodable here (e.g. HEIC outside Safari): sent as is.
      return file;
    }
    const scale = Math.min(1, RECEIPT_MAX / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.type === "image/jpeg" && file.size < 600_000) {
      bitmap.close();
      return file;
    }
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    canvas.getContext("2d").drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close();
    const blob = await new Promise((done) => canvas.toBlob(done, "image/jpeg", QUALITY));
    if (!blob) return file;
    return new File([blob], "receipt.jpg", { type: "image/jpeg" });
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

  // The transactions page's "Scan receipt" (input[data-receipt-quick], which
  // opens the create form) hands its file to the create form's scan.
  document.addEventListener("change", (e) => {
    const quick = e.target;
    if (!(quick instanceof HTMLInputElement) || !quick.matches("[data-receipt-quick]")) return;
    const file = quick.files?.[0];
    quick.value = "";
    const input = document.querySelector("#tx-create [data-receipt-input]:not([capture])");
    if (!file || !input) return;
    const dt = new DataTransfer();
    dt.items.add(file);
    input.files = dt.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });

  // Once sent, the file isn't posted again with the form.
  document.addEventListener("htmx:afterRequest", (e) => {
    const el = e.detail.elt;
    if (!(el instanceof Element)) return;
    el.querySelectorAll?.("[data-receipt-input]").forEach((i) => (i.value = ""));
    if (el.matches?.("[data-receipt-input]")) el.value = "";
  });
})();
