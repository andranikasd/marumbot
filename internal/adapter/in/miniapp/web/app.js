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
// Theme. The CSS reads Telegram's variables directly; this keeps the chrome
// (header, background) and the colour scheme in step when the theme changes
// while the app is open, and copies any params the CSS variables miss.
// ---------------------------------------------------------------------------
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

const haptic = {
  tap: () => tg?.HapticFeedback?.impactOccurred?.("light"),
  pick: () => tg?.HapticFeedback?.selectionChanged?.(),
  ok: () => tg?.HapticFeedback?.notificationOccurred?.("success"),
  bad: () => tg?.HapticFeedback?.notificationOccurred?.("error"),
};

// ---------------------------------------------------------------------------
// Language. Anything Marum does not speak is Armenian.
// ---------------------------------------------------------------------------
const STRINGS = {
  hy: {
    title: "Ավելացնել վարկ", lede: "Մուտքագրեք վարկի պայմանագրի տվյալները։",
    "title.field": "Անվանում", "title.hint": "Ինչպե՞ս եք անվանում այս վարկը։",
    description: "Նշում (ըստ ցանկության)", "description.hint": "Բանկի անուն կամ հաշվեհամար պետք չէ։",
    principal: "Վարկի գումար", balance: "Մնացորդ այսօր (ըստ ցանկության)",
    "balance.hint": "Մայր գումարի մնացորդը բանկի քաղվածքից։ Դատարկ՝ եթե վարկը նոր է։",
    "err.balance": "Չի կարող գերազանցել վարկի գումարը",
    prepay: "Ավել վճարելուց հետո", "prepay.unsure": "Չգիտեմ՝ կհաշվեմ երկու ձևով",
    "prepay.shorten": "Վճարը նույնն է, ժամկետը կրճատվում է",
    "prepay.reduce": "Բանկը նվազեցնում է ամսական վճարը",
    "prepay.hint": "Ինչ է անում բանկը վաղաժամկետ մարումից հետո։ Գրված է պայմանագրում։",
    "err.too_many": "Մեկ պլանում առավելագույնը 12 վարկ է։ Արխիվացրեք ավելորդները։", rate: "Տարեկան տոկոսադրույք", method: "Մարման եղանակ",
    "method.annuity": "Անուիտետային", "method.declining": "Դիֆերենցված",
    "method.hint": "Անուիտետի դեպքում ամսական վճարը նույնն է։",
    "method.hint.declining": "Դիֆերենցվածի դեպքում վճարը սկզբում մեծ է, հետո նվազում է։",
    start: "Սկիզբ", maturity: "Ավարտ", day: "Վճարման օրը", "unit.day": "ամսի օր",
    "day.hint": "Ամսվա այն օրը, երբ վճարումը պարտադիր է։",
    "sum.title": "Նախնական հաշվարկ", "sum.permonth": "ամսական", "sum.count": "Վճարումների թիվ",
    "sum.interest": "Ընդհանուր տոկոս", "sum.total": "Ընդամենը",
    "sum.note": "Հաշվարկը մոտավոր է մինչև բանկի հաստատումը։",
    save: "Պահպանել", saving: "Պահպանվում է…", saved: "Պահպանված է",
    loading: "Բեռնվում է…", retry: "Փորձել նորից",
    "err.required": "Պարտադիր դաշտ", "err.number": "Մուտքագրեք թիվ",
    "err.positive": "Պետք է լինի զրոյից մեծ", "err.day": "1-ից 31",
    "err.order": "Ավարտը պետք է լինի սկզբից հետո", "err.rate": "0-ից 200 միջակայքում",
    "err.save": "Չհաջողվեց պահպանել։ Փորձեք նորից։", "err.load": "Չհաջողվեց բեռնել։",
    "manage.title": "Իմ վարկերը", "manage.lede": "Ընդհանուր պատկերը, և յուրաքանչյուր վարկը՝ առանձին։",
    "manage.owed": "Ընդհանուր պարտք", "manage.required": "Այս ամիս պարտադիր", "manage.next": "Հաջորդ վճարումը",
    "manage.empty": "Դուք դեռ վարկ չեք ավելացրել։", "manage.add_first": "Ավելացնել առաջինը",
    "manage.edit": "Փոխել", "manage.remove": "Հեռացնել",
    "manage.save": "Պահպանել", "manage.cancel": "Չեղարկել",
    "manage.confirm": "Հեռացնե՞լ այս վարկը։ Հաշվարկները կպահպանվեն։",
    "manage.yours": "ձեր թիվը", "manage.until": "մինչև", "manage.nextpay": "Հաջորդը՝",
    "budget.title": "Ամսական բյուջե", "budget.lede": "Որքա՞ն կարող եք ամսական հատկացնել բոլոր վարկերին։",
    "budget.monthly": "Ամսական գումար", "budget.hint": "Ներառեք բոլոր պարտադիր վճարումները։",
    "budget.required": "Պարտադիր վճարումներ", "budget.surplus": "Ավելցուկ",
    "budget.payday": "Աշխատավարձի օր (ըստ ցանկության)",
    "budget.payday.hint": "Եթե գումարը ստանում եք մինչև վճարման օրը, վաղ վճարելը տոկոս է խնայում։",
    "budget.low": "Պակաս է պարտադիր վճարումներից՝ պլան չի ստացվի։",
    "budget.ok": "Ավելցուկն ամեն ամիս կուղղվի մեկ վարկի՝ ավելի շուտ փակելու համար։",
    "budget.exact": "Ծածկում է պարտադիրը՝ առանց ավելցուկի։",
    "budget.done": "Բյուջեն պահպանված է։ Պլանը տեսնելու համար չաթում սեղմեք <b>«💡 Ի՞նչ անել»</b>։",
    "budget.back": "Վերադառնալ չաթ",
    "tab.loans": "Վարկեր", "tab.add": "Ավելացնել", "tab.budget": "Բյուջե",
  },
  en: {
    title: "Add a loan", lede: "Enter the details from your loan agreement.",
    "title.field": "Name", "title.hint": "What do you call this loan?",
    description: "Note (optional)", "description.hint": "No bank name or account number needed.",
    principal: "Loan amount", balance: "Remaining balance today (optional)",
    "balance.hint": "The principal outstanding from your bank statement. Leave empty for a new loan.",
    "err.balance": "Cannot exceed the loan amount",
    prepay: "After an extra payment", "prepay.unsure": "Not sure — I will plan both ways",
    "prepay.shorten": "Instalment stays, term shortens",
    "prepay.reduce": "The bank lowers the instalment",
    "prepay.hint": "What the bank does after an early repayment. It is in the agreement.",
    "err.too_many": "A plan covers at most 12 loans. Archive the ones that no longer matter.", rate: "Annual interest rate", method: "Repayment method",
    "method.annuity": "Annuity", "method.declining": "Declining",
    "method.hint": "An annuity keeps the monthly instalment the same.",
    "method.hint.declining": "Declining starts higher and falls every month.",
    start: "Start", maturity: "Maturity", day: "Payment day", "unit.day": "of the month",
    "day.hint": "The day of the month the instalment falls due.",
    "sum.title": "Estimate", "sum.permonth": "per month", "sum.count": "Instalments",
    "sum.interest": "Total interest", "sum.total": "Total",
    "sum.note": "An estimate until your bank confirms it.",
    save: "Save", saving: "Saving…", saved: "Saved",
    loading: "Loading…", retry: "Try again",
    "err.required": "Required", "err.number": "Enter a number",
    "err.positive": "Must be above zero", "err.day": "Between 1 and 31",
    "err.order": "Maturity must be after the start", "err.rate": "Between 0 and 200",
    "err.save": "Could not save. Please try again.", "err.load": "Could not load.",
    "manage.title": "My loans", "manage.lede": "The whole picture, then each loan on its own.",
    "manage.owed": "Total owed", "manage.required": "Required this month", "manage.next": "Next instalment",
    "manage.empty": "You have not added a loan yet.", "manage.add_first": "Add the first one",
    "manage.edit": "Edit", "manage.remove": "Remove",
    "manage.save": "Save", "manage.cancel": "Cancel",
    "manage.confirm": "Remove this loan? Its calculations are kept.",
    "manage.yours": "your figure", "manage.until": "until", "manage.nextpay": "Next:",
    "budget.title": "Monthly budget", "budget.lede": "How much can you put towards all your loans each month?",
    "budget.monthly": "Monthly amount", "budget.hint": "Include every required instalment.",
    "budget.required": "Required instalments", "budget.surplus": "Surplus",
    "budget.payday": "Payday (optional)",
    "budget.payday.hint": "If the money arrives before the due date, paying early saves interest.",
    "budget.low": "Below your required instalments: no plan is possible.",
    "budget.ok": "The surplus goes to one loan every month, to clear it sooner.",
    "budget.exact": "Covers the required instalments with nothing spare.",
    "budget.done": "Budget saved. To see your plan, tap <b>“💡 What to do”</b> in the chat.",
    "budget.back": "Back to chat",
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
const fmtDate = (iso) => {
  const d = new Date(iso + "T00:00:00");
  return Number.isNaN(d.getTime()) ? iso
    : new Intl.DateTimeFormat(lang === "en" ? "en-GB" : "hy-AM", { day: "numeric", month: "short" }).format(d);
};

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
  if (name === "budget") { $("budget-done").classList.remove("show"); loadBudget(); }
}
for (const b of document.querySelectorAll("[data-go]")) {
  b.addEventListener("click", () => { haptic.tap(); go(b.dataset.go); });
}

// ---------------------------------------------------------------------------
// Add a loan.
// ---------------------------------------------------------------------------
const today = new Date();
const iso = (d) => d.toISOString().slice(0, 10);
function resetDates() {
  $("start").value = iso(today);
  $("maturity").value = iso(new Date(today.getFullYear() + 3, today.getMonth(), today.getDate()));
  $("day").value = String(today.getDate());
}
resetDates();

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
  const remRaw = $("balance").value.trim(), rem = remRaw ? num(remRaw) : 0;
  if (remRaw && Number.isNaN(rem)) errs.balance = T("err.number");
  else if (remRaw && rem <= 0) errs.balance = T("err.positive");
  else if (!Number.isNaN(p) && rem > p) errs.balance = T("err.balance");
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
    // An error is shown once the field has been touched, so an empty form does
    // not open covered in red; a submit shows everything.
    const touched = $(f).dataset.touched === "1" || submitted;
    if (box) box.textContent = touched && errs[f] ? errs[f] : "";
    $(f).setAttribute("aria-invalid", touched && errs[f] ? "true" : "false");
  }
  return { ok: Object.keys(errs).length === 0, p, r, d, rem };
}
let submitted = false;

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
  $("u-balance").textContent = $("currency").value;
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
  el.addEventListener("input", () => { el.dataset.touched = "1"; estimate(); });
  el.addEventListener("change", () => { el.dataset.touched = "1"; estimate(); });
  el.addEventListener("blur", () => { el.dataset.touched = "1"; estimate(); });
}
for (const el of document.querySelectorAll('input[name="method"]')) el.addEventListener("change", haptic.pick);

let busy = false;
async function saveLoan() {
  if (busy) return;
  submitted = true;
  const v = validate();
  if (!v.ok) { haptic.bad(); return; }
  busy = true;
  tg?.MainButton.showProgress();
  tg?.MainButton.setText(T("saving"));
  try {
    const res = await api("api/loans", {
      method: "POST",
      body: JSON.stringify({
        title: $("title").value.trim(),
        description: $("description").value.trim(),
        principal_major: v.p, balance_major: v.rem || 0, currency: $("currency").value, rate_percent: v.r,
        prepay_effect: $("prepay").value,
        method: method(), start_date: $("start").value,
        maturity_date: $("maturity").value, payment_day: v.d,
      }),
    });
    if (!res.ok) throw new Error(String(res.status));
    haptic.ok();
    $("f").reset();
    for (const el of $("f").querySelectorAll("input,select")) delete el.dataset.touched;
    submitted = false;
    resetDates();
    toast(T("saved"));
    go("manage");
  } catch {
    haptic.bad();
    $("e-title").textContent = T("err.save");
  } finally {
    busy = false;
    tg?.MainButton.hideProgress();
    tg?.MainButton.setText(T("save"));
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
  const name = document.createElement("span"); name.className = "name"; name.textContent = loan.name;
  const bal = document.createElement("span"); bal.className = "bal"; bal.textContent = loan.balance;
  h.append(name, bal);
  const meta = document.createElement("p"); meta.className = "meta";
  const metaText = () => (loan.description ? loan.description + " · " : "") + T("manage.until") + " " + fmtDate(loan.maturity);
  meta.append(document.createTextNode(metaText()));
  if (!loan.confirmed) {
    const tag = document.createElement("span"); tag.className = "tag yours"; tag.textContent = T("manage.yours");
    meta.append(tag);
  }
  const next = document.createElement("p"); next.className = "next";
  if (loan.next_due && loan.next_payment_major != null) {
    const b = document.createElement("b"); b.textContent = fmtMoney(loan.next_payment_major, loan.currency);
    next.append(document.createTextNode(T("manage.nextpay") + " "), b, document.createTextNode(" · " + fmtDate(loan.next_due)));
  } else next.style.margin = "0 0 12px";
  const actions = document.createElement("div"); actions.className = "actions";
  const edit = document.createElement("button"); edit.type = "button"; edit.textContent = T("manage.edit");
  edit.onclick = () => { haptic.tap(); el.classList.add("editing"); };
  const remove = document.createElement("button"); remove.type = "button"; remove.className = "danger";
  remove.textContent = T("manage.remove");
  remove.onclick = async () => {
    haptic.tap();
    const ok = await new Promise((resolve) => {
      if (tg?.showConfirm) tg.showConfirm(T("manage.confirm"), resolve);
      else resolve(window.confirm(T("manage.confirm")));
    });
    if (!ok) return;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "DELETE" });
    if (res.ok) { el.remove(); haptic.ok(); loadLoans(); }
    else haptic.bad();
  };
  actions.append(edit, remove);
  view.append(h, meta, next, actions);

  const form = document.createElement("div"); form.className = "edit";
  const nameIn = document.createElement("input"); nameIn.value = loan.name; nameIn.maxLength = 60;
  const descIn = document.createElement("input"); descIn.value = loan.description || "";
  descIn.maxLength = 200; descIn.placeholder = T("description");
  const row = document.createElement("div"); row.className = "actions";
  const save = document.createElement("button"); save.type = "button"; save.textContent = T("manage.save");
  save.onclick = async () => {
    if (!nameIn.value.trim()) { nameIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    save.disabled = true;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), {
      method: "PATCH",
      body: JSON.stringify({ name: nameIn.value.trim(), description: descIn.value.trim() }),
    });
    save.disabled = false;
    if (!res.ok) { haptic.bad(); return; }
    loan.name = nameIn.value.trim(); loan.description = descIn.value.trim();
    name.textContent = loan.name;
    meta.firstChild.textContent = metaText();
    el.classList.remove("editing");
    haptic.ok();
    toast(T("saved"));
  };
  const cancel = document.createElement("button"); cancel.type = "button"; cancel.textContent = T("manage.cancel");
  cancel.onclick = () => { nameIn.value = loan.name; descIn.value = loan.description || ""; el.classList.remove("editing"); };
  row.append(save, cancel);
  form.append(nameIn, descIn, row);
  el.append(view, form);
  return el;
}

// summarise totals the loans in the first loan's currency: total owed, what
// this month requires, and the earliest due date. A loan in another currency
// is left out of the sums rather than added as if it were the same money.
function summarise(loans) {
  const box = $("manage-summary");
  const live = loans.filter((l) => l.balance_major > 0);
  if (live.length === 0) { box.hidden = true; return; }
  const cur = live[0].currency;
  let owed = 0, required = 0, next = null;
  for (const l of live) {
    if (l.currency !== cur) continue;
    owed += l.balance_major;
    if (l.next_payment_major != null) required += l.next_payment_major;
    if (l.next_due && (!next || l.next_due < next.next_due)) next = l;
  }
  $("m-owed").textContent = fmtMoney(owed, cur);
  $("m-required").textContent = required > 0 ? fmtMoney(required, cur) : "—";
  $("m-next").textContent = next ? fmtDate(next.next_due) + " · " + next.name : "—";
  box.hidden = false;
}

let loansReq = 0;
async function loadLoans() {
  const seq = ++loansReq;
  const list = $("manage-list");
  $("manage-error").hidden = true;
  $("manage-empty").hidden = true;
  $("manage-loading").hidden = list.children.length > 0; // silent refresh when something is on screen
  try {
    const res = await api("api/loans");
    if (seq !== loansReq) return;
    if (!res.ok) throw new Error(String(res.status));
    const { loans } = await res.json();
    list.textContent = "";
    for (const l of loans) list.append(loanCard(l));
    summarise(loans);
    $("manage-empty").hidden = loans.length > 0;
  } catch {
    if (seq !== loansReq) return;
    $("manage-summary").hidden = true;
    $("manage-error").hidden = false;
  } finally {
    if (seq === loansReq) $("manage-loading").hidden = true;
  }
}
$("manage-retry").addEventListener("click", () => { haptic.tap(); loadLoans(); });

// ---------------------------------------------------------------------------
// Budget. Shows what the loans require this month beside what is entered, so
// the number is chosen against a fact rather than a guess.
// ---------------------------------------------------------------------------
let required = null; // { major, currency }
async function loadBudget() {
  const res = await api("api/budget");
  if (!res.ok) return;
  const b = await res.json();
  if (b.monthly_major != null) $("monthly").value = String(b.monthly_major);
  if (b.currency) $("budget-currency").value = b.currency;
  $("payday").value = b.pay_day > 0 ? String(b.pay_day) : "";
  required = b.required_major != null ? { major: b.required_major, currency: b.currency } : null;
  validateBudget();
}
function validateBudget() {
  if (current !== "budget") return { ok: false };
  const raw = $("monthly").value.trim(), v = num(raw);
  let err = "";
  if (!raw) err = T("err.required");
  else if (Number.isNaN(v)) err = T("err.number");
  else if (v <= 0) err = T("err.positive");
  // Only complain once there is something to complain about.
  $("e-monthly").textContent = raw ? err : "";
  $("monthly").setAttribute("aria-invalid", raw && err ? "true" : "false");
  const pdRaw = $("payday").value.trim(), pd = pdRaw ? num(pdRaw) : 0;
  const pdErr = pdRaw && (!Number.isInteger(pd) || pd < 1 || pd > 31) ? T("err.day") : "";
  $("e-payday").textContent = pdErr;
  $("payday").setAttribute("aria-invalid", pdErr ? "true" : "false");
  if (pdErr) err = err || pdErr;
  const cur = $("budget-currency").value;
  if (required && required.currency === cur) {
    const surplus = Number.isNaN(v) ? -required.major : v - required.major;
    $("b-required").textContent = fmtMoney(required.major, cur);
    $("b-surplus").textContent = raw && !Number.isNaN(v) ? fmtMoney(Math.max(0, surplus), cur) : "—";
    const note = $("b-note");
    if (!raw || Number.isNaN(v)) { note.textContent = ""; note.className = "note"; }
    else if (surplus < 0) { note.textContent = T("budget.low"); note.className = "note bad"; }
    else if (surplus === 0) { note.textContent = T("budget.exact"); note.className = "note"; }
    else { note.textContent = T("budget.ok"); note.className = "note good"; }
    $("budget-summary").hidden = false;
  } else $("budget-summary").hidden = true;
  if (!err) { tg?.MainButton.setText(T("save")); tg?.MainButton.show(); } else tg?.MainButton.hide();
  return { ok: !err, v, pd };
}
async function saveBudget() {
  if (busy) return;
  const b = validateBudget();
  if (!b.ok) { haptic.bad(); return; }
  busy = true;
  tg?.MainButton.showProgress(); tg?.MainButton.setText(T("saving"));
  try {
    const res = await api("api/budget", {
      method: "POST",
      body: JSON.stringify({ monthly_major: b.v, currency: $("budget-currency").value, pay_day: b.pd }),
    });
    if (!res.ok) throw new Error(String(res.status));
    haptic.ok();
    toast(T("saved"));
    // The hint carries markup of its own, never user text.
    $("budget-done-text").innerHTML = T("budget.done");
    $("budget-done").classList.add("show");
    $("budget-done").scrollIntoView({ behavior: "smooth", block: "nearest" });
  } catch {
    haptic.bad();
    $("e-monthly").textContent = T("err.save");
  } finally {
    busy = false;
    tg?.MainButton.hideProgress(); tg?.MainButton.setText(T("save"));
  }
}
$("monthly").addEventListener("input", () => { $("budget-done").classList.remove("show"); validateBudget(); });
$("budget-currency").addEventListener("change", validateBudget);
$("payday").addEventListener("input", validateBudget);
$("budget-form").addEventListener("submit", (e) => { e.preventDefault(); saveBudget(); });
$("budget-close").addEventListener("click", () => { haptic.tap(); if (tg?.close) tg.close(); });

// One MainButton, dispatched by screen.
tg?.MainButton.onClick(() => { if (current === "loan") saveLoan(); else if (current === "budget") saveBudget(); });

// Start where the caller asked, else on the list.
const REQUESTED = new URLSearchParams(location.search).get("screen");
go(REQUESTED === "budget" ? "budget" : REQUESTED === "loan" || REQUESTED === "add" ? "loan" : "manage");
