// The navigator. A screen is a plug-in: it registers its id, its markup and
// two hooks, and the navigator does the rest — the app bar, the tab dock,
// screen switching, scroll reset, MainButton hygiene.
//
// Two kinds of screen. A top-level screen has a tab and sits in the dock,
// in registration order. A sub-screen names a parent instead: it gets a
// back button (Telegram's own inside Telegram, ours outside) and the dock
// hides while it is on, because a detail page has one way out, not four.
"use strict";
import { haptic, mainButton, backButton, lang, toast } from "./core.js";
import { applyI18n, T, STRINGS } from "./i18n.js";

import { beginView } from "./api.js";

const screens = new Map();
let navigationRevision = 0;
let current = null;
let params = null;
let renderedLanguage = lang;

export function register({ id, icon, labelKey, titleKey, parent, html, onMount, onShow, onLanguage }) {
  screens.set(id, { id, icon, labelKey, titleKey, parent, html, onMount, onShow, onLanguage, htmlLanguage: lang, el: null });
}

// Secondary tools are downloaded only when opened. Registering metadata keeps
// parent navigation intact without evaluating their forms during startup.
export function registerLazy({id,parent,load}) {
  if(!screens.has(id))screens.set(id,{id,parent,load,el:null});
}

export function currentScreen() { return current; }
export function currentParams() { return params; }

const $ = (id) => document.getElementById(id);

// setTitle and setAction let a screen own its app bar while it is on. The
// action is the one secondary verb a screen may offer up top (Edit, Reset);
// anything more belongs in the body.
export function setTitle(text) { $("appbar-title").textContent = text; }
export function setAction(label, handler) {
  const b = $("appbar-action");
  if (!label) { b.hidden = true; b.onclick = null; return; }
  b.textContent = label;
  b.hidden = false;
  b.onclick = () => { haptic.tap(); handler(); };
}

export function go(id, withParams) {
  const revision=++navigationRevision;
  const pending=screens.get(id);
  if(pending?.load){
    $("view").setAttribute("aria-busy","true");
    toast(T("loading"));
    pending.promise ??= pending.load().catch(error=>{pending.promise=null;throw error;});
    pending.promise.then(()=>{
      if(revision===navigationRevision)go(id,withParams);
    }).catch(()=>{
      if(revision!==navigationRevision)return;
      $("view").removeAttribute("aria-busy");
      if(!current)go("loans");
      toast(T("load.failed"));
      setAction(T("retry"),()=>go(id,withParams));
    });
    return;
  }
  $("view").removeAttribute("aria-busy");
  if (!screens.has(id)) id = "loans";
  const view = $("view");
  for (const s of screens.values()) {
    if (!s.el) continue;
    s.el.classList.toggle("on", s.id === id);
  }
  const s = screens.get(id);
  if (!s.el) {
    s.el = document.createElement("section");
    s.el.className = "screen on";
    s.el.innerHTML = s.html;
    relabel(s.el, s.htmlLanguage);
    applyI18n(s.el);
    view.appendChild(s.el);
    s.onMount?.(s.el);
  }
  current = id;
  params = withParams || null;
  let tabId=id;
  while(screens.get(tabId)?.parent) tabId=screens.get(tabId).parent;
  for (const b of document.querySelectorAll("nav.tabs button")) {
    const on = b.dataset.go === tabId;
    b.classList.toggle("on", on);
    if (on) b.setAttribute("aria-current", "page");
    else b.removeAttribute("aria-current");
  }
  document.body.classList.toggle("sub", !!s.parent);
  setTitle(T(s.titleKey || s.labelKey));
  setAction(null);
  if (s.parent) backButton.show(() => go(s.parent));
  else backButton.hide();
  mainButton.hide();
  window.scrollTo(0, 0);
  beginView();
  s.onShow?.(s.el, params);
}

export function buildTabs() {
  const bar = $("tabs");
  bar.setAttribute("aria-label", T("nav.label"));
  bar.textContent = "";
  for (const s of screens.values()) {
    if (s.parent) continue;
    const b = document.createElement("button");
    b.type = "button";
    b.dataset.go = s.id;
    b.innerHTML = s.icon + "<span>" + T(s.labelKey) + "</span>";
    b.addEventListener("click", () => { haptic.tap(); go(s.id); });
    bar.appendChild(b);
  }
  // Buttons inside screens can navigate too; data-arg carries a parameter.
  $("view").addEventListener("click", (e) => {
    const t = e.target.closest("[data-go]");
    if (t) { haptic.tap(); go(t.dataset.go, t.dataset.arg ? { id: t.dataset.arg } : null); }
  });
  // The offline banner's retry re-shows the current screen, which reloads it.
  document.addEventListener("marum:retry", () => { if (current) go(current, params); });
  $("appbar-back").addEventListener("click", () => {
    haptic.tap();
    const s = screens.get(current);
    go(s?.parent || "loans");
  });
}

// Legacy generated action captions have no data-i18n key. Only these trusted
// action buttons may use catalogue matching; never inspect option text, hints,
// loan names or arbitrary content. Other labels require explicit annotations.
function relabel(root, from) {
 if(from===lang)return;
 const translations=new Map();
 for(const key of Object.keys(STRINGS[from])){
  const before=STRINGS[from][key],after=STRINGS[lang][key];
  if(!before||!after||before.includes('{'))continue;
  translations.set(before,translations.has(before)&&translations.get(before)!==after?null:after);
 }
 for(const el of root.querySelectorAll('button.cta, button.alink, #appbar-action')){
  if(el.hasAttribute('data-arg')||el.querySelector('input, select, textarea'))continue;
  for(const node of el.childNodes){
   if(node.nodeType!==3)continue;
   const label=node.nodeValue.trim(),replacement=translations.get(label);
   if(replacement)node.nodeValue=node.nodeValue.replace(label,replacement);
  }
 }
 // The adjustment operation values are fixed UI enums, not loan identifiers.
 for(const option of root.querySelectorAll('#bp-adjustments select option')){
  const key={replacement_minor:'bp.replace',delta_minor:'bp.delta'}[option.value];
  if(key)option.textContent=T(key);
 }
 for(const el of root.querySelectorAll('#bp-adjustments input[type="month"], #bp-adjustments select'))el.setAttribute('aria-label',T('bp.adjust'));
 for(const el of root.querySelectorAll('#bp-adjustments input[inputmode="decimal"]'))el.setAttribute('aria-label',T('bp.limit'));
}

// Refresh forms in place; preserve the current view's stale-resource tracking.
export function refreshLanguage(){
 $('tabs').setAttribute('aria-label',T('nav.label'));
 $('offline-text').textContent=T('offline');
 $('offline-retry').textContent=T('offline.retry');
 for(const s of screens.values())if(s.el){relabel(s.el,renderedLanguage);applyI18n(s.el);s.onLanguage?.(s.el);}
 for(const b of document.querySelectorAll('nav.tabs button')){
  const s=screens.get(b.dataset.go);if(s)b.querySelector('span').textContent=T(s.labelKey);
 }
 const s=screens.get(current);
 if(s&&$('appbar-title').textContent===STRINGS[renderedLanguage][s.titleKey||s.labelKey])setTitle(T(s.titleKey||s.labelKey));
 relabel($('appbar-action').parentElement,renderedLanguage);
 const select=$('settings-language');if(select&&!select.disabled)select.value=lang;
 renderedLanguage=lang;
 // Read-only root cards contain generated labels/dates without annotations.
 // Reuse their normal fresh-read path rather than guessing at user content or
 // presenting an in-memory snapshot as fresh. Child forms and More's editable
 // preferences must never be re-shown by a background language response.
 if(s&&!s.parent&&s.id!=='more')s.onShow?.(s.el,params);
}
