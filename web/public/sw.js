// Tyche's service worker: makes the app installable, answers page loads
// without a network with the offline page, takes images shared to the app
// and shows the badge's pushes. Only static files and /offline are ever
// cached, never a page or partial (they hold the user's data); a shared image
// is only kept until the transactions page picks it up.
// Bump CACHE when changing PRECACHE.
const CACHE = "tyche-static-v3";
const OFFLINE = "/offline";

const PRECACHE = [
  OFFLINE,
  "/styles/main.css",
  "/fonts/BioRhymeExpanded/BioRhymeExpanded.css",
  "/fonts/Afacad/Afacad.css",
  "/favicons/favicon.svg",
  "/favicons/icon-192.png",
  "/js/vendor/htmx.lib.js",
  "/js/vendor/litepicker.lib.js",
  "/js/vendor/alpine.lib.js",
  "/js/vendor/alpine.mask.lib.js",
  "/js/vendor/alpine.collapse.lib.js",
  "/js/main.js",
  "/js/datepickers.js",
  "/js/combobox.js",
  "/js/tags-input.js",
  "/js/drawer.js",
  "/js/transactions.js",
  "/js/receipts.js",
  "/js/money-inputs.js",
  "/js/passkeys.js",
  "/js/badge.js",
];

const STATIC = ["/styles/", "/js/", "/fonts/", "/icons/", "/favicons/", "/brand-icons/"];

self.addEventListener("install", (e) => {
  e.waitUntil(
    caches
      .open(CACHE)
      .then((c) => c.addAll(PRECACHE.map((u) => new Request(u, { cache: "reload" }))))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE && k !== BADGE_STATE && k !== SHARED).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

// An image shared to the app (the manifest's share_target posts it here) is
// kept for the transactions page, which opens the create form and scans it
// (receipts.js), then deletes it.
const SHARED = "tyche-shared";
const SHARED_URL = "/__shared-receipt";
const SHARE_ACTION = "/share-receipt";

async function takeShare(req) {
  try {
    const file = (await req.formData()).getAll("receipt").find((f) => f instanceof File && f.size > 0);
    const c = await caches.open(SHARED);
    if (file) {
      const headers = { "Content-Type": file.type || "application/octet-stream", "X-File-Name": encodeURIComponent(file.name || "receipt") };
      await c.put(SHARED_URL, new Response(file, { headers }));
      return Response.redirect("/transactions?new&shared=receipt", 303);
    }
  } catch {}
  return Response.redirect("/transactions?new", 303);
}

self.addEventListener("fetch", (e) => {
  const req = e.request;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;
  if (req.method === "POST" && url.pathname === SHARE_ACTION) {
    e.respondWith(takeShare(req));
    return;
  }
  if (req.method !== "GET") return;

  if (req.mode === "navigate") {
    e.respondWith(fetch(req).catch(() => caches.match(OFFLINE)));
    return;
  }

  // Network first: static urls aren't versioned, a cached copy would keep
  // old scripts after a deploy. The cache is only for when offline.
  if (STATIC.some((p) => url.pathname.startsWith(p))) {
    e.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            e.waitUntil(caches.open(CACHE).then((c) => c.put(req, copy)));
          }
          return res;
        })
        .catch(() => caches.match(req, { ignoreSearch: true }).then((res) => res ?? Response.error())),
    );
  }
});

// The badge's pushes (internal/badge): what needs attention, one notification
// per item (Android launchers count the notifications, there's no Badging
// API there) plus one summing up the ones left out, and the badge's number
// where there's one. A push replaces what's shown: new items buzz, the ones
// already shown stay as they are, gone ones are closed; 0 clears everything.
// A dismissed one stays hidden while unhandled until the server's repost
// push (the hours picked on the settings page after the first dismissal,
// POST /push/dismissed), which shows the dismissed ones again silently.
const BADGE_MORE = "badge-more";
// The single notification the badge used before.
const LEGACY_ATTENTION = "attention";
// What was last pushed and what was dismissed since, kept in a cache (a
// service worker has no localStorage): { items, more, title, url, dismissed }.
const BADGE_STATE = "tyche-badge-state";
const BADGE_STATE_URL = "/__badge-state";

function badgeOptions(silent) {
  const o = { icon: "/favicons/icon-192.png", badge: "/favicons/badge-96.png", silent };
  // A silent notification mustn't have a vibration pattern (TypeError).
  if (!silent) o.vibrate = [200, 100, 200];
  return o;
}

async function loadBadgeState() {
  try {
    const res = await (await caches.open(BADGE_STATE)).match(BADGE_STATE_URL);
    if (res) return await res.json();
  } catch {}
  return { items: [], more: 0, title: "", url: "/", dismissed: [] };
}

async function saveBadgeState(st) {
  try {
    const res = new Response(JSON.stringify(st), { headers: { "Content-Type": "application/json" } });
    await (await caches.open(BADGE_STATE)).put(BADGE_STATE_URL, res);
  } catch {}
}

function badgeNotifications() {
  return self.registration
    .getNotifications()
    .then((ns) => ns.filter((n) => n.data?.badge === true || n.tag === LEGACY_ATTENTION));
}

// showBadgeItem shows an item (or the summary of the ones left out).
function showBadgeItem(st, tag, silent) {
  if (tag === BADGE_MORE) {
    return self.registration.showNotification(st.title, {
      ...badgeOptions(silent),
      body: `And ${st.more} more`,
      tag: BADGE_MORE,
      data: { badge: true, url: st.url || "/" },
    });
  }
  const it = st.items.find((it) => it.tag === tag);
  return self.registration.showNotification(it.title, {
    ...badgeOptions(silent),
    body: it.body || "",
    tag: it.tag,
    data: { badge: true, url: it.url || "/" },
  });
}

function badgeTags(st) {
  const tags = st.items.map((it) => it.tag);
  if (st.more > 0) tags.push(BADGE_MORE);
  return tags;
}

// showBadge makes the shown notifications match st: closes the gone ones,
// shows the missing ones but the dismissed (buzzing, but for the summary
// only changing its number).
async function showBadge(st) {
  const tags = badgeTags(st);
  const shown = await badgeNotifications();
  const kept = new Set();
  for (const n of shown) {
    if (tags.includes(n.tag)) kept.add(n.tag);
    else n.close();
  }
  const missing = tags.filter((t) => !kept.has(t) && !st.dismissed.includes(t));
  await Promise.all(missing.map((t) => showBadgeItem(st, t, false)));
  for (const t of tags.filter((t) => t === BADGE_MORE && kept.has(t))) await showBadgeItem(st, t, true);
}

// repostBadge shows the dismissed notifications still unhandled again,
// silently. A push must show something: with nothing to show and nothing
// shown, the summary.
async function repostBadge() {
  const st = await loadBadgeState();
  const tags = badgeTags(st);
  const again = st.dismissed.filter((t) => tags.includes(t));
  st.dismissed = [];
  await saveBadgeState(st);
  await Promise.all(again.map((t) => showBadgeItem(st, t, true)));
  if (again.length === 0 && (await badgeNotifications()).length === 0) {
    await self.registration.showNotification(st.title || "Tyche", {
      ...badgeOptions(true),
      tag: BADGE_MORE,
      data: { badge: true, url: st.url || "/" },
    });
  }
}

self.addEventListener("push", (e) => {
  let c = {};
  try {
    c = e.data?.json() ?? {};
  } catch {}
  if (c.repost) {
    e.waitUntil(repostBadge());
    return;
  }
  const total = c.total || 0;
  const badge = self.navigator.setAppBadge
    ? (total > 0 ? self.navigator.setAppBadge(total) : self.navigator.clearAppBadge()).catch(() => {})
    : Promise.resolve();

  e.waitUntil(
    Promise.all([
      badge,
      loadBadgeState().then(async (prev) => {
        const st = {
          items: total > 0 ? c.items || [] : [],
          more: total > 0 ? c.more || 0 : 0,
          title: c.title || "",
          url: c.url || "/",
          dismissed: [],
        };
        // Still dismissed while still there.
        st.dismissed = prev.dismissed.filter((t) => badgeTags(st).includes(t));
        // Saved before closing the gone ones, so their close isn't taken
        // for a dismissal.
        await saveBadgeState(st);
        await showBadge(st);
      }),
    ]),
  );
});

// noteDismissed keeps a badge notification closed by the user (dismissed,
// or clicked: seen) hidden while unhandled, until the repost the server is
// asked for.
async function noteDismissed(n) {
  if (n.data?.badge !== true) return;
  const st = await loadBadgeState();
  if (!badgeTags(st).includes(n.tag) || st.dismissed.includes(n.tag)) return;
  st.dismissed.push(n.tag);
  await saveBadgeState(st);
  const sub = await self.registration.pushManager.getSubscription();
  if (!sub) return;
  await fetch("/push/dismissed", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ endpoint: sub.endpoint }),
  }).catch(() => {});
}

self.addEventListener("notificationclose", (e) => e.waitUntil(noteDismissed(e.notification)));

self.addEventListener("notificationclick", (e) => {
  e.notification.close();
  const url = new URL(e.notification.data?.url || "/", location.origin).href;
  e.waitUntil(
    Promise.all([
      noteDismissed(e.notification),
      self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((cs) => {
        const c = cs.find((c) => new URL(c.url).origin === location.origin);
        if (c) return c.focus().then((c) => c.navigate(url));
        return self.clients.openWindow(url);
      }),
    ]),
  );
});
