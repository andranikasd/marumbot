// The add-a-loan screen: the form, live validation, and the client-side
// estimate. The estimate is approximate on purpose — the authoritative
// figure is computed on the server, where it is tested against real bank
// schedules.
"use strict";
import { haptic, toast, fmtMoney, num, mainButton, group } from "../core.js";
import { T } from "../i18n.js";
import { api, invalidate } from "../api.js";
import { register, go, currentScreen } from "../nav.js";

const HTML = `
<h1 data-i18n="title">Ավելացնել վարկ</h1>
  <p class="lede" data-i18n="lede">Մուտքագրեք վարկի պայմանագրի տվյալները։</p>
  <form id="f" novalidate>
    <div class="field">
      <label for="title" data-i18n="title.field">Անվանում</label>
      <input id="title" name="title" autocomplete="off" maxlength="60" required>
      <p class="hint" data-i18n="title.hint">Ինչպե՞ս եք անվանում այս վարկը։</p>
      <p class="error" id="e-title"></p>
    </div>
    <div class="field">
      <label for="description" data-i18n="description">Նշում (ըստ ցանկության)</label>
      <input id="description" name="description" autocomplete="off" maxlength="200">
      <p class="hint" data-i18n="description.hint">Բանկի անուն կամ հաշվեհամար պետք չէ։</p>
    </div>
    <div class="field">
      <label for="principal" data-i18n="principal">Վարկի գումար</label>
      <div class="row">
        <input id="principal" name="principal" inputmode="decimal" placeholder="0" required>
        <select id="currency" name="currency" class="narrow">
          <option value="AMD" selected>AMD ֏</option>
          <option value="USD">USD $</option>
          <option value="EUR">EUR €</option>
          <option value="RUB">RUB ₽</option>
        </select>
      </div>
      <p class="error" id="e-principal"></p>
    </div>
    <div class="field">
      <label for="balance" data-i18n="balance">Մնացորդ այսօր (ըստ ցանկության)</label>
      <div class="in">
        <input id="balance" name="balance" inputmode="decimal" placeholder="0">
        <span class="unit" id="u-balance">AMD</span>
      </div>
      <p class="hint" data-i18n="balance.hint">Մայր գումարի մնացորդը բանկի քաղվածքից։ Դատարկ՝ եթե վարկը նոր է։</p>
      <p class="error" id="e-balance"></p>
    </div>
    <div class="field">
      <label for="prepay" data-i18n="prepay">Ավել վճարելուց հետո</label>
      <select id="prepay" name="prepay">
        <option value="" data-i18n="prepay.unsure">Չգիտեմ՝ կհաշվեմ երկու ձևով</option>
        <option value="shorten_term" data-i18n="prepay.shorten">Վճարը նույնն է, ժամկետը կրճատվում է</option>
        <option value="reduce_instalment" data-i18n="prepay.reduce">Բանկը նվազեցնում է ամսական վճարը</option>
      </select>
      <p class="hint" data-i18n="prepay.hint">Ինչ է անում բանկը վաղաժամկետ մարումից հետո։ Գրված է պայմանագրում։</p>
    </div>
    <div class="field">
      <label for="rate" data-i18n="rate">Տարեկան տոկոսադրույք</label>
      <div class="in">
        <input id="rate" name="rate" inputmode="decimal" placeholder="14" required>
        <span class="unit">%</span>
      </div>
      <p class="error" id="e-rate"></p>
    </div>
    <div class="field">
      <label data-i18n="method">Մարման եղանակ</label>
      <div class="seg">
        <input type="radio" name="method" id="m-annuity" value="annuity" checked>
        <label for="m-annuity" data-i18n="method.annuity">Անուիտետային</label>
        <input type="radio" name="method" id="m-declining" value="declining">
        <label for="m-declining" data-i18n="method.declining">Դիֆերենցված</label>
      </div>
      <p class="hint" id="method-hint" data-i18n="method.hint">Անուիտետի դեպքում ամսական վճարը նույնն է։</p>
    </div>
    <div class="row">
      <div class="field">
        <label for="start" data-i18n="start">Սկիզբ</label>
        <input id="start" name="start" type="date" required>
        <p class="error" id="e-start"></p>
      </div>
      <div class="field">
        <label for="maturity" data-i18n="maturity">Ավարտ</label>
        <input id="maturity" name="maturity" type="date" required>
        <p class="error" id="e-maturity"></p>
      </div>
    </div>
    <div class="field">
      <label for="day" data-i18n="day">Վճարման օրը</label>
      <div class="in">
        <input id="day" name="day" inputmode="numeric" value="15" required>
        <span class="unit" data-i18n="unit.day">ամսի օր</span>
      </div>
      <p class="hint" data-i18n="day.hint">Ամսվա այն օրը, երբ վճարումը պարտադիր է։</p>
      <p class="error" id="e-day"></p>
    </div>
    <div class="summary" id="summary" hidden>
      <p class="label" data-i18n="sum.title">Նախնական հաշվարկ</p>
      <p class="big"><span id="s-payment">—</span><small data-i18n="sum.permonth">ամսական</small></p>
      <dl>
        <dt data-i18n="sum.count">Վճարումների թիվ</dt><dd id="s-count">—</dd>
        <dt data-i18n="sum.interest">Ընդհանուր տոկոս</dt><dd id="s-interest">—</dd>
        <dt data-i18n="sum.total">Ընդամենը</dt><dd id="s-total">—</dd>
      </dl>
      <p class="note" data-i18n="sum.note">Հաշվարկը մոտավոր է մինչև բանկի հաստատումը։</p>
    </div>
  </form>
`;

const $ = (id) => document.getElementById(id);
const today = new Date();
const iso = (d) => d.toISOString().slice(0, 10);
let submitted = false;
let busy = false;

function resetDates() {
  $("start").value = iso(today);
  $("maturity").value = iso(new Date(today.getFullYear() + 3, today.getMonth(), today.getDate()));
  $("day").value = String(today.getDate());
}

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
    const touched = $(f).dataset.touched === "1" || submitted;
    if (box) box.textContent = touched && errs[f] ? errs[f] : "";
    $(f).setAttribute("aria-invalid", touched && errs[f] ? "true" : "false");
  }
  return { ok: Object.keys(errs).length === 0, p, r, d, rem };
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

// Dated ACT/365 in floats: approximate on purpose; shaped like the server's
// integer arithmetic so the preview does not contradict it.
function estimate() {
  if (currentScreen() !== "add") return;
  $("method-hint").textContent = T(method() === "declining" ? "method.hint.declining" : "method.hint");
  $("u-balance").textContent = $("currency").value;
  const v = validate();
  if (!v.ok) { $("summary").hidden = true; mainButton.hide(); return; }
  const dates = occurrences($("start").value, v.d, $("maturity").value);
  if (dates.length === 0) { $("summary").hidden = true; mainButton.hide(); return; }

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
  mainButton.show(T("save"));
}

async function saveLoan() {
  if (busy) return;
  submitted = true;
  const v = validate();
  if (!v.ok) { haptic.bad(); return; }
  busy = true;
  mainButton.busy(T("saving"));
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
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || String(res.status));
    }
    haptic.ok();
    $("f").reset();
    for (const el of $("f").querySelectorAll("input,select")) delete el.dataset.touched;
    submitted = false;
    resetDates();
    invalidate("api/");
    toast(T("saved"));
    go("loans");
  } catch (err) {
    haptic.bad();
    $("e-title").textContent = err.message === "too_many_loans" ? T("err.too_many") : T("err.save");
  } finally {
    busy = false;
    mainButton.done(T("save"));
  }
}

register({
  id: "add",
  icon: "➕",
  labelKey: "tab.add",
  html: HTML,
  onMount() {
    resetDates();
    group($("principal"));
    group($("balance"));
    for (const el of $("f").querySelectorAll("input,select")) {
      el.addEventListener("input", () => { el.dataset.touched = "1"; estimate(); });
      el.addEventListener("change", () => { el.dataset.touched = "1"; estimate(); });
      el.addEventListener("blur", () => { el.dataset.touched = "1"; estimate(); });
    }
    for (const el of document.querySelectorAll('input[name="method"]')) el.addEventListener("change", haptic.pick);
    $("f").addEventListener("submit", (e) => { e.preventDefault(); saveLoan(); });
    mainButton.own(saveLoan);
  },
  onShow() {
    mainButton.own(saveLoan);
    estimate();
  },
});
