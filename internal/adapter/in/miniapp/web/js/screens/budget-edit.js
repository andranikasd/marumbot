// The budget, written. The monthly amount and the payday, validated against
// what the loans actually require this month; the cash on hand and the
// untouched reserve; the months that differ from the normal one.
"use strict";
import { haptic, toast, fmtMoney, moneyNum, num, group } from "../core.js";
import { T, sub, addStrings } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register, go, currentScreen } from "../nav.js";

addStrings({
  "budget.editing": "Խմբագրել բյուջեն", "budget.save": "Պահպանել բյուջեն",
}, {
  "budget.editing": "Edit budget", "budget.save": "Save budget",
});

const HTML = `
  <form id="budget-form" novalidate class="stack">
    <div class="card stack">
      <div class="field">
        <label for="monthly" data-i18n="budget.monthly">Սովորական ամսվա գումար</label>
        <div class="row">
          <input id="monthly" name="monthly" inputmode="decimal" placeholder="0" required>
          <select id="budget-currency" name="budget-currency" class="narrow">
            <option value="AMD" selected>AMD</option>
            <option value="USD">USD</option>
            <option value="EUR">EUR</option>
            <option value="RUB">RUB</option>
          </select>
        </div>
        <p class="hint" data-i18n="budget.hint"></p>
        <p class="error" id="e-monthly"></p>
        <p class="hint" id="b-note" hidden></p>
      </div>
      <div class="field">
        <label for="payday" data-i18n="budget.payday">Երբ է գումարը հասանելի</label>
        <div class="in unit-w"><input id="payday" name="payday" inputmode="numeric" placeholder="—"><span class="unit" data-i18n="unit.day">ամսի օր</span></div>
        <p class="hint" data-i18n="budget.payday.hint"></p>
        <p class="error" id="e-payday"></p>
      </div>
    </div>

    <div class="card stack">
      <div class="field">
        <label for="opening" data-i18n="budget.opening">Վարկերի համար հասանելի գումար հիմա</label>
        <div class="in unit-w"><input id="opening" name="opening" inputmode="decimal" placeholder="0"><span class="unit" id="u-opening">AMD</span></div>
        <p class="hint" data-i18n="budget.opening.hint"></p>
        <p class="error" id="e-opening"></p>
      </div>
      <div class="field">
        <label for="reserve" data-i18n="budget.reserve">Պահպանել անձեռնմխելի</label>
        <div class="in unit-w"><input id="reserve" name="reserve" inputmode="decimal" placeholder="0"><span class="unit" id="u-reserve">AMD</span></div>
        <p class="hint" data-i18n="budget.reserve.hint"></p>
        <p class="error" id="e-reserve"></p>
      </div>
      <div class="kv" id="b-usable-row" hidden><div><span data-i18n="budget.usable.label">Օգտագործելի միանգամյա գումար</span><b class="num ok" id="b-usable"></b></div></div>
    </div>

    <div class="card stack">
      <div class="field">
        <span class="lbl" data-i18n="budget.months">Տարբեր ամիսներ</span>
        <p class="hint" style="margin:0 0 8px" data-i18n="budget.months.hint"></p>
        <div id="override-list" class="stack" style="gap:8px"></div>
        <p class="error" id="e-overrides"></p>
      </div>
      <button class="alink" type="button" id="override-add" data-i18n="budget.months.add">Ավելացնել ամիս</button>
    </div>

    <button class="cta" type="submit" id="budget-save" data-i18n="budget.save">Պահպանել բյուջեն</button>
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
  del.type = "button"; del.className = "alink quiet"; del.style.width = "44px"; del.style.flexShrink = "0"; del.textContent = "✕";
  del.setAttribute("aria-label", T("budget.months.remove"));
  del.onclick = () => { haptic.tap(); row.remove(); validate(); };
  m.addEventListener("input", validate);
  a.addEventListener("input", validate);
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
      $("reserve").value = b.reserve_major > 0 ? String(b.reserve_major) : "";
      const list = $("override-list");
      list.textContent = "";
      for (const [month, amount] of Object.entries(b.overrides || {}).sort()) {
        list.append(overrideRow(month, amount));
      }
      required = b.required_major != null ? { major: b.required_major, currency: b.currency } : null;
      validate();
    });
  } catch { /* the form still works; the note simply lacks the fact */ }
}

function validate() {
  if (currentScreen() !== "budget-edit") return { ok: false };
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
  const reserveRaw = $("reserve").value.trim();
  const reserve = reserveRaw ? moneyNum(reserveRaw) : 0;
  let reserveErr = reserveRaw && (Number.isNaN(reserve) || reserve < 0) ? T("err.number") : "";
  if (!reserveErr && !opErr && reserve > op) reserveErr = T("budget.reserve.too_high");
  $("e-reserve").textContent = reserveErr;
  $("reserve").setAttribute("aria-invalid", reserveErr ? "true" : "false");
  if (reserveErr) err = err || reserveErr;
  const cur = $("budget-currency").value;
  $("u-opening").textContent = cur;
  $("u-reserve").textContent = cur;
  const usable = $("b-usable-row");
  usable.hidden = !opRaw || !!opErr || !!reserveErr;
  if (!usable.hidden) $("b-usable").textContent = fmtMoney(Math.max(0, op - reserve), cur);
  const o = readOverrides();
  $("e-overrides").textContent = o.err;
  if (o.err) err = err || o.err;
  const note = $("b-note");
  if (required && required.currency === cur && raw && !Number.isNaN(v)) {
    const surplus = v - required.major;
    note.hidden = false;
    note.textContent = sub("budget.against", { r: fmtMoney(required.major, cur) }) + " " +
      T(surplus < 0 ? "budget.low" : surplus === 0 ? "budget.exact" : "budget.ok");
    note.style.color = surplus < 0 ? "var(--danger)" : "";
  } else note.hidden = true;
  return { ok: !err, v, pd, opening: opRaw ? op : 0, reserve: reserveRaw ? reserve : 0, overrides: o.overrides };
}

async function save(e) {
  e?.preventDefault();
  if (busy) return;
  const b = validate();
  if (!b.ok) { haptic.bad(); if (!$("monthly").value.trim()) $("e-monthly").textContent = T("err.required"); return; }
  busy = true;
  $("budget-save").disabled = true;
  try {
    // The form shows the whole budget, so it posts the whole budget: the
    // stated months as a complete document, and the cash on hand as it
    // stands (zero withdraws the statement).
    const res = await api("api/budget", {
      method: "POST",
      body: JSON.stringify({
        monthly_major: b.v, currency: $("budget-currency").value, pay_day: b.pd,
        opening_major: b.opening, reserve_major: b.reserve, overrides: b.overrides,
      }),
    });
    if (!res.ok) throw new Error(String(res.status));
    haptic.ok();
    invalidate("api/");
    toast(T("saved"));
    go("budget");
  } catch {
    haptic.bad();
    $("e-monthly").textContent = T("err.save");
  } finally {
    busy = false;
    $("budget-save").disabled = false;
  }
}

register({
  id: "budget-edit",
  parent: "budget",
  titleKey: "budget.editing",
  html: HTML,
  onMount() {
    group($("monthly"));
    group($("opening"));
    group($("reserve"));
    for (const id of ["monthly", "payday", "opening", "reserve"]) $(id).addEventListener("input", validate);
    $("budget-currency").addEventListener("change", validate);
    $("override-add").addEventListener("click", () => {
      haptic.tap();
      $("override-list").append(overrideRow("", null));
      validate();
    });
    $("budget-form").addEventListener("submit", save);
  },
  onShow() { load(); },
});
