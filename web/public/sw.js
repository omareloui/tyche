// Tyche's service worker: makes the app installable, answers page loads
// without a network with the offline page and shows the badge's pushes. Only static files and /offline
// are ever cached, never a page or partial (they hold the user's data).
// Bump CACHE when changing PRECACHE.
const CACHE = "tyche-static-v2";
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
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;

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
// where there's one. A push replaces what's shown: items already shown stay
// as they are (no new buzz), gone ones are closed. 0 clears everything.
const BADGE_MORE = "badge-more";
// The single notification the badge used before.
const LEGACY_ATTENTION = "attention";
const BADGE_OPTIONS = {
  icon: "/favicons/icon-192.png",
  badge: "/favicons/badge-96.png",
  vibrate: [200, 100, 200],
};

function badgeNotifications() {
  return self.registration
    .getNotifications()
    .then((ns) => ns.filter((n) => n.data?.badge === true || n.tag === LEGACY_ATTENTION));
}

self.addEventListener("push", (e) => {
  let c = {};
  try {
    c = e.data?.json() ?? {};
  } catch {}
  const total = c.total || 0;
  const items = total > 0 ? c.items || [] : [];
  const more = total > 0 ? c.more || 0 : 0;
  const badge = self.navigator.setAppBadge
    ? (total > 0 ? self.navigator.setAppBadge(total) : self.navigator.clearAppBadge()).catch(() => {})
    : Promise.resolve();

  const wanted = new Set(items.map((it) => it.tag));
  if (more > 0) wanted.add(BADGE_MORE);
  const notify = badgeNotifications().then((shown) => {
    const kept = new Set();
    for (const n of shown) {
      if (wanted.has(n.tag)) kept.add(n.tag);
      else n.close();
    }
    const show = items
      .filter((it) => !kept.has(it.tag))
      .map((it) =>
        self.registration.showNotification(it.title, {
          ...BADGE_OPTIONS,
          body: it.body || "",
          tag: it.tag,
          data: { badge: true, url: it.url || "/" },
        }),
      );
    if (more > 0) {
      // Replaced in place, buzzing only when it's new.
      show.push(
        self.registration.showNotification(c.title, {
          ...BADGE_OPTIONS,
          body: `And ${more} more`,
          tag: BADGE_MORE,
          silent: kept.has(BADGE_MORE),
          data: { badge: true, url: c.url || "/" },
        }),
      );
    }
    return Promise.all(show);
  });
  e.waitUntil(Promise.all([badge, notify]));
});

self.addEventListener("notificationclick", (e) => {
  e.notification.close();
  const url = new URL(e.notification.data?.url || "/", location.origin).href;
  e.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((cs) => {
      const c = cs.find((c) => new URL(c.url).origin === location.origin);
      if (c) return c.focus().then((c) => c.navigate(url));
      return self.clients.openWindow(url);
    }),
  );
});
