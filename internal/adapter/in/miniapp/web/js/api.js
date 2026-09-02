// Transport with a small read cache. GETs are stale-while-revalidate: the
// screen renders the cached body at once and re-renders when the fresh one
// lands, so tab switches feel instant while every figure still ends up
// current. Writes invalidate whatever they touched.
"use strict";
import { tg } from "./core.js";
import { T } from "./i18n.js";

export const api = (path, init = {}) => fetch(path, {
  ...init,
  headers: {
    "Content-Type": "application/json",
    "X-Telegram-Init-Data": tg?.initData || "",
    ...(init.headers || {}),
  },
});

const cache = new Map(); // path -> { body, at }
const TTL = 30_000;

// getJSON(path, onData): calls onData with the cached body immediately when
// one exists, then with the fresh body when it arrives (skipped if equal).
// Returns the fresh body, or throws when there is no cache to fall back on.
export async function getJSON(path, onData) {
  const hit = cache.get(path);
  // Financial values are rendered only after a fresh response.
  try {
    const res = await api(path);
    if (!res.ok) throw new Error("http " + res.status);
    const body = await res.json();
    offline(false);
    cache.set(path, { body, at: Date.now() });
    if (onData) onData(body, { stale: false });
    return body;
  } catch (err) {
    if (err instanceof TypeError) offline(true); // the network, not the server
    if (err instanceof TypeError && hit && Date.now() - hit.at < TTL) {
      if (onData) onData(hit.body, { stale:true });
      return hit.body;
    }
    throw err;
  }
}

// The offline banner: one line above every screen, shown when a request
// could not leave the phone and hidden the moment one succeeds. The cached
// figures stay on screen underneath; a retry re-runs the current screen.
let offlineNow = false;
function offline(on) {
  if (on === offlineNow) return;
  offlineNow = on;
  const el = document.getElementById("offline");
  if (el) el.hidden = !on;
}
export function watchOffline() {
  const el = document.getElementById("offline");
  if (!el) return;
  document.getElementById("offline-text").textContent = T("offline");
  const retry = document.getElementById("offline-retry");
  retry.textContent = T("offline.retry");
  retry.addEventListener("click", () => { offline(false); document.dispatchEvent(new Event("marum:retry")); });
  window.addEventListener("online", () => offline(false));
  window.addEventListener("offline", () => offline(true));
}

export function invalidate(prefix) {
  for (const k of cache.keys()) if (k.startsWith(prefix)) cache.delete(k);
}

export function prefetch(paths) {
  for (const p of paths) getJSON(p).catch(() => { /* warmed later by the screen */ });
}
