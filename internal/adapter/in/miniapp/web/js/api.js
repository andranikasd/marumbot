// Fresh financial reads share concurrent requests. Offline fallback stays explicitly marked.
"use strict";
import { tg } from "./core.js";
import { T, addStrings } from "./i18n.js";

addStrings({'offline.stale':'Նախկինում բեռնված որոշ տվյալներ թարմացման կարիք ունեն։'}, {'offline.stale':'Some previously loaded figures need refreshing.'});
addStrings({
 'auth.title':'Բացեք հավելվածը Telegram բոտից',
 'auth.missing':'Փակեք այս պատուհանը, բոտին ուղարկեք /start և սեղմեք բոտի պատասխանի կոճակը։',
 'auth.expired':'Մուտքն ավարտվել է։ Փակեք և նորից բացեք հավելվածը բոտի կոճակից։',
 'auth.account':'Նախ բոտին ուղարկեք /start, ապա նորից բացեք հավելվածը նրա պատասխանի կոճակից։',
 'auth.close':'Փակել հավելվածը'
},{
 'auth.title':'Open the app from the Telegram bot',
 'auth.missing':'Close this window, send /start to the bot, then tap the button attached to its reply.',
 'auth.expired':'Your session has expired or could not be verified. Close and reopen the app from the bot button.',
 'auth.account':'Send /start to the bot first, then reopen the app using the button in its reply.',
 'auth.close':'Close Mini App'
});
let authReason=null;
export function authenticationRequired(){return authReason!==null;}
export function refreshAuthentication(){
 const panel=document.getElementById('auth-required');if(!panel)return;
 panel.hidden=!authReason;
 if(!authReason)return;
 document.getElementById('auth-title').textContent=T('auth.title');
 document.getElementById('auth-message').textContent=T('auth.'+authReason);
 const close=document.getElementById('auth-close');close.textContent=T('auth.close');close.hidden=!tg?.close;
 close.onclick=()=>tg?.close?.();
 tg?.MainButton?.hide?.();tg?.BackButton?.hide?.();
 document.body?.classList?.remove('mb-on');
 for(const id of ['view','tabs','offline']){const el=document.getElementById(id);if(el)el.hidden=true;}
}
function rejectAuthentication(reason){
 authReason=reason;refreshAuthentication();
 return new Error('Telegram authentication required');
}
export function watchAuthentication(){
 if(!tg?.initData?.trim()){authReason='missing';refreshAuthentication();}
}

// Bound both connection and response-body waits. Mutations are never retried
// automatically: their screen retains the original idempotency key.
const REQUEST_TIMEOUT = 20_000;
async function bounded(operation, controller) {
  let timer;
  try {
    return await Promise.race([
      operation(),
      new Promise((_, reject) => {
        timer = setTimeout(() => {
          controller.abort();
          reject(new Error("request timed out"));
        }, REQUEST_TIMEOUT);
      }),
    ]);
  } finally { clearTimeout(timer); }
}
export async function api(path, init = {}) {
  if(authReason)throw rejectAuthentication(authReason);
  if(!tg?.initData?.trim())throw rejectAuthentication('missing');
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (init.signal?.aborted) abort();
  init.signal?.addEventListener("abort", abort, {once:true});
  try {
    const response = await bounded(() => fetch(path, {
      ...init, signal:controller.signal,
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Init-Data": tg?.initData || "",
        ...(init.headers || {}),
      },
    }), controller);
    if(response.status===401)throw rejectAuthentication('expired');
    if(response.status===403&&response.headers?.get('X-Marum-Auth-State')==='account-required')throw rejectAuthentication('account');
    for (const method of ["json", "text", "blob"]) {
      if (typeof response[method] !== "function") continue;
      const read = response[method].bind(response);
      response[method] = () => bounded(read, controller);
    }
    return response;
  } finally { init.signal?.removeEventListener("abort", abort); }
}

const cache = new Map(); // path -> { body, at }
const TTL = 30_000;
const inFlight=new Map();
let generation=0;
const stalePaths=new Set();
const displayedPaths=new Set();
let disconnected=false;
let visiblePaths=null;
// Hidden screens retain their stale state, but their warning belongs to them.
export function beginView() { visiblePaths=new Set(); updateOffline(); }
function updateOffline(){
 const label=document.getElementById('offline-text');
 if(label)label.textContent=T(disconnected?'offline':'offline.stale');
 offline(disconnected||[...stalePaths].some(path=>!visiblePaths||visiblePaths.has(path)));
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
  visiblePaths?.add(path);
  updateOffline();
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
  if (el) el.hidden = !on || authenticationRequired();
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
