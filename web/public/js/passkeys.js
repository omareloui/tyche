// Passkeys unlocking the encrypted database (fingerprint, face or screen
// lock), with WebAuthn's PRF extension: its output is the secret the
// database key is wrapped with, sent to the server like a password.
//
// - [data-passkey-support] is shown when this device can add one: it has a
//   built-in authenticator verifying the user (fingerprint, face or screen
//   lock, the browser doesn't tell which), [data-passkey-unsupported]
//   otherwise. [data-passkey-support="any"] is shown when the browser can use
//   one at all (also a passkey synced from another device or a phone nearby).
// - The encrypt form ([data-passkey-encrypt]) with its "passkey" box checked
//   adds one right after encrypting, where its answer has [data-passkey-enroll].
// - [data-passkey-add] adds one from the security page, [data-passkey-unlock]
//   unlocks with one.
(() => {
  const hintKey = "passkeyOnDevice";

  const b64 = {
    encode(buf) {
      let s = "";
      for (const b of new Uint8Array(buf)) s += String.fromCharCode(b);
      return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    },
    decode(str) {
      const s = atob(str.replace(/-/g, "+").replace(/_/g, "/"));
      return Uint8Array.from(s, (c) => c.charCodeAt(0)).buffer;
    },
  };

  let support;
  // support resolves to { any, platform }: whether the browser can use
  // passkeys with the PRF extension, and the device has a built-in
  // authenticator verifying the user.
  function checkSupport() {
    support ??= (async () => {
      const none = { any: false, platform: false };
      if (!window.PublicKeyCredential || !navigator.credentials) return none;
      try {
        if (PublicKeyCredential.getClientCapabilities) {
          const caps = await PublicKeyCredential.getClientCapabilities();
          if (caps["extension:prf"] === false) return none;
        }
      } catch {}
      let platform = false;
      try {
        platform = await PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable();
      } catch {}
      return { any: true, platform };
    })();
    return support;
  }

  async function showSupported(root) {
    const s = await checkSupport();
    for (const el of root.querySelectorAll("[data-passkey-support]")) {
      el.hidden = !(el.dataset.passkeySupport === "any" ? s.any : s.platform);
    }
    for (const el of root.querySelectorAll("[data-passkey-unsupported]")) el.hidden = s.platform;
  }

  function setHint() {
    try {
      localStorage.setItem(hintKey, "1");
    } catch {}
  }

  function hasHint() {
    try {
      return localStorage.getItem(hintKey) === "1";
    } catch {
      return false;
    }
  }

  // post sends a same-origin request with the HX-Request header the unsafe
  // routes need, throwing the error message of a failed one.
  async function post(url, body) {
    const headers = { "HX-Request": "true" };
    if (!(body instanceof URLSearchParams)) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(body);
    }
    const res = await fetch(url, { method: "POST", headers, body, credentials: "same-origin" });
    const text = await res.text();
    if (!res.ok) {
      let msg = text;
      try {
        msg = JSON.parse(text).error || text;
      } catch {}
      throw new Error(msg || "Something went wrong, try again.");
    }
    return text;
  }

  function ceremonyError(err) {
    if (err && err.name === "NotAllowedError") return "Cancelled or timed out.";
    if (err && err.name === "InvalidStateError") return "This device already unlocks your data.";
    return (err && err.message) || "Something went wrong, try again.";
  }

  function noPRF() {
    return new Error("This device can't unlock your data with a passkey, use the password.");
  }

  // register adds a passkey of this device, returning the passkeys
  // section's HTML.
  async function register(password) {
    const opts = JSON.parse(await post("/security/passkeys/options", new URLSearchParams({ password })));
    const pk = opts.publicKey;
    const salt = b64.decode(opts.prf[""]);
    pk.challenge = b64.decode(pk.challenge);
    pk.user.id = b64.decode(pk.user.id);
    pk.excludeCredentials = (pk.excludeCredentials || []).map((c) => ({ ...c, id: b64.decode(c.id) }));
    pk.extensions = { ...(pk.extensions || {}), prf: { eval: { first: salt } } };

    const cred = await navigator.credentials.create({ publicKey: pk });
    const ext = cred.getClientExtensionResults();
    if (!ext.prf || ext.prf.enabled === false) throw noPRF();

    // Some authenticators only evaluate the PRF when asserting.
    let out = ext.prf.results && ext.prf.results.first;
    if (!out) {
      const a = await navigator.credentials.get({
        publicKey: {
          challenge: crypto.getRandomValues(new Uint8Array(32)),
          rpId: pk.rp.id,
          allowCredentials: [{ type: "public-key", id: cred.rawId }],
          userVerification: "required",
          extensions: { prf: { eval: { first: salt } } },
        },
      });
      const aext = a.getClientExtensionResults();
      out = aext.prf && aext.prf.results && aext.prf.results.first;
      if (!out) throw noPRF();
    }

    const html = await post("/security/passkeys", {
      ceremony: opts.ceremony,
      prf: b64.encode(out),
      credential: {
        id: cred.id,
        rawId: b64.encode(cred.rawId),
        type: cred.type,
        authenticatorAttachment: cred.authenticatorAttachment,
        response: {
          clientDataJSON: b64.encode(cred.response.clientDataJSON),
          attestationObject: b64.encode(cred.response.attestationObject),
          transports: cred.response.getTransports ? cred.response.getTransports() : [],
        },
        clientExtensionResults: {},
      },
    });
    setHint();
    return html;
  }

  function swapPasskeys(html) {
    const el = document.getElementById("passkeys");
    if (!el) return;
    el.outerHTML = html;
    const fresh = document.getElementById("passkeys");
    if (fresh) {
      htmx.process(fresh);
      showSupported(fresh);
    }
  }

  function status(el, msg, ok) {
    if (!el) return;
    el.textContent = msg;
    el.classList.toggle("text-red-400", !ok);
    el.classList.toggle("text-emerald-400", !!ok);
  }

  // unlock unlocks with a passkey and goes where the server says. auto is
  // without a tap, which some browsers refuse: quietly then.
  async function unlock(btn, auto) {
    const statusEl = btn.parentElement.querySelector("[data-passkey-status]");
    btn.disabled = true;
    status(statusEl, "", true);
    try {
      const opts = JSON.parse(await post("/unlock/passkey/options", new URLSearchParams()));
      const pk = opts.publicKey;
      pk.challenge = b64.decode(pk.challenge);
      pk.allowCredentials = (pk.allowCredentials || []).map((c) => ({ ...c, id: b64.decode(c.id) }));
      const evalByCredential = {};
      for (const [id, salt] of Object.entries(opts.prf || {})) evalByCredential[id] = { first: b64.decode(salt) };
      pk.extensions = { ...(pk.extensions || {}), prf: { evalByCredential } };

      const a = await navigator.credentials.get({ publicKey: pk });
      const ext = a.getClientExtensionResults();
      const out = ext.prf && ext.prf.results && ext.prf.results.first;
      if (!out) throw noPRF();

      const res = JSON.parse(
        await post("/unlock/passkey", {
          ceremony: opts.ceremony,
          prf: b64.encode(out),
          next: btn.dataset.next || "/",
          credential: {
            id: a.id,
            rawId: b64.encode(a.rawId),
            type: a.type,
            authenticatorAttachment: a.authenticatorAttachment,
            response: {
              clientDataJSON: b64.encode(a.response.clientDataJSON),
              authenticatorData: b64.encode(a.response.authenticatorData),
              signature: b64.encode(a.response.signature),
              userHandle: a.response.userHandle ? b64.encode(a.response.userHandle) : null,
            },
            clientExtensionResults: {},
          },
        }),
      );
      setHint();
      if (res.app) {
        // The app's deep link may need a tap when opened without one.
        statusEl.textContent = "";
        const link = document.createElement("a");
        link.href = res.redirect;
        link.className = "btn-primary w-full";
        link.textContent = "Back to the app";
        statusEl.replaceWith(link);
        btn.hidden = true;
      }
      window.location.assign(res.redirect);
    } catch (err) {
      if (!(auto && err && err.name === "NotAllowedError")) status(statusEl, ceremonyError(err), false);
      btn.disabled = false;
    }
  }

  // The encrypt form's password, kept until its answer adds the passkey.
  let encryptPassword = null;

  document.addEventListener("htmx:configRequest", (e) => {
    const form = e.target.closest && e.target.closest("form[data-passkey-encrypt]");
    if (!form) return;
    const box = form.querySelector("[data-passkey-support] input[name=passkey]");
    const wanted = box && box.checked && !box.closest("[data-passkey-support]").hidden;
    encryptPassword = wanted ? form.querySelector("input[name=password]").value : null;
  });

  async function enroll(el) {
    const password = encryptPassword;
    encryptPassword = null;
    if (!password) return;
    status(el, "Adding your passkey…", true);
    el.classList.remove("text-emerald-400");
    el.classList.add("text-zinc-400");
    try {
      swapPasskeys(await register(password));
      el.classList.remove("text-zinc-400");
      status(el, "Passkey added: unlock with your fingerprint, face or screen lock too.", true);
    } catch (err) {
      el.classList.remove("text-zinc-400");
      status(el, `The passkey wasn't added (${ceremonyError(err)}). Your password works, add one later from Security.`, false);
    }
  }

  document.addEventListener("submit", async (e) => {
    const form = e.target.closest("form[data-passkey-add]");
    if (!form) return;
    e.preventDefault();
    const btn = form.querySelector("button[type=submit]");
    const statusEl = form.querySelector("[data-passkey-status]");
    btn.disabled = true;
    status(statusEl, "", true);
    try {
      swapPasskeys(await register(form.querySelector("input[name=password]").value));
    } catch (err) {
      status(statusEl, ceremonyError(err), false);
      btn.disabled = false;
    }
  });

  document.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-passkey-unlock]");
    if (btn) unlock(btn);
  });

  async function init(root) {
    await showSupported(root);
    for (const el of root.querySelectorAll("[data-passkey-enroll]")) enroll(el);
    // Unlocking on a device that has one: no need to tap first.
    const btn = root.querySelector("[data-passkey-unlock]");
    if (btn && hasHint() && !btn.closest("[data-passkey-support]").hidden && !btn.dataset.autoTried) {
      btn.dataset.autoTried = "1";
      unlock(btn, true);
    }
  }

  // htmx:load fires for the page and every swapped in element.
  document.addEventListener("htmx:load", (e) => init(e.target));
})();
