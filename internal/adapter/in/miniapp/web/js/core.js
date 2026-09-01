// The kernel: Telegram bridge, theme, haptics, formatting, toast, and the
// one MainButton. Everything else imports from here and from nav.js; no
// module reaches for window.Telegram on its own, so a change in the bridge
// is a change in one file.
"use strict";

export const tg = window.Telegram?.WebApp;
tg?.ready();
tg?.expand();

function applyTheme() {
  const scheme = tg?.colorScheme === "dark" ? "dark" : "light";
  document.documentElement.dataset.theme = scheme;
  document.documentElement.style.colorScheme = scheme;
  if (!tg) return;
  const p = tg.themeParams || {};
  const root = document.documentElement.style;
  // Telegram supplies only the ground: background, ink, hint, surfaces.
  // Accent, buttons and links stay brand green in every client theme —
  // mapping them too painted half the screen Telegram blue under a green
  // hero, two brands on one screen.
  const map = {
    bg_color: "--bg", text_color: "--fg", hint_color: "--hint",
    secondary_bg_color: "--field", section_bg_color: "--card",
  };
  for (const [k, v] of Object.entries(map)) if (p[k]) root.setProperty(v, p[k]);
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
// num parses an ordinary decimal field such as a rate. Its punctuation is
// always decimal; moneyNum has the different grouping rules money needs.
export const num = (s) => {
  const normalized = String(s).trim().replace(",", ".");
  const value = /^\d+(\.\d+)?$/.test(normalized) ? Number(normalized) : NaN;
  return Number.isFinite(value) ? value : NaN;
};

// moneyNum accepts the separators people actually use for money. A lone separator
// followed by one or two digits is decimal; three digits means grouping.
// When both marks occur, the last one is decimal (1,000.50 or 1.000,50).
export const moneyNum = (s) => {
  const raw = String(s).replace(/[\s\u00a0']/g, "");
  if (!/^\d[\d.,]*$/.test(raw)) return NaN;
  const dots = (raw.match(/\./g) || []).length;
  const commas = (raw.match(/,/g) || []).length;
  let normalized = raw;
  if (dots && commas) {
    const decimal = raw.lastIndexOf(".") > raw.lastIndexOf(",") ? "." : ",";
    const grouping = decimal === "." ? /,/g : /\./g;
    normalized = raw.replace(grouping, "").replace(decimal, ".");
  } else if (dots + commas === 1) {
    const separator = dots ? "." : ",";
    const fraction = raw.length - raw.lastIndexOf(separator) - 1;
    normalized = fraction >= 1 && fraction <= 2 ? raw.replace(separator, ".") : raw.replace(separator, "");
  } else if (dots + commas > 1) {
    const separator = dots ? "." : ",";
    normalized = raw.split(separator).every((part, i) => i === 0 ? part.length > 0 : part.length === 3)
      ? raw.replaceAll(separator, "") : "";
  }
  const value = /^\d+(\.\d+)?$/.test(normalized) ? Number(normalized) : NaN;
  return Number.isFinite(value) ? value : NaN;
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
// current owner and hides the button on every switch. While the button is
// up the tab dock hides (body.mb-on): one save action, one layer of chrome.
let mbHandler = null;
const mbOn = (on) => document.body.classList.toggle("mb-on", on);
export const mainButton = {
  own(handler) { mbHandler = handler; },
  show(label) { tg?.MainButton.setText(label); tg?.MainButton.show(); mbOn(!!tg); },
  hide() { tg?.MainButton.hide(); mbOn(false); },
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
    const value = moneyNum(el.value);
    if (Number.isNaN(value)) return;
    const raw = String(value);
    const [i, f] = raw.split(".");
    el.value = i.replace(/\B(?=(\d{3})+(?!\d))/g, " ") + (f !== undefined ? "." + f : "");
  };
  el.addEventListener("blur", fmt);
}
