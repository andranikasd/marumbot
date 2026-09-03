// The navigator. A screen is a plug-in: it registers its id, its markup and
// two hooks, and the navigator does the rest — the app bar, the tab dock,
// screen switching, scroll reset, MainButton hygiene.
//
// Two kinds of screen. A top-level screen has a tab and sits in the dock,
// in registration order. A sub-screen names a parent instead: it gets a
// back button (Telegram's own inside Telegram, ours outside) and the dock
// hides while it is on, because a detail page has one way out, not four.
"use strict";
import { haptic, mainButton, backButton } from "./core.js";
import { applyI18n, T } from "./i18n.js";

const screens = new Map();
let current = null;
let params = null;

export function register({ id, icon, labelKey, titleKey, parent, html, onMount, onShow }) {
  screens.set(id, { id, icon, labelKey, titleKey, parent, html, onMount, onShow, el: null });
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

// Update labels without resetting unsaved child-screen forms.
export function refreshLanguage(){
 $('tabs').setAttribute('aria-label',T('nav.label'));
 $('offline-text').textContent=T('offline');
 $('offline-retry').textContent=T('offline.retry');
 for(const s of screens.values())if(s.el)applyI18n(s.el);
 for(const b of document.querySelectorAll('nav.tabs button')){
  const s=screens.get(b.dataset.go);if(s)b.querySelector('span').textContent=T(s.labelKey);
 }
 const s=screens.get(current);if(!s)return;
 setTitle(T(s.titleKey||s.labelKey));
 if(!s.parent)s.onShow?.(s.el,params);
}
