// The kernel: Telegram bridge, theme, haptics, formatting, toast, and the
// one MainButton. Everything else imports from here and from nav.js; no
// module reaches for window.Telegram on its own, so a change in the bridge
// is a change in one file.
"use strict";

export const tg = window.Telegram?.WebApp;
tg?.ready();
tg?.expand();

function applyTheme() {
  if (!tg) return;
  const p = tg.themeParams || {};
  const root = document.documentElement.style;
  const map = {
    bg_color: "--bg", text_color: "--fg", hint_color: "--hint", link_color: "--link",
    button_color: "--btn", button_text_color: "--btn-fg", secondary_bg_color: "--field",
    section_bg_color: "--card", accent_text_color: "--accent", destructive_text_color: "--danger",
  };
  for (const [k, v] of Object.entries(map)) if (p[k]) root.setProperty(v, p[k]);
  document.documentElement.style.colorScheme = tg.colorScheme || "light";
  try { tg.setHeaderColor?.("bg_color"); tg.setBackgroundColor?.("bg_color"); } catch { /* older clients */ }
}
applyTheme();
tg?.onEvent?.("themeChanged", applyTheme);

export const haptic = {
  tap: () => tg?.HapticFeedback?.impactOccurred?.("light"),
  pick: () => tg?.HapticFeedback?.selectionChanged?.(),
  ok: () => tg?.HapticFeedback?.notificationOccurred?.("success"),
  bad: () => tg?.HapticFeedback?.notificationOccurred?.("error"),
};

export const lang = (tg?.initDataUnsafe?.user?.language_code || "hy").slice(0, 2) === "en" ? "en" : "hy";
document.documentElement.lang = lang;

export const fmtMoney = (n, cur) => new Intl.NumberFormat(lang === "en" ? "en-US" : "hy-AM",
  { style: "currency", currency: cur, maximumFractionDigits: 0 }).format(n);
export const fmtDate = (iso) => {
  const d = new Date(iso + "T00:00:00");
  return Number.isNaN(d.getTime()) ? iso
    : new Intl.DateTimeFormat(lang === "en" ? "en-GB" : "hy-AM", { day: "numeric", month: "short" }).format(d);
};
export const num = (s) => {
  const cleaned = String(s).replace(/\s/g, "").replace(/,/g, ".");
  return /^\d*\.?\d*$/.test(cleaned) && cleaned !== "" ? Number(cleaned) : NaN;
};
export const esc = (t) => { const d = document.createElement("span"); d.textContent = t; return d.innerHTML; };

let toastTimer;
export function toast(msg) {
  const el = document.getElementById("toast");
  el.textContent = msg;
  el.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove("show"), 1800);
}

// One MainButton, owned by whichever screen is on. A screen calls
// mainButton.own(handler) on show; the kernel routes the click to the
// current owner and hides the button on every switch.
let mbHandler = null;
export const mainButton = {
  own(handler) { mbHandler = handler; },
  show(label) { tg?.MainButton.setText(label); tg?.MainButton.show(); },
  hide() { tg?.MainButton.hide(); },
  busy(label) { tg?.MainButton.showProgress(); tg?.MainButton.setText(label); },
  done(label) { tg?.MainButton.hideProgress(); tg?.MainButton.setText(label); },
};
tg?.MainButton.onClick(() => { if (mbHandler) mbHandler(); });

export const confirmDialog = (msg) => new Promise((resolve) => {
  if (tg?.showConfirm) tg.showConfirm(msg, resolve);
  else resolve(window.confirm(msg));
});

// group re-renders an amount input with thousand separators as the user
// types, keeping the caret at the end. Separators are spaces, which num()
// already strips, so validation never sees them.
export function group(el) {
  const fmt = () => {
    const raw = el.value.replace(/[^\d.,]/g, "");
    const [i, f] = raw.replace(/,/g, ".").split(".");
    if (!i) return;
    el.value = i.replace(/\B(?=(\d{3})+(?!\d))/g, " ") + (f !== undefined ? "." + f : "");
  };
  el.addEventListener("input", fmt);
  el.addEventListener("blur", fmt);
}
