// The budget: the monthly amount and the payday, validated against what the
// loans actually require this month.
"use strict";
import { tg, haptic, toast, fmtMoney, moneyNum, num, mainButton, group } from "../core.js";
import { T } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register, currentScreen } from "../nav.js";

const HTML = `
<h1 data-i18n="budget.title">Ամսական բյուջե</h1>
  <p class="lede" data-i18n="budget.lede">Որքա՞ն կարող եք ամսական հատկացնել բոլոր վարկերին։</p>
  <form id="budget-form" novalidate>
    <div class="field">
      <label for="monthly" data-i18n="budget.monthly">Ամսական գումար</label>
      <div class="row">
        <input id="monthly" name="monthly" inputmode="decimal" placeholder="0" required>
        <select id="budget-currency" name="budget-currency" class="narrow">
          <option value="AMD" selected>AMD ֏</option>
          <option value="USD">USD $</option>
          <option value="EUR">EUR €</option>
          <option value="RUB">RUB ₽</option>
        </select>
      </div>
      <p class="hint" data-i18n="budget.hint">Ներառեք բոլոր պարտադիր վճարումները։</p>
      <p class="error" id="e-monthly"></p>
    </div>
    <div class="field">
      <label for="payday" data-i18n="budget.payday">Աշխատավարձի օր (ըստ ցանկության)</label>
      <div class="in">
        <input id="payday" name="payday" inputmode="numeric" min="1" max="31" placeholder="—">
        <span class="unit" data-i18n="unit.day">ամսի օր</span>
      </div>
      <p class="hint" data-i18n="budget.payday.hint">Եթե գումարը ստանում եք մինչև վճարման օրը, վաղ վճարելը տոկոս է խնայում։</p>
      <p class="error" id="e-payday"></p>
    </div>
    <div class="field">
      <label for="opening" data-i18n="budget.opening">Այսօր առկա գումար (ըստ ցանկության)</label>
      <div class="in">
        <input id="opening" name="opening" inputmode="decimal" placeholder="0">
        <span class="unit" id="u-opening">AMD</span>
      </div>
      <p class="hint" data-i18n="budget.opening.hint">Ինչ ունեք հիմա, որ կարող է գնալ վարկերին՝ բացի ամսական բյուջեից։ Գործում է միայն այս ամսվա համար։</p>
      <p class="error" id="e-opening"></p>
    </div>
    <div class="field">
      <label data-i18n="budget.months">Առանձին ամիսներ (ըստ ցանկության)</label>
      <p class="hint" data-i18n="budget.months.hint">Եթե որևէ ամիս ավելի շատ կամ քիչ ունեք, նշեք այդ ամսվա ամբողջ գումարը։ Մնացած ամիսները ամսական բյուջեով են։</p>
      <div id="override-list"></div>
      <button class="cta ghost" type="button" id="override-add" data-i18n="budget.months.add">Ավելացնել ամիս</button>
      <p class="error" id="e-overrides"></p>
    </div>
    <div class="summary" id="budget-summary" hidden>
      <dl class="bare">
        <dt data-i18n="budget.required">Պարտադիր վճարումներ</dt><dd id="b-required">—</dd>
        <dt data-i18n="budget.surplus">Ավելցուկ</dt><dd id="b-surplus">—</dd>
      </dl>
      <p class="note" id="b-note"></p>
    </div>
    <div class="done" id="budget-done">
      <p id="budget-done-text"></p>
      <button class="cta" type="button" id="budget-close" data-i18n="budget.back">Վերադառնալ չաթ</button>
    </div>
  </form>
`;

const $ = (id) => document.getElementById(id);
let required = null; // { major, currency }
let busy = false;
const maxOverrideMonths = 36;

// One stated month: a month picker beside its whole-month figure.
function overrideRow(month, amount) {
  const row = document.createElement("div");
  row.className = "row override";
  const m = document.createElement("input");
  m.type = "month"; m.className = "o-month"; m.value = month || "";
  const a = document.createElement("input");
  a.inputMode = "decimal"; a.className = "o-amount"; a.placeholder = "0";
  if (amount != null) a.value = String(amount);
  const del = document.createElement("button");
  del.type = "button"; del.className = "o-del"; del.textContent = "✕";
  del.setAttribute("aria-label", T("budget.months.remove"));
  del.onclick = () => { haptic.tap(); row.remove(); $("budget-done").classList.remove("show"); validate(); };
  m.addEventListener("input", validate);
  a.addEventListener("input", () => { $("budget-done").classList.remove("show"); validate(); });
  row.append(m, a, del);
  return row;
}

function readOverrides() {
  const out = {}; const seen = new Set(); let err = "";
  const rows = $("override-list").querySelectorAll(".override");
  if (rows.length > maxOverrideMonths) err = T("budget.months.limit");
  for (const row of rows) {
    const month = row.querySelector(".o-month").value;
    const raw = row.querySelector(".o-amount").value.trim();
    const v = raw ? moneyNum(raw) : NaN;
    if (!month || Number.isNaN(v) || v < 0) { err = T("budget.months.bad"); continue; }
    if (seen.has(month)) { err = T("budget.months.duplicate"); continue; }
    seen.add(month);
    out[month] = v;
  }
  return { overrides: out, err };
}

async function load() {
  try {
    await getJSON("api/budget", (b) => {
      if (b.monthly_major != null) $("monthly").value = String(b.monthly_major);
      if (b.currency) $("budget-currency").value = b.currency;
      $("payday").value = b.pay_day > 0 ? String(b.pay_day) : "";
      $("opening").value = b.opening_major > 0 ? String(b.opening_major) : "";
      const list = $("override-list");
      list.textContent = "";
      for (const [month, amount] of Object.entries(b.overrides || {}).sort()) {
        list.append(overrideRow(month, amount));
      }
      required = b.required_major != null ? { major: b.required_major, currency: b.currency } : null;
      validate();
    });
  } catch { /* the form still works; the summary simply lacks the fact */ }
}

function validate() {
  if (currentScreen() !== "budget") return { ok: false };
  const raw = $("monthly").value.trim(), v = moneyNum(raw);
  let err = "";
  if (!raw) err = T("err.required");
  else if (Number.isNaN(v)) err = T("err.number");
  else if (v <= 0) err = T("err.positive");
  $("e-monthly").textContent = raw ? err : "";
  $("monthly").setAttribute("aria-invalid", raw && err ? "true" : "false");
  const pdRaw = $("payday").value.trim(), pd = pdRaw ? num(pdRaw) : 0;
  const pdErr = pdRaw && (!Number.isInteger(pd) || pd < 1 || pd > 31) ? T("err.day") : "";
  $("e-payday").textContent = pdErr;
  $("payday").setAttribute("aria-invalid", pdErr ? "true" : "false");
  if (pdErr) err = err || pdErr;
  const opRaw = $("opening").value.trim(), op = opRaw ? moneyNum(opRaw) : 0;
  const opErr = opRaw && (Number.isNaN(op) || op < 0) ? T("err.number") : "";
  $("e-opening").textContent = opErr;
  $("opening").setAttribute("aria-invalid", opErr ? "true" : "false");
  if (opErr) err = err || opErr;
  const o = readOverrides();
  $("e-overrides").textContent = o.err;
  if (o.err) err = err || o.err;
  const cur = $("budget-currency").value;
  $("u-opening").textContent = cur;
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
  if (!err) mainButton.show(T("save")); else mainButton.hide();
  return { ok: !err, v, pd, opening: opRaw ? op : 0, overrides: o.overrides };
}

async function save() {
  if (busy) return;
  const b = validate();
  if (!b.ok) { haptic.bad(); return; }
  busy = true;
  mainButton.busy(T("saving"));
  try {
    // The form shows the whole budget, so it posts the whole budget: the
    // stated months as a complete document, and the cash on hand as it
    // stands (zero withdraws the statement).
    const res = await api("api/budget", {
      method: "POST",
      body: JSON.stringify({
        monthly_major: b.v, currency: $("budget-currency").value, pay_day: b.pd,
        opening_major: b.opening, overrides: b.overrides,
      }),
    });
    if (!res.ok) throw new Error(String(res.status));
    haptic.ok();
    invalidate("api/");
    toast(T("saved"));
    $("budget-done-text").innerHTML = T("budget.done");
    $("budget-done").classList.add("show");
    $("budget-done").scrollIntoView({ behavior: "smooth", block: "nearest" });
  } catch {
    haptic.bad();
    $("e-monthly").textContent = T("err.save");
  } finally {
    busy = false;
    mainButton.done(T("save"));
  }
}

register({
  id: "budget",
  icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><ellipse cx="12" cy="7" rx="7" ry="3"/><path d="M5 7v5c0 1.7 3.1 3 7 3s7-1.3 7-3V7M5 12v5c0 1.7 3.1 3 7 3s7-1.3 7-3v-5"/></svg>',
  labelKey: "tab.budget",
  html: HTML,
  onMount() {
    group($("monthly"));
    $("monthly").addEventListener("input", () => { $("budget-done").classList.remove("show"); validate(); });
    $("budget-currency").addEventListener("change", validate);
    $("payday").addEventListener("input", validate);
    $("opening").addEventListener("input", () => { $("budget-done").classList.remove("show"); validate(); });
    $("override-add").addEventListener("click", () => {
      haptic.tap();
      $("override-list").append(overrideRow("", null));
      validate();
    });
    $("budget-form").addEventListener("submit", (e) => { e.preventDefault(); save(); });
    $("budget-close").addEventListener("click", () => { haptic.tap(); if (tg?.close) tg.close(); });
  },
  onShow() {
    mainButton.own(save);
    $("budget-done").classList.remove("show");
    load();
  },
});
