// The navigator. A screen is a plug-in: it registers its id, its tab, its
// markup and two hooks, and the navigator does the rest — tab bar, screen
// switching, scroll reset, MainButton hygiene. Order in the tab bar is the
// order of registration in main.js.
"use strict";
import { haptic, mainButton } from "./core.js";
import { applyI18n, T } from "./i18n.js";

const screens = new Map();
let current = null;

export function register({ id, icon, labelKey, html, onMount, onShow }) {
  screens.set(id, { id, icon, labelKey, html, onMount, onShow, el: null });
}

export function currentScreen() { return current; }

export function go(id) {
  if (!screens.has(id)) id = "loans";
  const view = document.getElementById("view");
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
  for (const b of document.querySelectorAll("nav.tabs button")) {
    const on = b.dataset.go === id;
    b.classList.toggle("on", on);
    if (on) b.setAttribute("aria-current", "page");
    else b.removeAttribute("aria-current");
  }
  mainButton.hide();
  window.scrollTo(0, 0);
  s.onShow?.(s.el);
}

export function buildTabs() {
  const bar = document.getElementById("tabs");
  bar.setAttribute("aria-label", T("nav.label"));
  bar.textContent = "";
  for (const s of screens.values()) {
    const b = document.createElement("button");
    b.type = "button";
    b.dataset.go = s.id;
    b.innerHTML = '<span class="ic">' + s.icon + "</span><span>" + T(s.labelKey) + "</span>";
    b.addEventListener("click", () => { haptic.tap(); go(s.id); });
    bar.appendChild(b);
  }
  // Buttons inside screens can navigate too.
  document.getElementById("view").addEventListener("click", (e) => {
    const t = e.target.closest("[data-go]");
    if (t) { haptic.tap(); go(t.dataset.go); }
  });
}
