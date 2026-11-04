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

// The badge's pushes (internal/badge): the counts of what needs attention, as
// one silent notification replaced in place (the icon's dot on Android) and
// the badge's number where there's one. 0 clears both.
const ATTENTION = "attention";

function attentionBody(c) {
  const parts = [];
  if (c.sms) parts.push(`${c.sms} SMS to check`);
  if (c.review) parts.push(`${c.review} to review`);
  if (c.overdue) parts.push(`${c.overdue} overdue`);
  return parts.join(" · ");
}

self.addEventListener("push", (e) => {
  let c = {};
  try {
    c = e.data?.json() ?? {};
  } catch {}
  const total = c.total || 0;
  const badge = self.navigator.setAppBadge
    ? (total > 0 ? self.navigator.setAppBadge(total) : self.navigator.clearAppBadge()).catch(() => {})
    : Promise.resolve();

  if (total === 0) {
    e.waitUntil(
      Promise.all([badge, self.registration.getNotifications({ tag: ATTENTION }).then((ns) => ns.forEach((n) => n.close()))]),
    );
    return;
  }
  e.waitUntil(
    Promise.all([
      badge,
      self.registration.showNotification(total === 1 ? "1 thing needs your attention" : `${total} things need your attention`, {
        body: attentionBody(c),
        tag: ATTENTION,
        renotify: false,
        silent: true,
        icon: "/favicons/icon-192.png",
        data: { url: c.url || "/" },
      }),
    ]),
  );
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
