// Boot. The screens are plug-ins: importing one registers it, and the order
// of imports is the order of the tabs. Adding a screen is one file under
// screens/ and one import here.
"use strict";
import "./screens/home.js";
import "./screens/plan.js";
import "./screens/plan-history.js";
import "./screens/plan-inverse.js";
import "./screens/plan-comparison.js";
import "./screens/plan-scenarios.js";
import "./screens/loans.js";
import "./screens/activity.js";
import "./screens/payment.js";
import "./screens/reconcile.js";
import "./screens/more.js";
import "./screens/loan.js";
import "./screens/add.js";
import "./screens/budget.js";
import "./screens/budget-edit.js";
import "./screens/budget-policy.js";
import { buildTabs, go, refreshLanguage } from "./nav.js";
import { api, prefetch, watchOffline } from "./api.js";

import {lang,setLanguage} from "./core.js";

buildTabs();
document.getElementById("appbar-language").onclick=()=>go("more");
watchOffline();

// The build badge: the one honest answer to "which version am I looking
// at". It reads the stamp off this module's own URL, so a cached copy
// names itself.
const stamp = (new URL(import.meta.url).pathname.match(/\/a\/([^/]+)\//) || [, "unstamped"])[1];
document.getElementById("build").textContent = "Marum " + stamp;

// Staleness. Telegram keeps a minimised Mini App alive across deploys and
// reopens the same instance, so a deploy is invisible until the page asks
// what is deployed and reloads itself. Asked on boot and each time the app
// comes back to the foreground; the reload goes to a URL that names the new
// build, so a webview that ignored no-store still fetches afresh. One reload
// per target version, so a server that disagrees forever cannot loop.
async function checkBuild() {
  try {
    const res = await fetch("version", { cache: "no-store" });
    if (!res.ok) return;
    const { version } = await res.json();
    if (!version || version === decodeURIComponent(stamp)) return;
    const key = "marum.reloaded-to";
    if (sessionStorage.getItem(key) === version) return;
    sessionStorage.setItem(key, version);
    const next = new URL(location.href);
    next.searchParams.set("v", version);
    location.replace(next.toString());
  } catch { /* offline, or an older server without the endpoint */ }
}
checkBuild();
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") checkBuild();
});
window.Telegram?.WebApp?.onEvent?.("activated", checkBuild);

// Share the initial loan request with Home; calculate plans only when opened.
prefetch(["api/loans"]);

// The bot deep-links by screen name; an unknown name lands on the loans.
// A loan id beside the name opens that loan.
const query = new URLSearchParams(location.search);
const requested = query.get("screen") || "home";

let languageSync=null;
function syncLanguage(){
 if(languageSync)return languageSync;
 languageSync=(async()=>{try{const res=await api('api/settings');if(!res.ok)return;const settings=await res.json();if(settings.locale!==lang){setLanguage(settings.locale);refreshLanguage();}}catch{}finally{languageSync=null;}})();
 return languageSync;
}
// Resolve the account language before mounting deep-linked forms, whose
// dynamic labels otherwise retain Telegram's language until reopened.
syncLanguage().finally(()=>go(requested, query.get("id") ? { id: query.get("id") } : null));
document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible')syncLanguage();});
window.Telegram?.WebApp?.onEvent?.('activated',syncLanguage);
