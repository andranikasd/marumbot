// The loan app: one page, three screens, no reloads.
//
// The Telegram webview reloads the whole app on navigation, and every reload is
// a cold start in front of the user. So there is one page and screens show and
// hide. The estimate on the add screen is deliberately marked approximate --
// the authoritative figure is computed on the server, where it is tested.
"use strict";

const tg = window.Telegram?.WebApp;
tg?.ready();
tg?.expand();

// ---------------------------------------------------------------------------
// Language. Anything Marum does not speak is Armenian.
// ---------------------------------------------------------------------------
const STRINGS = {
  hy: {
    title: "Ավելացնել վարկ", lede: "Մուտքագրեք վարկի պայմանագրի տվյալները։",
    "title.field": "Անվանում", "title.hint": "Ինչպե՞ս եք անվանում այս վարկը։",
    description: "Նշում (ըստ ցանկության)", "description.hint": "Բանկի անուն կամ հաշվեհամար պետք չէ։",
    principal: "Վարկի գումար", balance: "Արդեն մարված գումար (ըստ ցանկության)",
    "balance.hint": "Եթե վարկն արդեն ընթացքի մեջ է՝ նշեք մայր գումարից մարված մասը։",
    "err.balance": "Պետք է լինի վարկի գումարից փոքր", rate: "Տարեկան տոկոսադրույք (%)", method: "Մարման եղանակ",
    "method.annuity": "Անուիտետային", "method.declining": "Դիֆերենցված",
    "method.hint": "Անուիտետի դեպքում ամսական վճարը նույնն է։",
    "method.hint.declining": "Դիֆերենցվածի դեպքում վճարը սկզբում մեծ է, հետո նվազում է։",
    start: "Սկիզբ", maturity: "Ավարտ", day: "Վճարման օրը",
    "day.hint": "Ամսվա այն օրը, երբ վճարումը պարտադիր է։",
    "sum.permonth": "ամսական", "sum.count": "Վճարումների թիվ",
    "sum.interest": "Ընդհանուր տոկոս", "sum.total": "Ընդամենը",
    "sum.note": "Հաշվարկը մոտավոր է մինչև բանկի հաստատումը։",
    save: "Պահպանել", saving: "Պահպանվում է…", saved: "Պահպանված է",
    "err.required": "Պարտադիր դաշտ", "err.number": "Մուտքագրեք թիվ",
    "err.positive": "Պետք է լինի զրոյից մեծ", "err.day": "1-ից 31",
    "err.order": "Ավարտը պետք է լինի սկզբից հետո", "err.rate": "0-ից 200 միջակայքում",
    "err.save": "Չհաջողվեց պահպանել։ Փորձեք նորից։",
    "manage.title": "Իմ վարկերը", "manage.lede": "Փոխեք անվանումը կամ հեռացրեք վարկը։",
    "manage.empty": "Դուք դեռ վարկ չեք ավելացրել։", "manage.add_first": "Ավելացնել առաջինը",
    "manage.edit": "Փոխել", "manage.remove": "Հեռացնել",
    "manage.save": "Պահպանել", "manage.cancel": "Չեղարկել",
    "manage.confirm": "Հեռացնե՞լ այս վարկը։ Հաշվարկները կպահպանվեն։",
    "manage.yours": "ձեր թիվը", "manage.until": "մինչև",
    "budget.title": "Ամսական բյուջե", "budget.lede": "Որքա՞ն կարող եք ամսական հատկացնել վարկերին։",
    "budget.monthly": "Ամսական գումար", "budget.hint": "Ներառեք բոլոր պարտադիր վճարումները։",
    "budget.required": "Պարտադիր վճարումներ", "budget.surplus": "Ավելցուկ",
    "budget.low": "Պակաս է պարտադիր վճարումներից։",
    "budget.ok": "Ավելցուկը կուղղվի մեկ վարկի՝ արագ մարման համար։",
    "tab.loans": "Վարկեր", "tab.add": "Ավելացնել", "tab.budget": "Բյուջե",
  },
  en: {
    title: "Add a loan", lede: "Enter the details from your loan agreement.",
    "title.field": "Name", "title.hint": "What do you call this loan?",
    description: "Note (optional)", "description.hint": "No bank name or account number needed.",
    principal: "Loan amount", balance: "Already repaid (optional)",
    "balance.hint": "If the loan is already running, enter the principal you have repaid so far.",
    "err.balance": "Must be less than the loan amount", rate: "Annual interest rate (%)", method: "Repayment method",
    "method.annuity": "Annuity", "method.declining": "Declining",
    "method.hint": "An annuity keeps the monthly payment the same.",
    "method.hint.declining": "Declining starts higher and falls every month.",
    start: "Start", maturity: "Maturity", day: "Payment day",
    "day.hint": "The day of the month the payment falls due.",
    "sum.permonth": "per month", "sum.count": "Payments",
    "sum.interest": "Total interest", "sum.total": "Total",
    "sum.note": "An estimate until your bank confirms it.",
    save: "Save", saving: "Saving…", saved: "Saved",
    "err.required": "Required", "err.number": "Enter a number",
    "err.positive": "Must be above zero", "err.day": "Between 1 and 31",
    "err.order": "Maturity must be after the start", "err.rate": "Between 0 and 200",
    "err.save": "Could not save. Please try again.",
    "manage.title": "My loans", "manage.lede": "Rename a loan or remove it.",
    "manage.empty": "You have not added a loan yet.", "manage.add_first": "Add the first one",
    "manage.edit": "Edit", "manage.remove": "Remove",
    "manage.save": "Save", "manage.cancel": "Cancel",
    "manage.confirm": "Remove this loan? Its calculations are kept.",
    "manage.yours": "your figure", "manage.until": "until",
    "budget.title": "Monthly budget", "budget.lede": "How much can you put towards your loans each month?",
    "budget.monthly": "Monthly amount", "budget.hint": "Include every required payment.",
    "budget.required": "Required payments", "budget.surplus": "Surplus",
    "budget.low": "Below your required payments.",
    "budget.ok": "The surplus goes to one loan, to clear it faster.",
    "tab.loans": "Loans", "tab.add": "Add", "tab.budget": "Budget",
  },
};
const lang = (tg?.initDataUnsafe?.user?.language_code || "hy").slice(0, 2) === "en" ? "en" : "hy";
const T = (k) => STRINGS[lang][k] ?? k;
document.documentElement.lang = lang;
for (const el of document.querySelectorAll("[data-i18n]")) el.textContent = T(el.dataset.i18n);

const $ = (id) => document.getElementById(id);
const fmtMoney = (n, cur) => new Intl.NumberFormat(lang === "en" ? "en-US" : "hy-AM",
  { style: "currency", currency: cur, maximumFractionDigits: 0 }).format(n);

// ---------------------------------------------------------------------------
// Transport. initData goes in a header, never in the body: it is a credential,
// and a credential in a body ends up in a log the first time somebody debugs.
// ---------------------------------------------------------------------------
const api = (path, init = {}) => fetch(path, {
  ...init,
  headers: {
    "Content-Type": "application/json",
    "X-Telegram-Init-Data": tg?.initData || "",
    ...(init.headers || {}),
  },
});

let toastTimer;
function toast(msg) {
  const el = $("toast");
  el.textContent = msg;
  el.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove("show"), 1800);
}

// ---------------------------------------------------------------------------
// Screens.
// ---------------------------------------------------------------------------
const SCREENS = ["loan", "manage", "budget"];
let current = null;

function go(name) {
  if (!SCREENS.includes(name)) name = "manage";
  current = name;
  for (const s of SCREENS) $("s-" + s).classList.toggle("on", s === name);
  for (const b of document.querySelectorAll("nav.tabs button")) b.classList.toggle("on", b.dataset.go === name);
  tg?.MainButton.hide();
  window.scrollTo(0, 0);
  if (name === "loan") estimate();
  if (name === "manage") loadLoans();
  if (name === "budget") loadBudget();
}
for (const b of document.querySelectorAll("[data-go]")) b.addEventListener("click", () => go(b.dataset.go));

// ---------------------------------------------------------------------------
// Add a loan.
// ---------------------------------------------------------------------------
const today = new Date();
const iso = (d) => d.toISOString().slice(0, 10);
$("start").value = iso(today);
$("maturity").value = iso(new Date(today.getFullYear() + 3, today.getMonth(), today.getDate()));
$("day").value = String(today.getDate());

const num = (s) => {
  const cleaned = String(s).replace(/\s/g, "").replace(/,/g, ".");
  return /^\d*\.?\d*$/.test(cleaned) && cleaned !== "" ? Number(cleaned) : NaN;
};
const method = () => document.querySelector('input[name="method"]:checked').value;

function validate() {
  const errs = {};
  if (!$("title").value.trim()) errs.title = T("err.required");
  const p = num($("principal").value);
  if (!$("principal").value.trim()) errs.principal = T("err.required");
  else if (Number.isNaN(p)) errs.principal = T("err.number");
  else if (p <= 0) errs.principal = T("err.positive");
  const paidRaw = $("balance").value.trim(), paid = paidRaw ? num(paidRaw) : 0;
  if (paidRaw && Number.isNaN(paid)) errs.balance = T("err.number");
  else if (paid < 0) errs.balance = T("err.positive");
  else if (!Number.isNaN(p) && paid >= p) errs.balance = T("err.balance");
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
  for (const f of ["title", "principal", "balance", "rate", "day", "start", "maturity"]) {
    const box = $("e-" + f);
    if (box) box.textContent = errs[f] || "";
    $(f).setAttribute("aria-invalid", errs[f] ? "true" : "false");
  }
  return { ok: Object.keys(errs).length === 0, p, r, d, paid };
}

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

// Dated ACT/365 in floats: approximate on purpose, the server's integer figure
// is the real one. Shaped the same way so the preview does not contradict it.
function estimate() {
  if (current !== "loan") return;
  $("method-hint").textContent = T(method() === "declining" ? "method.hint.declining" : "method.hint");
  const v = validate();
  if (!v.ok) { $("summary").hidden = true; tg?.MainButton.hide(); return; }
  const dates = occurrences($("start").value, v.d, $("maturity").value);
  if (dates.length === 0) { $("summary").hidden = true; tg?.MainButton.hide(); return; }

  const annual = v.r / 100, declining = method() === "declining";
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
    let lo = 0, hi = v.p * (1 + annual) + 1;
    for (let k = 0; k < 60; k++) {
      const mid = (lo + hi) / 2;
      if (run(mid).bal <= 0) hi = mid; else lo = mid;
    }
    payment = hi;
    interest = run(hi).interest;
  }
  const cur = $("currency").value;
  $("s-payment").textContent = fmtMoney(payment, cur);
  $("s-count").textContent = String(spans.length);
  $("s-interest").textContent = fmtMoney(interest, cur);
  $("s-total").textContent = fmtMoney(v.p + interest, cur);
  $("summary").hidden = false;
  tg?.MainButton.setText(T("save"));
  tg?.MainButton.show();
}
for (const el of $("f").querySelectorAll("input,select")) {
  el.addEventListener("input", estimate);
  el.addEventListener("change", estimate);
}

async function saveLoan() {
  const v = validate();
  if (!v.ok) { tg?.HapticFeedback?.notificationOccurred("error"); return; }
  tg?.MainButton.showProgress();
  tg?.MainButton.setText(T("saving"));
  try {
    const res = await api("api/loans", {
      method: "POST",
      body: JSON.stringify({
        title: $("title").value.trim(),
        description: $("description").value.trim(),
        principal_major: v.p, balance_major: v.paid ? v.p - v.paid : 0, currency: $("currency").value, rate_percent: v.r,
        method: method(), start_date: $("start").value,
        maturity_date: $("maturity").value, payment_day: v.d,
      }),
    });
    if (!res.ok) throw new Error(String(res.status));
    tg?.HapticFeedback?.notificationOccurred("success");
    $("f").reset();
    $("start").value = iso(today);
    $("maturity").value = iso(new Date(today.getFullYear() + 3, today.getMonth(), today.getDate()));
    $("day").value = String(today.getDate());
    toast(T("saved"));
    go("manage");
  } catch {
    tg?.MainButton.hideProgress();
    tg?.MainButton.setText(T("save"));
    tg?.HapticFeedback?.notificationOccurred("error");
    $("e-title").textContent = T("err.save");
  }
}
$("f").addEventListener("submit", (e) => { e.preventDefault(); saveLoan(); });

// ---------------------------------------------------------------------------
// My loans. Contract terms are deliberately not editable here: changing a rate
// or a date rewrites what every past balance meant, and the schema keeps
// contract versions for exactly that.
// ---------------------------------------------------------------------------
function loanCard(loan) {
  const el = document.createElement("div");
  el.className = "card"; el.dataset.id = loan.id;

  const view = document.createElement("div"); view.className = "view";
  const h = document.createElement("h2");
  const name = document.createElement("span"); name.textContent = loan.name;
  const bal = document.createElement("span"); bal.className = "bal"; bal.textContent = loan.balance;
  h.append(name, bal);
  const meta = document.createElement("p"); meta.className = "meta";
  meta.textContent = (loan.description ? loan.description + " · " : "") + T("manage.until") + " " + loan.maturity;
  if (!loan.confirmed) {
    const tag = document.createElement("span"); tag.className = "tag yours"; tag.textContent = T("manage.yours");
    meta.append(tag);
  }
  const actions = document.createElement("div"); actions.className = "actions";
  const edit = document.createElement("button"); edit.type = "button"; edit.textContent = T("manage.edit");
  edit.onclick = () => el.classList.add("editing");
  const remove = document.createElement("button"); remove.type = "button"; remove.className = "danger";
  remove.textContent = T("manage.remove");
  remove.onclick = async () => {
    const ok = await new Promise((resolve) => {
      if (tg?.showConfirm) tg.showConfirm(T("manage.confirm"), resolve);
      else resolve(window.confirm(T("manage.confirm")));
    });
    if (!ok) return;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "DELETE" });
    if (res.ok) { el.remove(); refreshEmpty(); tg?.HapticFeedback?.notificationOccurred("success"); }
    else tg?.HapticFeedback?.notificationOccurred("error");
  };
  actions.append(edit, remove);
  view.append(h, meta, actions);

  const form = document.createElement("div"); form.className = "edit";
  const nameIn = document.createElement("input"); nameIn.value = loan.name; nameIn.maxLength = 60;
  const descIn = document.createElement("input"); descIn.value = loan.description || "";
  descIn.maxLength = 200; descIn.placeholder = T("description");
  const row = document.createElement("div"); row.className = "actions";
  const save = document.createElement("button"); save.type = "button"; save.textContent = T("manage.save");
  save.onclick = async () => {
    if (!nameIn.value.trim()) { nameIn.setAttribute("aria-invalid", "true"); return; }
    const res = await api("api/loans/" + encodeURIComponent(loan.id), {
      method: "PATCH",
      body: JSON.stringify({ name: nameIn.value.trim(), description: descIn.value.trim() }),
    });
    if (!res.ok) { tg?.HapticFeedback?.notificationOccurred("error"); return; }
    loan.name = nameIn.value.trim(); loan.description = descIn.value.trim();
    name.textContent = loan.name;
    meta.firstChild.textContent = (loan.description ? loan.description + " · " : "") + T("manage.until") + " " + loan.maturity;
    el.classList.remove("editing");
    toast(T("saved"));
  };
  const cancel = document.createElement("button"); cancel.type = "button"; cancel.textContent = T("manage.cancel");
  cancel.onclick = () => { nameIn.value = loan.name; descIn.value = loan.description || ""; el.classList.remove("editing"); };
  row.append(save, cancel);
  form.append(nameIn, descIn, row);
  el.append(view, form);
  return el;
}
function refreshEmpty() { $("manage-empty").hidden = $("manage-list").children.length > 0; }
async function loadLoans() {
  const res = await api("api/loans");
  const list = $("manage-list"); list.textContent = "";
  if (res.ok) { const { loans } = await res.json(); for (const l of loans) list.append(loanCard(l)); }
  refreshEmpty();
}

// ---------------------------------------------------------------------------
// Budget. Shows what the loans require this month beside what is entered, so
// the number is chosen against a fact rather than a guess.
// ---------------------------------------------------------------------------
let required = null; // { amount_minor, currency, exponent }
async function loadBudget() {
  const res = await api("api/budget");
  if (!res.ok) return;
  const b = await res.json();
  if (b.monthly_major != null) $("monthly").value = String(b.monthly_major);
  if (b.currency) $("budget-currency").value = b.currency;
  required = b.required_major != null ? { major: b.required_major, currency: b.currency } : null;
  validateBudget();
}
function validateBudget() {
  if (current !== "budget") return { ok: false };
  const v = num($("monthly").value);
  let err = "";
  if (!$("monthly").value.trim()) err = T("err.required");
  else if (Number.isNaN(v)) err = T("err.number");
  else if (v <= 0) err = T("err.positive");
  $("e-monthly").textContent = err;
  $("monthly").setAttribute("aria-invalid", err ? "true" : "false");
  const cur = $("budget-currency").value;
  if (!err && required && required.currency === cur) {
    const surplus = v - required.major;
    $("b-required").textContent = fmtMoney(required.major, cur);
    $("b-surplus").textContent = fmtMoney(Math.max(0, surplus), cur);
    $("b-note").textContent = surplus < 0 ? T("budget.low") : T("budget.ok");
    $("b-note").style.color = surplus < 0 ? "var(--danger)" : "";
    $("budget-summary").hidden = false;
  } else $("budget-summary").hidden = true;
  if (!err) { tg?.MainButton.setText(T("save")); tg?.MainButton.show(); } else tg?.MainButton.hide();
  return { ok: !err, v };
}
async function saveBudget() {
  const b = validateBudget();
  if (!b.ok) { tg?.HapticFeedback?.notificationOccurred("error"); return; }
  tg?.MainButton.showProgress(); tg?.MainButton.setText(T("saving"));
  try {
    const res = await api("api/budget", {
      method: "POST",
      body: JSON.stringify({ monthly_major: b.v, currency: $("budget-currency").value }),
    });
    if (!res.ok) throw new Error(String(res.status));
    tg?.HapticFeedback?.notificationOccurred("success");
    tg?.MainButton.hideProgress(); tg?.MainButton.setText(T("save"));
    toast(T("saved"));
  } catch {
    tg?.MainButton.hideProgress(); tg?.MainButton.setText(T("save"));
    tg?.HapticFeedback?.notificationOccurred("error");
    $("e-monthly").textContent = T("err.save");
  }
}
$("monthly").addEventListener("input", validateBudget);
$("budget-currency").addEventListener("change", validateBudget);
$("budget-form").addEventListener("submit", (e) => { e.preventDefault(); saveBudget(); });

// One MainButton, dispatched by screen.
tg?.MainButton.onClick(() => { if (current === "loan") saveLoan(); else if (current === "budget") saveBudget(); });

// Start where the caller asked, else on the list.
const REQUESTED = new URLSearchParams(location.search).get("screen");
go(REQUESTED === "budget" ? "budget" : REQUESTED === "loan" || REQUESTED === "add" ? "loan" : "manage");
