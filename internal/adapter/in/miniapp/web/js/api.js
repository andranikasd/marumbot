// Transport with a small read cache. GETs are stale-while-revalidate: the
// screen renders the cached body at once and re-renders when the fresh one
// lands, so tab switches feel instant while every figure still ends up
// current. Writes invalidate whatever they touched.
"use strict";
import { tg } from "./core.js";

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
  if (hit && onData) onData(hit.body, { stale: true });
  try {
    const res = await api(path);
    if (!res.ok) throw new Error("http " + res.status);
    const body = await res.json();
    const same = hit && JSON.stringify(hit.body) === JSON.stringify(body);
    cache.set(path, { body, at: Date.now() });
    if (onData && !same) onData(body, { stale: false });
    return body;
  } catch (err) {
    if (hit && Date.now() - hit.at < TTL) return hit.body;
    throw err;
  }
}

export function invalidate(prefix) {
  for (const k of cache.keys()) if (k.startsWith(prefix)) cache.delete(k);
}

export function prefetch(paths) {
  for (const p of paths) getJSON(p).catch(() => { /* warmed later by the screen */ });
}
