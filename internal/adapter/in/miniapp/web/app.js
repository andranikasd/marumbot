// The loan form.
//
// It shows a live estimate as the user types, which is the point of a form over
// a chat flow: seeing the monthly payment change while adjusting the term is
// how someone decides what they can afford. The estimate is explicitly marked
// as approximate -- the authoritative figure comes from the engine, and the
// engine runs on the server where it can be tested.
"use strict";

const tg = window.Telegram?.WebApp;
tg?.ready();
tg?.expand();

// ---------------------------------------------------------------------------
// Language. Telegram tells us the user's; anything we do not speak is Armenian,
// because an Armenian user with an unusual phone locale is far likelier here
// than an English speaker who set one.
// ---------------------------------------------------------------------------
const STRINGS = {
  hy: {
    title: "Ավելացնել վարկ", lede: "Մուտքագրեք վարկի պայմանագրի տվյալները։",
    "title.field": "Անվանում",
    "title.hint": "Ինչպե՞ս եք անվանում այս վարկը։",
    description: "Նշում (ըստ ցանկության)",
    "description.hint": "Բանկի անուն կամ հաշվեհամար պետք չէ։",
    principal: "Վարկի գումար", rate: "Տարեկան տոկոսադրույք (%)",
    method: "Մարման եղանակ",
    "method.annuity": "Անուիտետային (հավասար վճար)",
    "method.declining": "Դիֆերենցված (նվազող վճար)",
    "method.hint": "Անուիտետի դեպքում ամսական վճարը նույնն է։",
    start: "Սկիզբ", maturity: "Ավարտ", day: "Վճարման օրը",
    "day.hint": "Ամսվա այն օրը, երբ վճարումը պարտադիր է։",
    "sum.payment": "Ամսական վճար", "sum.count": "Վճարումների թիվ",
    "sum.interest": "Ընդհանուր տոկոս", "sum.total": "Ընդամենը",
    "sum.note": "Հաշվարկը մոտավոր է մինչև բանկի հաստատումը։",
      save: "Պահպանել", saving: "Պահպանվում է…",
    "budget.title": "Ամսական բյուջե",
    "budget.lede": "Որքա՞ն կարող եք ամսական հատկացնել վարկերին։",
    "budget.monthly": "Ամսական գումար",
    "budget.hint": "Ներառեք բոլոր պարտադիր վճարումները։",
    "manage.title": "Իմ վարկերը",
    "manage.lede": "Փոխեք անվանումը կամ հեռացրեք վարկը։",
    "manage.empty": "Դուք դեռ վարկ չեք ավելացրել։",
    "manage.edit": "Փոխել",
    "manage.remove": "Հեռացնել",
    "manage.save": "Պահպանել",
    "manage.cancel": "Չեղարկել",
    "manage.confirm": "Հեռացնե՞լ այս վարկը։ Հաշվարկները կպահպանվեն։",
    "manage.balance": "Մնացորդ՝ %s",
    "manage.yours": "Ձեր նշած թիվը",
    "err.required": "Պարտադիր դաշտ", "err.number": "Մուտքագրեք թիվ",
    "err.positive": "Պետք է լինի զրոյից մեծ", "err.day": "1-ից 31",
    "err.order": "Ավարտը պետք է լինի սկզբից հետո", "err.rate": "0-ից 200 միջակայքում",
    "err.save": "Չհաջողվեց պահպանել։ Փորձեք նորից։",
  },
  en: {
    title: "Add a loan", lede: "Enter the details from your loan agreement.",
    "title.field": "Name",
    "title.hint": "What do you call this loan?",
    description: "Note (optional)",
    "description.hint": "No bank name or account number needed.",
    principal: "Loan amount", rate: "Annual interest rate (%)",
    method: "Repayment method",
    "method.annuity": "Annuity (level payment)",
    "method.declining": "Declining (falling payment)",
    "method.hint": "An annuity keeps the monthly payment the same.",
    start: "Start", maturity: "Maturity", day: "Payment day",
    "day.hint": "The day of the month the payment falls due.",
    "sum.payment": "Monthly payment", "sum.count": "Payments",
    "sum.interest": "Total interest", "sum.total": "Total",
    "sum.note": "An estimate until your bank confirms it.",
    save: "Save", saving: "Saving…",
    "budget.title": "Monthly budget",
    "budget.lede": "How much can you put towards your loans each month?",
    "budget.monthly": "Monthly amount",
    "budget.hint": "Include every required payment.",
    "manage.title": "My loans",
    "manage.lede": "Rename a loan or remove it.",
    "manage.empty": "You have not added a loan yet.",
    "manage.edit": "Edit",
    "manage.remove": "Remove",
    "manage.save": "Save",
    "manage.cancel": "Cancel",
    "manage.confirm": "Remove this loan? Its calculations are kept.",
    "manage.balance": "Balance: %s",
    "manage.yours": "Your own figure",
    "err.required": "Required", "err.number": "Enter a number",
    "err.positive": "Must be above zero", "err.day": "Between 1 and 31",
    "err.order": "Maturity must be after the start", "err.rate": "Between 0 and 200",
    "err.save": "Could not save. Please try again.",
  },
};
const lang = (tg?.initDataUnsafe?.user?.language_code || "hy").slice(0, 2) === "en" ? "en" : "hy";
const T = (k) => STRINGS[lang][k] ?? k;
document.documentElement.lang = lang;
for (const el of document.querySelectorAll("[data-i18n]")) el.textContent = T(el.dataset.i18n);

// ---------------------------------------------------------------------------
// Which screen. One page with two forms rather than two pages: the Telegram
// webview reloads the whole app on navigation, and a second load is a second
// cold start in front of the user.
// ---------------------------------------------------------------------------
const REQUESTED = new URLSearchParams(location.search).get("screen");
const SCREEN = REQUESTED === "budget" || REQUESTED === "manage" ? REQUESTED : "loan";

// ---------------------------------------------------------------------------
// Sensible defaults, so the form opens mostly filled in.
// ---------------------------------------------------------------------------
const $ = (id) => document.getElementById(id);
const today = new Date();
const iso = (d) => d.toISOString().slice(0, 10);
$("start").value = iso(today);
$("maturity").value = iso(new Date(today.getFullYear() + 3, today.getMonth(), today.getDate()));
$("day").value = String(today.getDate());

// ---------------------------------------------------------------------------
// Validation. Every rule here is repeated on the server: this exists to tell
// the user quickly, not to be trusted.
// ---------------------------------------------------------------------------
const num = (s) => {
  // Accept both decimal separators and spaced thousands, because a phone
  // keyboard and an Armenian bank statement disagree about which is which.
  const cleaned = String(s).replace(/\s/g, "").replace(/,/g, ".");
  return /^\d*\.?\d*$/.test(cleaned) && cleaned !== "" ? Number(cleaned) : NaN;
};

function validate() {
  const errs = {};
  if (!$("title").value.trim()) errs.title = T("err.required");

  const p = num($("principal").value);
  if (!$("principal").value.trim()) errs.principal = T("err.required");
  else if (Number.isNaN(p)) errs.principal = T("err.number");
  else if (p <= 0) errs.principal = T("err.positive");

  const r = num($("rate").value);
  if (!$("rate").value.trim()) errs.rate = T("err.required");
  else if (Number.isNaN(r)) errs.rate = T("err.number");
  else if (r < 0 || r > 200) errs.rate = T("err.rate");

  const d = num($("day").value);
  if (!Number.isInteger(d) || d < 1 || d > 31) errs.day = T("err.day");

  const s = $("start").value, m = $("maturity").value;
  if (!s) errs.start = T("err.required");
  if (!m) errs.maturity = T("err.required");
  if (s && m && m <= s) errs.maturity = T("err.order");

  for (const f of ["title", "principal", "rate", "day", "start", "maturity"]) {
    const box = $("e-" + f);
    if (box) box.textContent = errs[f] || "";
    $(f).setAttribute("aria-invalid", errs[f] ? "true" : "false");
  }
  return { ok: Object.keys(errs).length === 0, p, r, d };
}

// ---------------------------------------------------------------------------
// The live estimate.
//
// Dated ACT/365, the same shape the server uses, so the preview does not
// contradict the answer that follows. It is float arithmetic and therefore
// approximate on purpose: the server's figure is the integer one, and this is
// labelled an estimate for exactly that reason.
// ---------------------------------------------------------------------------
function occurrences(start, day, maturity) {
  const out = [], s = new Date(start), end = new Date(maturity);
  for (let n = 1; n <= 600; n++) {
    const d = new Date(s.getFullYear(), s.getMonth() + n, 1);
    d.setDate(Math.min(day, new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate()));
    if (d > end) break;
    out.push(d);
  }
  return out;
}

function estimate() {
  const v = validate();
  if (!v.ok) { $("summary").hidden = true; tg?.MainButton.hide(); return; }

  const dates = occurrences($("start").value, v.d, $("maturity").value);
  if (dates.length === 0) { $("summary").hidden = true; tg?.MainButton.hide(); return; }

  const annual = v.r / 100, declining = $("method").value === "declining";
  const spans = [];
  let prev = new Date($("start").value);
  for (const d of dates) { spans.push((d - prev) / 86400000); prev = d; }

  const run = (pay) => {
    let bal = v.p, interest = 0;
    for (const days of spans) {
      const i = bal * annual * days / 365;
      interest += i;
      bal = bal + i - Math.min(pay, bal + i);
      if (bal <= 0) break;
    }
    return { bal, interest };
  };

  let payment, interest;
  if (declining) {
    const per = v.p / spans.length;
    let bal = v.p; interest = 0; payment = 0;
    spans.forEach((days, k) => {
      const i = bal * annual * days / 365;
      interest += i;
      if (k === 0) payment = per + i;
      bal -= per;
    });
  } else {
    // Bisect, exactly as the server does: unequal periods have no closed form.
    let lo = 0, hi = v.p * (1 + annual) + 1;
    for (let k = 0; k < 60; k++) {
      const mid = (lo + hi) / 2;
      if (run(mid).bal <= 0) hi = mid; else lo = mid;
    }
    payment = hi;
    interest = run(hi).interest;
  }

  const cur = $("currency").value;
  const fmt = new Intl.NumberFormat(lang === "en" ? "en-US" : "hy-AM",
    { style: "currency", currency: cur, maximumFractionDigits: 0 });
  $("s-payment").textContent = fmt.format(payment);
  $("s-count").textContent = String(spans.length);
  $("s-interest").textContent = fmt.format(interest);
  $("s-total").textContent = fmt.format(v.p + interest);
  $("summary").hidden = false;

  tg?.MainButton.setText(T("save"));
  tg?.MainButton.show();
}

for (const el of document.querySelectorAll("input,select")) {
  el.addEventListener("input", estimate);
  el.addEventListener("change", estimate);
}

// ---------------------------------------------------------------------------
// Saving. initData goes in a header, never in the body: it is a credential, and
// a credential in a body ends up in a log the first time somebody debugs a
// request.
// ---------------------------------------------------------------------------
async function save() {
  const v = validate();
  if (!v.ok) { tg?.HapticFeedback?.notificationOccurred("error"); return; }

  tg?.MainButton.showProgress();
  tg?.MainButton.setText(T("saving"));
  try {
    const res = await fetch("api/loans", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Init-Data": tg?.initData || "",
      },
      body: JSON.stringify({
        title: $("title").value.trim(),
        description: $("description").value.trim(),
        principal_major: v.p,
        currency: $("currency").value,
        rate_percent: v.r,
        method: $("method").value,
        start_date: $("start").value,
        maturity_date: $("maturity").value,
        payment_day: v.d,
      }),
    });
    if (!res.ok) throw new Error(String(res.status));
    tg?.HapticFeedback?.notificationOccurred("success");
    tg?.close();
  } catch {
    tg?.MainButton.hideProgress();
    tg?.MainButton.setText(T("save"));
    tg?.HapticFeedback?.notificationOccurred("error");
    $("e-title").textContent = T("err.save");
  }
}

// ---------------------------------------------------------------------------
// Budget screen.
// ---------------------------------------------------------------------------
function validateBudget() {
  const v = num($("monthly").value);
  let err = "";
  if (!$("monthly").value.trim()) err = T("err.required");
  else if (Number.isNaN(v)) err = T("err.number");
  else if (v <= 0) err = T("err.positive");
  $("e-monthly").textContent = err;
  $("monthly").setAttribute("aria-invalid", err ? "true" : "false");
  if (!err) { tg?.MainButton.setText(T("save")); tg?.MainButton.show(); }
  else tg?.MainButton.hide();
  return { ok: !err, v };
}

async function saveBudget() {
  const b = validateBudget();
  if (!b.ok) { tg?.HapticFeedback?.notificationOccurred("error"); return; }
  tg?.MainButton.showProgress();
  tg?.MainButton.setText(T("saving"));
  try {
    const res = await fetch("api/budget", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Init-Data": tg?.initData || "",
      },
      body: JSON.stringify({
        monthly_major: b.v,
        currency: $("budget-currency").value,
      }),
    });
    if (!res.ok) throw new Error(String(res.status));
    tg?.HapticFeedback?.notificationOccurred("success");
    tg?.close();
  } catch {
    tg?.MainButton.hideProgress();
    tg?.MainButton.setText(T("save"));
    tg?.HapticFeedback?.notificationOccurred("error");
    $("e-monthly").textContent = T("err.save");
  }
}

// ---------------------------------------------------------------------------
// Manage screen: rename or remove a loan.
//
// Contract terms are deliberately not editable here. Changing a rate or a date
// rewrites what every past balance meant, and doing that in place would leave
// no record that it happened. The schema keeps contract VERSIONS for exactly
// that reason, so a genuine change adds one rather than overwriting.
// ---------------------------------------------------------------------------
const api = (path, init = {}) => fetch(path, {
  ...init,
  headers: {
    "Content-Type": "application/json",
    "X-Telegram-Init-Data": tg?.initData || "",
    ...(init.headers || {}),
  },
});

function fmt(s, ...args) {
  let i = 0;
  return s.replace(/%s/g, () => args[i++] ?? "");
}

function loanCard(loan) {
  const el = document.createElement("div");
  el.className = "card";
  el.dataset.id = loan.id;

  const view = document.createElement("div");
  view.className = "view";
  const h = document.createElement("h2");
  h.textContent = loan.name;
  const meta = document.createElement("p");
  meta.className = "meta";
  meta.textContent = fmt(T("manage.balance"), loan.balance)
    + (loan.description ? " · " + loan.description : "")
    + (loan.confirmed ? "" : " · " + T("manage.yours"));
  const actions = document.createElement("div");
  actions.className = "actions";

  const edit = document.createElement("button");
  edit.type = "button";
  edit.textContent = T("manage.edit");
  edit.onclick = () => el.classList.add("editing");

  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "danger";
  remove.textContent = T("manage.remove");
  remove.onclick = async () => {
    // Telegram's own dialog, because a browser confirm() is blocked in the
    // webview on some clients and silently does nothing.
    const go = await new Promise((resolve) => {
      if (tg?.showConfirm) tg.showConfirm(T("manage.confirm"), resolve);
      else resolve(window.confirm(T("manage.confirm")));
    });
    if (!go) return;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "DELETE" });
    if (res.ok) { el.remove(); refreshEmpty(); tg?.HapticFeedback?.notificationOccurred("success"); }
    else tg?.HapticFeedback?.notificationOccurred("error");
  };

  actions.append(edit, remove);
  view.append(h, meta, actions);

  const form = document.createElement("div");
  form.className = "edit";
  const name = document.createElement("input");
  name.value = loan.name; name.maxLength = 60;
  const desc = document.createElement("input");
  desc.value = loan.description || ""; desc.maxLength = 200;
  desc.placeholder = T("description");
  const row = document.createElement("div");
  row.className = "actions";

  const save = document.createElement("button");
  save.type = "button";
  save.textContent = T("manage.save");
  save.onclick = async () => {
    if (!name.value.trim()) { name.setAttribute("aria-invalid", "true"); return; }
    const res = await api("api/loans/" + encodeURIComponent(loan.id), {
      method: "PATCH",
      body: JSON.stringify({ name: name.value.trim(), description: desc.value.trim() }),
    });
    if (!res.ok) { tg?.HapticFeedback?.notificationOccurred("error"); return; }
    h.textContent = name.value.trim();
    meta.textContent = fmt(T("manage.balance"), loan.balance)
      + (desc.value.trim() ? " · " + desc.value.trim() : "");
    el.classList.remove("editing");
    tg?.HapticFeedback?.notificationOccurred("success");
  };

  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.textContent = T("manage.cancel");
  cancel.onclick = () => {
    name.value = loan.name; desc.value = loan.description || "";
    el.classList.remove("editing");
  };

  row.append(save, cancel);
  form.append(name, desc, row);
  el.append(view, form);
  return el;
}

function refreshEmpty() {
  const any = $("manage-list").children.length > 0;
  $("manage-empty").hidden = any;
}

async function loadLoans() {
  const res = await api("api/loans");
  if (!res.ok) { $("manage-empty").hidden = false; return; }
  const { loans } = await res.json();
  const list = $("manage-list");
  list.textContent = "";
  for (const l of loans) list.append(loanCard(l));
  refreshEmpty();
}

if (SCREEN === "manage") {
  $("f").hidden = true;
  $("manage").hidden = false;
  document.querySelector("h1").textContent = T("manage.title");
  document.querySelector("p.lede").textContent = T("manage.lede");
  tg?.MainButton.hide();
  loadLoans();
} else if (SCREEN === "budget") {
  $("f").hidden = true;
  $("budget-form").hidden = false;
  document.querySelector("h1").textContent = T("budget.title");
  document.querySelector("p.lede").textContent = T("budget.lede");
  $("monthly").addEventListener("input", validateBudget);
  $("budget-currency").addEventListener("change", validateBudget);
  tg?.MainButton.onClick(saveBudget);
  $("budget-form").addEventListener("submit", (e) => { e.preventDefault(); saveBudget(); });
  validateBudget();
} else {
  tg?.MainButton.onClick(save);
  $("f").addEventListener("submit", (e) => { e.preventDefault(); save(); });
  estimate();
}
