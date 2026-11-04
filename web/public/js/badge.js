// The installed app's badge: the sms needing attention, the transactions to
// review and the overdue scheduled ones (internal/badge). Each device picks
// what it counts (the settings page's "App badge" card, only shown in the
// installed app), kept in localStorage and sent with its Web Push
// subscription: the server pushes the counts when they change, the service
// worker (sw.js) showing them as a silent notification (the icon's dot on
// Android, which has no Badging API) and the badge's number where there's
// one. While the app is open the badge is set from GET /badge. It's off
// until turned on, which needs the notifications allowed: without them every
// source is turned off again.

const BADGE_SOURCES = ["sms", "review", "overdue"];
const BADGE_SOURCES_KEY = "badgeSources";
// The subscription last sent and when, to send it again once a day (or when
// the browser rotated it) rather than on every page load.
const BADGE_SYNCED_KEY = "badgeSynced";

function isInstalledPWA() {
  return window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
}

function badgeSources() {
  try {
    const s = JSON.parse(localStorage.getItem(BADGE_SOURCES_KEY));
    if (s && typeof s === "object") return Object.fromEntries(BADGE_SOURCES.map((k) => [k, s[k] === true]));
  } catch {}
  return Object.fromEntries(BADGE_SOURCES.map((k) => [k, false]));
}

function saveBadgeSources(sources) {
  try {
    localStorage.setItem(BADGE_SOURCES_KEY, JSON.stringify(sources));
  } catch {}
}

function badgeSourceList(sources) {
  return BADGE_SOURCES.filter((k) => sources[k]);
}

function pushSupported() {
  return "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
}

// badgeRequest sends a same-origin JSON request with the HX-Request header
// the unsafe /push routes require.
async function badgeRequest(method, url, body) {
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: { "HX-Request": "true", "Content-Type": "application/json", Accept: "application/json" },
    body: body && JSON.stringify(body),
  });
  const json = (res.headers.get("Content-Type") || "").includes("application/json");
  if (!res.ok || !json) {
    const err = json ? (await res.json().catch(() => ({}))).error : "";
    throw new Error(err || "Couldn't reach Tyche, try again.");
  }
  return res.json();
}

// syncBadge sets the badge's number from the current counts, closing the
// notification once there's nothing left. Locked or signed out, /badge
// answers a page, not JSON: the badge is left as is.
async function syncBadge() {
  if (!isInstalledPWA()) return;
  const res = await fetch("/badge", { credentials: "same-origin", headers: { Accept: "application/json" } }).catch(() => null);
  if (!res || !res.ok || !(res.headers.get("Content-Type") || "").includes("application/json")) return;
  const counts = await res.json();
  const total = badgeSourceList(badgeSources()).reduce((n, k) => n + (counts[k] || 0), 0);
  if ("setAppBadge" in navigator) {
    (total > 0 ? navigator.setAppBadge(total) : navigator.clearAppBadge()).catch(() => {});
  }
  if (total === 0 && "serviceWorker" in navigator) {
    const reg = await navigator.serviceWorker.getRegistration();
    for (const n of (await reg?.getNotifications({ tag: "attention" })) ?? []) n.close();
  }
}

function urlBase64ToUint8Array(s) {
  const b64 = (s + "=".repeat((4 - (s.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/");
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

// syncPush makes the push subscription match the picked sources: dropped
// when none is, else created (asking for the notifications permission when
// ask, so only from a click) and sent. force sends it even when sent today.
// Without the notifications allowed (or supported) every source is turned
// off, throwing why when asked.
async function syncPush({ ask = false, force = false } = {}) {
  let sources = badgeSourceList(badgeSources());
  if (sources.length > 0 && !pushSupported()) {
    saveBadgeSources({});
    if (ask) throw new Error("This browser can't show notifications, so it can't badge the app.");
    return;
  }
  if (!pushSupported()) return;
  // Before any await: browsers only ask from a user gesture.
  const asked = ask && sources.length > 0 && Notification.permission === "default" ? Notification.requestPermission() : null;
  const permission = asked ? await asked : Notification.permission;
  if (sources.length > 0 && permission !== "granted") {
    saveBadgeSources({});
    sources = [];
  }

  const reg = await navigator.serviceWorker.ready;
  let sub = await reg.pushManager.getSubscription();

  if (sources.length === 0) {
    if (sub) {
      await badgeRequest("DELETE", "/push/subscription", { endpoint: sub.endpoint }).catch(() => {});
      await sub.unsubscribe().catch(() => {});
    }
    for (const n of await reg.getNotifications({ tag: "attention" })) n.close();
    try {
      localStorage.removeItem(BADGE_SYNCED_KEY);
    } catch {}
    if (ask && permission !== "granted") {
      throw new Error(
        permission === "denied"
          ? "Notifications are blocked for Tyche: allow them in the browser's site settings, then turn the badge on again."
          : "The badge needs notifications: allow them to turn it on.",
      );
    }
    return;
  }

  if (!sub) {
    const { key } = await badgeRequest("GET", "/push/key");
    sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToUint8Array(key) });
  }

  const today = new Date().toISOString().slice(0, 10);
  const synced = `${sub.endpoint} ${sources.join(",")} ${today}`;
  try {
    if (!force && localStorage.getItem(BADGE_SYNCED_KEY) === synced) return;
  } catch {}
  await badgeRequest("PUT", "/push/subscription", { subscription: sub.toJSON(), sources });
  try {
    localStorage.setItem(BADGE_SYNCED_KEY, synced);
  } catch {}
}

if (isInstalledPWA()) {
  // Tells the server it's the installed app, e.g. for the onboarding to
  // offer the badge (skipped in a browser tab).
  document.addEventListener("htmx:configRequest", (e) => {
    e.detail.headers["X-Installed-App"] = "1";
  });
  window.addEventListener("load", () => {
    syncBadge();
    syncPush().catch(() => {});
  });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") syncBadge();
  });
  // Anything written may change the counts.
  let badgeTimer;
  document.addEventListener("htmx:afterRequest", (e) => {
    if ((e.detail.requestConfig?.verb || "get") === "get") return;
    clearTimeout(badgeTimer);
    badgeTimer = setTimeout(syncBadge, 500);
  });
}

// badgeSettings is the badge's switches: the settings page's "App badge"
// card and the onboarding's notifications step.
document.addEventListener("alpine:init", () => {
  Alpine.data("badgeSettings", () => ({
    installed: isInstalledPWA(),
    sources: badgeSources(),
    busy: false,
    status: "",
    failed: false,

    init() {
      if (pushSupported() && Notification.permission === "denied") {
        this.status = "Notifications are blocked for Tyche: allow them in the browser's site settings to turn the badge on.";
      }
    },

    async save() {
      this.busy = true;
      this.failed = false;
      this.status = "";
      saveBadgeSources(this.sources);
      try {
        await syncPush({ ask: true, force: true });
        this.status = "Saved";
      } catch (e) {
        this.failed = true;
        this.status = e.message;
      } finally {
        // Turned off again without the notifications.
        this.sources = badgeSources();
        this.busy = false;
      }
      syncBadge();
    },
  }));
});
