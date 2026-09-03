// Fresh financial reads share concurrent requests. Offline fallback stays explicitly marked.
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
const inFlight=new Map();
let generation=0;
function fresh(path){
 if(inFlight.has(path))return inFlight.get(path);
 const stamp=generation;
 const pending=(async()=>{const res=await api(path);if(!res.ok)throw new Error("http "+res.status);const body=await res.json();if(stamp===generation)cache.set(path,{body,at:Date.now()});return body;})();
 inFlight.set(path,pending);pending.finally(()=>{if(inFlight.get(path)===pending)inFlight.delete(path);}).catch(()=>{});
 return pending;
}

// Return current data, or a labeled recent offline fallback.
export async function getJSON(path, onData) {
  const hit = cache.get(path);
  // Financial values are rendered only after a fresh response.
  try {
    const body = await fresh(path);
    offline(false);
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
 generation++;
 for(const k of inFlight.keys())if(k.startsWith(prefix))inFlight.delete(k);
  for (const k of cache.keys()) if (k.startsWith(prefix)) cache.delete(k);
}

export function prefetch(paths) {
  for (const p of paths) getJSON(p).catch(() => { /* warmed later by the screen */ });
}
