// Tyche's service worker: makes the app installable and answers page loads
// without a network with the offline page. Only static files and /offline
// are ever cached, never a page or partial (they hold the user's data).
// Bump CACHE when changing PRECACHE.
const CACHE = "tyche-static-v1";
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
