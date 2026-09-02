// One loan. Three modes on one screen: the facts, restating the balance,
// and editing the terms. Restating the balance is the everyday act after a
// payment, so it is the one button; editing sits in the app bar; removal is
// a quiet link at the end, because it is a contract-level act.
//
// Contract terms ARE editable — the server never rewrites the current
// version, it files a NEW contract version with its own effective date, so
// every past balance keeps meaning what it meant. The currency alone is
// fixed: re-denominating a ledger is archive-and-refile, not an edit.
"use strict";
import { haptic, toast, fmtMoney, fmtFull, fmtMonth, moneyNum, num, confirmDialog, group } from "../core.js";
import { T, sub, addStrings } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register, go, setAction, setTitle } from "../nav.js";

addStrings({
  "loan.balance": "Մնացորդ", "loan.stated": "ձեր թիվը, {d}", "loan.confirmed": "բանկի հաստատած, {d}",
  "loan.paid": "{p}% մարված", "loan.active": "ակտիվ",
  "loan.next": "Հաջորդ վճարումը", "loan.payment": "Վճարում", "loan.day": "Վճարման օրը",
  "loan.rate": "Տոկոսադրույք", "loan.method": "Մարման եղանակ", "loan.maturity": "Ավարտ",
  "loan.contract": "Պայմանագիր", "loan.principal": "Վարկի գումար", "loan.start": "Սկիզբ",
  "loan.prepay": "Ավել վճարելուց հետո", "loan.note": "Նշում",
  "loan.update": "Թարմացնել մնացորդը", "loan.edit": "Խմբագրել", "loan.remove": "Հեռացնել վարկը",
  "loan.newbalance": "Նոր մնացորդ", "loan.newbalance.hint": "Մայր գումարի մնացորդը, որը ցույց է տալիս բանկի հավելվածը։",
  "loan.editing": "Խմբագրել պայմանները", "loan.save": "Պահպանել փոփոխությունները",
  "loan.missing": "Վարկը չի գտնվել։", "loan.day.unit": "ամսի օր",
}, {
  "loan.balance": "Balance", "loan.stated": "your figure, {d}", "loan.confirmed": "bank-confirmed, {d}",
  "loan.paid": "{p}% paid off", "loan.active": "active",
  "loan.next": "Next due", "loan.payment": "Payment", "loan.day": "Payment day",
  "loan.rate": "Rate", "loan.method": "Method", "loan.maturity": "Maturity",
  "loan.contract": "Contract", "loan.principal": "Loan amount", "loan.start": "Start",
  "loan.prepay": "After an extra payment", "loan.note": "Note",
  "loan.update": "Update balance", "loan.edit": "Edit", "loan.remove": "Remove loan",
  "loan.newbalance": "New balance", "loan.newbalance.hint": "The principal the bank app shows now.",
  "loan.editing": "Edit terms", "loan.save": "Save changes",
  "loan.missing": "This loan was not found.", "loan.day.unit": "of the month",
});

const HTML = `
  <div id="loan-view" class="stack" hidden>
    <div class="hero">
      <div class="k"><span data-i18n="loan.balance">Մնացորդ</span><span class="pill" id="ln-state"></span></div>
      <div class="v num" id="ln-balance">—</div>
      <div class="sub" id="ln-sub"></div>
      <div class="track" id="ln-track" hidden><i id="ln-track-fill"></i></div>
    </div>
    <div class="card kv" id="ln-facts"></div>
    <p class="sec" data-i18n="loan.contract">Պայմանագիր</p>
    <div class="card kv" id="ln-contract"></div>
    <button class="cta" type="button" id="ln-update" data-i18n="loan.update">Թարմացնել մնացորդը</button>
    <button class="alink red" type="button" id="ln-remove" data-i18n="loan.remove">Հեռացնել վարկը</button>
  </div>

  <form id="loan-balance" hidden novalidate class="card stack">
    <div class="field">
      <label for="ln-newbal" data-i18n="loan.newbalance">Նոր մնացորդ</label>
      <div class="in unit-w"><input id="ln-newbal" inputmode="decimal" placeholder="0"><span class="unit" id="ln-newbal-unit"></span></div>
      <p class="hint" data-i18n="loan.newbalance.hint"></p>
      <p class="error" id="e-newbal"></p>
    </div>
    <button class="cta" type="submit" data-i18n="save">Պահպանել</button>
    <button class="alink quiet" type="button" id="ln-bal-cancel" data-i18n="manage.cancel">Չեղարկել</button>
  </form>

  <form id="loan-edit" class="stack" hidden novalidate>
    <div class="card stack">
      <div class="field"><label for="le-name" data-i18n="title.field">Անվանում</label><input id="le-name" maxlength="60"><p class="error" id="e-le-name"></p></div>
      <div class="field"><label for="le-desc" data-i18n="loan.note">Նշում</label><input id="le-desc" maxlength="200"></div>
      <div class="pair">
        <div class="field"><label for="le-rate" data-i18n="loan.rate">Տոկոսադրույք</label><div class="in"><input id="le-rate" inputmode="decimal"><span class="unit">%</span></div><p class="error" id="e-le-rate"></p></div>
        <div class="field"><label for="le-day" data-i18n="loan.day">Վճարման օրը</label><input id="le-day" inputmode="numeric"><p class="error" id="e-le-day"></p></div>
      </div>
      <div class="field"><span class="lbl" data-i18n="loan.method">Մարման եղանակ</span>
        <div class="seg">
          <input type="radio" name="le-method" id="le-annuity" value="annuity"><label for="le-annuity" data-i18n="method.annuity">Անուիտետային</label>
          <input type="radio" name="le-method" id="le-declining" value="declining"><label for="le-declining" data-i18n="method.declining">Դիֆերենցված</label>
        </div>
      </div>
    </div>
    <div class="card stack">
      <div class="pair">
        <div class="field"><label for="le-start" data-i18n="loan.start">Սկիզբ</label><input id="le-start" type="date"></div>
        <div class="field"><label for="le-maturity" data-i18n="loan.maturity">Ավարտ</label><input id="le-maturity" type="date"><p class="error" id="e-le-maturity"></p></div>
      </div>
      <div class="field"><label for="le-prepay" data-i18n="loan.prepay">Ավել վճարելուց հետո</label>
        <select id="le-prepay">
          <option value="" data-i18n="prepay.unsure"></option>
          <option value="shorten_term" data-i18n="prepay.shorten"></option>
          <option value="reduce_instalment" data-i18n="prepay.reduce"></option>
        </select>
      </div>
    </div>
    <button class="cta" type="submit" data-i18n="loan.save">Պահպանել փոփոխությունները</button>
    <button class="alink quiet" type="button" id="ln-edit-cancel" data-i18n="manage.cancel">Չեղարկել</button>
  </form>

  <div id="loan-loading" hidden><div class="skel hero-skel"></div><div class="skel" style="margin-top:12px"></div></div>
  <div class="state" id="loan-missing" hidden>
    <b data-i18n="loan.missing">Վարկը չի գտնվել։</b>
    <button class="alink" type="button" data-go="loans" data-i18n="tab.loans">Վարկեր</button>
  </div>
`;

const $ = (id) => document.getElementById(id);
let loan = null;
let busy = false;

const row = (label, value, cls) => {
  const d = document.createElement("div");
  const s = document.createElement("span"); s.textContent = label;
  const b = document.createElement("b"); b.textContent = value; if (cls) b.className = cls;
  d.append(s, b);
  return d;
};

function mode(which) {
  $("loan-view").hidden = which !== "view";
  $("loan-balance").hidden = which !== "balance";
  $("loan-edit").hidden = which !== "edit";
  if (!loan) return;
  setTitle(which === "edit" ? T("loan.editing") : which === "balance" ? T("loan.update") : loan.name);
  if (which === "view") setAction(T("loan.edit"), () => { fill(); mode("edit"); });
  else setAction(null);
}

function render() {
  const cur = loan.currency;
  $("ln-balance").textContent = fmtMoney(loan.balance_major, cur);
  $("ln-state").textContent = T("loan.active");
  const bits = [];
  if (loan.balance_as_of) bits.push(sub(loan.confirmed ? "loan.confirmed" : "loan.stated", { d: fmtFull(loan.balance_as_of) }));
  let share = 0;
  if (loan.original_major && loan.original_major > loan.balance_major) {
    share = Math.round((1 - loan.balance_major / loan.original_major) * 100);
    bits.push(sub("loan.paid", { p: share }));
  }
  $("ln-sub").textContent = bits.join(" · ");
  $("ln-track").hidden = share <= 0;
  $("ln-track-fill").style.width = Math.max(2, Math.min(98, share)) + "%";

  const facts = $("ln-facts"); facts.textContent = "";
  facts.append(row(T("loan.next"), loan.next_due ? fmtFull(loan.next_due) : "—"));
  facts.append(row(T("loan.payment"), loan.next_payment_major != null ? fmtMoney(loan.next_payment_major, cur) : "—", "num"));
  facts.append(row(T("loan.day"), loan.payment_day != null ? String(loan.payment_day) : "—"));
  facts.append(row(T("loan.rate"), loan.rate_percent != null ? loan.rate_percent + "%" : "—"));
  facts.append(row(T("loan.method"), T(loan.method === "declining" ? "method.declining" : "method.annuity")));
  facts.append(row(T("loan.maturity"), fmtMonth(loan.maturity)));

  const c = $("ln-contract"); c.textContent = "";
  if (loan.original_major) c.append(row(T("loan.principal"), fmtMoney(loan.original_major, cur), "num"));
  c.append(row(T("loan.start"), fmtFull(loan.start)));
  c.append(row(T("loan.maturity"), fmtFull(loan.maturity)));
  c.append(row(T("loan.prepay"), T(loan.prepay_effect === "shorten_term" ? "prepay.shorten"
    : loan.prepay_effect === "reduce_instalment" ? "prepay.reduce" : "prepay.unsure")));
  if (loan.description) c.append(row(T("loan.note"), loan.description));
  $("ln-newbal-unit").textContent = cur;
}

function fill() {
  $("le-name").value = loan.name;
  $("le-desc").value = loan.description || "";
  $("le-rate").value = loan.rate_percent != null ? String(loan.rate_percent) : "";
  $("le-day").value = loan.payment_day != null ? String(loan.payment_day) : "";
  $("le-start").value = loan.start || "";
  $("le-maturity").value = loan.maturity || "";
  ($("le-" + (loan.method === "declining" ? "declining" : "annuity"))).checked = true;
  $("le-prepay").value = loan.prepay_effect || "";
  for (const id of ["e-le-name", "e-le-rate", "e-le-day", "e-le-maturity"]) $(id).textContent = "";
}

// patch sends the whole contract with one thing changed. The server files
// a new version; a partial patch would be read as the old rename shape.
async function patch(body, okKey) {
  if (busy) return false;
  busy = true;
  try {
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "PATCH", body: JSON.stringify(body) });
    if (!res.ok) throw new Error(String(res.status));
    haptic.ok();
    invalidate("api/");
    toast(T(okKey));
    return true;
  } catch {
    haptic.bad();
    toast(T("err.save"));
    return false;
  } finally {
    busy = false;
  }
}

const terms = () => ({
  name: loan.name, description: loan.description || "",
  rate_percent: loan.rate_percent, payment_day: loan.payment_day,
  start_date: loan.start, maturity_date: loan.maturity,
  method: loan.method || "annuity", prepay_effect: loan.prepay_effect || "",
});

async function saveBalance(e) {
  e.preventDefault();
  const raw = $("ln-newbal").value.trim(), v = moneyNum(raw);
  const err = !raw ? T("err.required") : Number.isNaN(v) ? T("err.number") : v <= 0 ? T("err.positive") : "";
  $("e-newbal").textContent = err;
  $("ln-newbal").setAttribute("aria-invalid", err ? "true" : "false");
  if (err) { haptic.bad(); return; }
  if (await patch({ ...terms(), balance_major: v }, "saved")) { await load(loan.id); mode("view"); }
}

async function saveEdit(e) {
  e.preventDefault();
  const name = $("le-name").value.trim();
  const rate = num($("le-rate").value), day = num($("le-day").value);
  const start = $("le-start").value, maturity = $("le-maturity").value;
  const errs = {
    name: name ? "" : T("err.required"),
    rate: Number.isNaN(rate) || rate < 0 || rate > 200 ? T("err.rate") : "",
    day: !Number.isInteger(day) || day < 1 || day > 31 ? T("err.day") : "",
    maturity: !start || !maturity ? T("err.required") : maturity <= start ? T("err.order") : "",
  };
  for (const [k, v] of Object.entries(errs)) {
    $("e-le-" + k).textContent = v;
    $("le-" + k).setAttribute("aria-invalid", v ? "true" : "false");
  }
  if (Object.values(errs).some(Boolean)) { haptic.bad(); return; }
  const body = {
    name, description: $("le-desc").value.trim(), rate_percent: rate, payment_day: day,
    start_date: start, maturity_date: maturity,
    method: document.querySelector('input[name="le-method"]:checked')?.value || "annuity",
    prepay_effect: $("le-prepay").value, balance_major: 0,
  };
  if (await patch(body, "saved")) { await load(loan.id); mode("view"); }
}

async function remove() {
  haptic.tap();
  if (!(await confirmDialog(T("manage.confirm")))) return;
  try {
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "DELETE" });
    if (!res.ok) throw new Error(String(res.status));
    haptic.ok(); invalidate("api/"); go("loans");
  } catch {
    haptic.bad(); toast(T("err.remove"));
  }
}

async function load(id) {
  $("loan-missing").hidden = true;
  $("loan-loading").hidden = !!loan;
  try {
    await getJSON("api/loans", (body) => {
      const found = (body.loans || []).find((l) => l.id === id);
      if (!found) { loan = null; return; }
      loan = found;
      render();
      setTitle(loan.name);
    });
  } catch { /* the cached copy, if any, already rendered */ }
  $("loan-loading").hidden = true;
  if (!loan) { $("loan-view").hidden = true; $("loan-missing").hidden = false; setAction(null); }
}

register({
  id: "loan",
  parent: "loans",
  titleKey: "manage.title",
  html: HTML,
  onMount() {
    group($("ln-newbal"));
    $("ln-update").addEventListener("click", () => { haptic.tap(); $("ln-newbal").value = ""; $("e-newbal").textContent = ""; mode("balance"); $("ln-newbal").focus({ preventScroll: true }); });
    $("ln-bal-cancel").addEventListener("click", () => { haptic.tap(); mode("view"); });
    $("ln-edit-cancel").addEventListener("click", () => { haptic.tap(); mode("view"); });
    $("loan-balance").addEventListener("submit", saveBalance);
    $("loan-edit").addEventListener("submit", saveEdit);
    $("ln-remove").addEventListener("click", remove);
  },
  async onShow(_, params) {
    loan = null;
    $("loan-view").hidden = true;
    mode("view");
    await load(params?.id);
    if (loan) mode("view");
  },
});
