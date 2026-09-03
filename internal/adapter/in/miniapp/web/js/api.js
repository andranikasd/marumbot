// Fresh financial reads share concurrent requests. Offline fallback stays explicitly marked.
"use strict";
import { tg } from "./core.js";
import { T, addStrings } from "./i18n.js";

addStrings({'offline.stale':'Նախկինում բեռնված որոշ տվյալներ թարմացման կարիք ունեն։'}, {'offline.stale':'Some previously loaded figures need refreshing.'});

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
const stalePaths=new Set();
const displayedPaths=new Set();
let disconnected=false;
function updateOffline(){
 const label=document.getElementById('offline-text');
 if(label)label.textContent=T(disconnected?'offline':'offline.stale');
 offline(disconnected||stalePaths.size>0);
}
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
  const stamp=generation;
  // Financial values are rendered only after a fresh response.
  try {
    const body = await fresh(path);
    if(stamp!==generation)throw new Error("inputs changed during request");
    displayedPaths.add(path);
    stalePaths.delete(path);
    disconnected=false;
    updateOffline();
    if (onData) onData(body, { stale: false });
    return body;
  } catch (err) {
    if(stamp!==generation)throw err;
    if(displayedPaths.has(path)||hit)stalePaths.add(path);
    if(err instanceof TypeError)disconnected=true;
    updateOffline();
    if (err instanceof TypeError && hit && Date.now() - hit.at < TTL) {
      displayedPaths.add(path);
      if (onData) onData(hit.body, { stale:true });
      return hit.body;
    }
    throw err;
  }
}

// The offline banner: one line above every screen, shown when a request
// could not leave the phone. Only a fresh read of each affected path clears
// its stale state; connectivity and unrelated requests cannot certify it. The cached
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
  retry.addEventListener("click", () => { document.dispatchEvent(new Event("marum:retry")); });
  window.addEventListener("online", () => {disconnected=false;updateOffline();});
  window.addEventListener("offline", () => {disconnected=true;for(const path of displayedPaths)stalePaths.add(path);updateOffline();});
  updateOffline();
}

export function invalidate(prefix) {
 generation++;
 for(const k of inFlight.keys())if(k.startsWith(prefix))inFlight.delete(k);
  for (const k of cache.keys()) if (k.startsWith(prefix)) cache.delete(k);
  for(const path of displayedPaths)if(path.startsWith(prefix))stalePaths.add(path);
  updateOffline();
}

export function prefetch(paths) {
  for (const p of paths) fresh(p).catch(() => { /* warmed later by the screen */ });
}
