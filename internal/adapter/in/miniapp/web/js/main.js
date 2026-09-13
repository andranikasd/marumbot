// Boot. The screens are plug-ins: importing one registers it, and the order
// of imports is the order of the tabs. Adding a screen is one file under
// screens/ and one import here.
"use strict";
import "./screens/simple-plan.js";
import "./screens/loans.js";
import "./screens/more.js";
import { buildTabs, go, refreshLanguage, registerLazy } from "./nav.js";
import { api, watchOffline, watchAuthentication, authenticationRequired, refreshAuthentication } from "./api.js";

import {lang,setLanguage,languageRevision,esc} from "./core.js";

import {T,addStrings} from "./i18n.js";
addStrings({'session.unavailable':'Այս հաշիվն անհասանելի է։ Դիմեք աջակցությանը։'},{'session.unavailable':'This account is unavailable. Contact support.'});

registerLazy({id:"welcome",parent:"simple-plan",load:()=>import("./screens/welcome.js")});
registerLazy({id:"extra",parent:"simple-plan",load:()=>import("./screens/extra.js")});
registerLazy({id:"loan-setup",parent:"loans",load:()=>import("./screens/loan-setup.js")});
registerLazy({id:"reminder-setup",parent:"more",load:()=>import("./screens/reminder-setup.js")});
registerLazy({id:"home",parent:"more",load:()=>import("./screens/home.js")});
registerLazy({id:"plan",parent:"more",load:()=>import("./screens/plan.js")});
registerLazy({id:"activity",parent:"loans",load:()=>import("./screens/activity.js")});
registerLazy({id:"budget",parent:"more",load:()=>import("./screens/budget.js")});
registerLazy({id:"plan-history",parent:"plan",load:()=>import("./screens/plan-history.js")});
registerLazy({id:"plan-inverse",parent:"plan",load:()=>import("./screens/plan-inverse.js")});
registerLazy({id:"plan-comparison",parent:"plan",load:()=>import("./screens/plan-comparison.js")});
registerLazy({id:"plan-scenarios",parent:"plan",load:()=>import("./screens/plan-scenarios.js")});
registerLazy({id:"payment",parent:"activity",load:()=>import("./screens/payment.js")});
registerLazy({id:"reconcile",parent:"activity",load:()=>import("./screens/reconcile.js")});
registerLazy({id:"add",parent:"loans",load:()=>import("./screens/add.js")});
registerLazy({id:"budget-edit",parent:"budget",load:()=>import("./screens/budget-edit.js")});
registerLazy({id:"budget-policy",parent:"budget",load:()=>import("./screens/budget-policy.js")});
registerLazy({id:"loan",parent:"loans",load:()=>import("./screens/loan.js")});
registerLazy({id:"paid-months",parent:"loans",load:()=>import("./screens/paid-months.js")});
registerLazy({id:"plan-start",parent:"plan",load:()=>import("./screens/plan-start.js")});

let sessionReady=false,sessionErrorKey='',prebootLocaleChoice=null;
buildTabs();
document.getElementById("tabs").hidden=true;
document.getElementById("appbar-language").onclick=()=>{
 if(authenticationRequired()||!sessionReady){setLanguage(lang==='hy'?'en':'hy');if(!sessionReady)prebootLocaleChoice=lang;refreshLanguage();refreshAuthentication();if(sessionErrorKey)document.getElementById('session-error').textContent=T(sessionErrorKey);}
 else go("more");
};
watchOffline();
watchAuthentication();

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

// The bot deep-links by screen name; an unknown name lands on the loans.
// A loan id beside the name opens that loan.
const query = new URLSearchParams(location.search);
const requested = query.get("screen") === "home" ? "simple-plan" : query.get("screen") || "simple-plan";

let languageSync=null;
let languageChoice=0;
// A pending account read must not overtake an explicit choice, even while
// saving that choice is still in flight.
document.addEventListener('change',event=>{
 if(event.target.id==='settings-language'){languageChoice++;prebootLocaleChoice=null;}
},true);
function syncLanguage(bootstrap=false){
 if((!sessionReady&&!bootstrap)||authenticationRequired())return;
 if(languageSync)return languageSync;
 const revision=languageRevision,choice=languageChoice;
 languageSync=(async()=>{try{
  if(prebootLocaleChoice){
   const choiceToSave=prebootLocaleChoice;
   const saved=await api('api/settings',{method:'POST',body:JSON.stringify({locale:choiceToSave})});
   if(saved.ok){const body=await saved.json();if(body.locale===choiceToSave&&prebootLocaleChoice===choiceToSave)prebootLocaleChoice=null;}
   return;
  }
  const res=await api('api/settings');if(!res.ok)return;
  const settings=await res.json();
  if(revision!==languageRevision||choice!==languageChoice||document.getElementById('settings-language')?.disabled)return;
  if((settings.locale==='en'||settings.locale==='hy')&&settings.locale!==lang){setLanguage(settings.locale);refreshLanguage();}
 }catch{}finally{languageSync=null;}})();
 return languageSync;
}
// Establish a verified account before any screen asks for private data. This
// makes direct Main Mini App entry work without a preceding /start message.
let sessionLoading=false;
async function startSession(){
 if(sessionLoading||sessionReady||authenticationRequired())return;
 sessionLoading=true;sessionErrorKey='';
 const root=document.getElementById('view');
 root.innerHTML=`<div class="state" role="status">${esc(T('loading'))}</div>`;
 try{
  const res=await api('api/session',{method:'POST',body:'{}'});
  if(!res.ok){
   if(res.status===403){const error=await res.json().catch(()=>({}));if(error.error==='account_unavailable')throw new Error('session.unavailable');}
   throw new Error('session');
  }
  const result=await res.json();if(result.ready!==true)throw new Error('session');
  if(prebootLocaleChoice){
   const languageButton=document.getElementById('appbar-language');languageButton.disabled=true;
   try{await syncLanguage(true);}finally{languageButton.disabled=false;}
  }
  sessionReady=true;root.innerHTML='';document.getElementById('tabs').hidden=false;
  go(requested,query.get('id')?{id:query.get('id')}:null);
  syncLanguage();
 }catch(error){
  if(!authenticationRequired()){
   sessionErrorKey=error.message==='session.unavailable'?'session.unavailable':'err.load';
   if(sessionErrorKey==='session.unavailable'){root.innerHTML=`<div class="state"><p id="session-error" role="alert">${esc(T(sessionErrorKey))}</p></div>`;return;}
   root.innerHTML=`<div class="state"><p id="session-error" role="alert">${esc(T('err.load'))}</p><button class="cta" id="session-retry">${esc(T('retry'))}</button></div>`;
   document.getElementById('session-retry').onclick=startSession;
  }
 }finally{sessionLoading=false;}
}
startSession();
document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible')syncLanguage();});
window.Telegram?.WebApp?.onEvent?.('activated',syncLanguage);
