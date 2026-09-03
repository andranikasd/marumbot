// Boot. The screens are plug-ins: importing one registers it, and the order
// of imports is the order of the tabs. Adding a screen is one file under
// screens/ and one import here.
"use strict";
import "./screens/home.js";
import "./screens/plan.js";
import "./screens/loans.js";
import "./screens/activity.js";
import "./screens/more.js";
import "./screens/budget.js";
import "./screens/budget-edit.js";
import { buildTabs, go, refreshLanguage, registerLazy } from "./nav.js";
import { api, prefetch, watchOffline } from "./api.js";

import {lang,setLanguage,languageRevision} from "./core.js";

registerLazy({id:"plan-history",parent:"plan",load:()=>import("./screens/plan-history.js")});
registerLazy({id:"plan-inverse",parent:"plan",load:()=>import("./screens/plan-inverse.js")});
registerLazy({id:"plan-comparison",parent:"plan",load:()=>import("./screens/plan-comparison.js")});
registerLazy({id:"plan-scenarios",parent:"plan",load:()=>import("./screens/plan-scenarios.js")});
registerLazy({id:"payment",parent:"activity",load:()=>import("./screens/payment.js")});
registerLazy({id:"reconcile",parent:"activity",load:()=>import("./screens/reconcile.js")});
registerLazy({id:"add",parent:"loans",load:()=>import("./screens/add.js")});
registerLazy({id:"budget-policy",parent:"budget",load:()=>import("./screens/budget-policy.js")});
registerLazy({id:"loan",parent:"loans",load:()=>import("./screens/loan.js")});

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
let languageChoice=0;
// A pending account read must not overtake an explicit choice, even while
// saving that choice is still in flight.
document.addEventListener('change',event=>{
 if(event.target.id==='settings-language')languageChoice++;
},true);
function syncLanguage(){
 if(languageSync)return languageSync;
 const revision=languageRevision,choice=languageChoice;
 languageSync=(async()=>{try{
  const res=await api('api/settings');if(!res.ok)return;
  const settings=await res.json();
  if(revision!==languageRevision||choice!==languageChoice||document.getElementById('settings-language')?.disabled)return;
  if((settings.locale==='en'||settings.locale==='hy')&&settings.locale!==lang){setLanguage(settings.locale);refreshLanguage();}
 }catch{}finally{languageSync=null;}})();
 return languageSync;
}
// Settings are not a prerequisite for useful content. Mount the deep link
// once; a late locale response only relabels the existing view in place.
go(requested, query.get("id") ? { id: query.get("id") } : null);
syncLanguage();
document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible')syncLanguage();});
window.Telegram?.WebApp?.onEvent?.('activated',syncLanguage);
