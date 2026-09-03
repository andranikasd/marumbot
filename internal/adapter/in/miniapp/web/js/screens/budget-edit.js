"use strict";
import { haptic, toast, fmtMoney } from "../core.js";
import { T, sub, addStrings } from "../i18n.js";
import { api, invalidate } from "../api.js";
import { register, go, currentScreen } from "../nav.js";
import { fundingHTML, createFunding, majorAmount, validMonth, validDate, minorText } from "./budget-funding.js";

addStrings({
  "budget.editing": "Խմբագրել բյուջեն", "budget.save": "Պահպանել բյուջեն",
  "be.policyActive": "Բյուջեն ունի հաստատված կանոններ։ Սահմանաչափը փոխեք Բյուջեի կանոններ բաժնում։ Գումարի և ծախսերի նոր քաղվածքը ներկայացրեք համադրման միջոցով։",
  "be.budget": "Բյուջե", "be.funding": "Գումար", "be.months": "Ամիսներ",
  "be.permission": "Ամսական ծախսի սահմանաչափ",
  "be.permissionHint": "Վարկերի համար ամսական թույլատրելի ծախսը։ Հասանելի գումարը նշեք Գումար բաժնում։",
  "be.retry": "Կրկնել պահպանումը", "be.uncertain": "Պահպանման արդյունքը հայտնի չէ։ Կրկնեք նույն հարցումը՝ նախքան փոփոխելը։",
  "be.reload": "Վերբեռնել՝ չպահպանված փոփոխությունները հեռացնելով",
  "be.conflict": "Բյուջեն այլ տեղ փոփոխվել է։ Ձեր մուտքագրածը պահպանվել է այս ձևում։ Վերբեռնեք վերջին տարբերակը՝ փոփոխությունները նորից կատարելու համար։",
  "be.load": "Չհաջողվեց բեռնել բյուջեի անվտանգ, արդիական տարբերակը։ Վերբեռնեք՝ շարունակելու համար։",
  "be.rejected": "Տվյալները չեն ընդունվել։ Ստուգեք գումարները, ամսաթվերը և սահմանները։",
}, {
  "budget.editing": "Edit budget", "budget.save": "Save budget",
  "be.policyActive": "Approved rules govern this budget. Change permission in Budget rules. Update cash and spending statements through reconciliation.",
  "be.budget": "Budget", "be.funding": "Money", "be.months": "Months",
  "be.permission": "Monthly spending limit",
  "be.permissionHint": "How much may be spent on loans each month. Tell us when your money is available in Money.",
  "be.retry": "Retry save", "be.uncertain": "The save outcome is unknown. Retry the same request before editing.",
  "be.reload": "Reload and discard unsaved changes",
  "be.conflict": "The budget changed elsewhere. Your entries are still here. Reload the latest version before making your changes again.",
  "be.load": "Could not load a safe, current budget. Reload before continuing.",
  "be.rejected": "The values were not accepted. Check amounts, dates and limits.",
});

import { budgetHelpHTML } from "./budget-help.js";

const HTML = `
  ${budgetHelpHTML}
  <form id="budget-form" novalidate class="stack">
    <p id="budget-status" class="error" role="alert"></p>
    <button type="button" class="alink" data-go="budget-policy" data-i18n="bp.title"></button>
    <button type="button" id="budget-reload" class="alink" data-i18n="be.reload" hidden></button>
    <button type="button" id="budget-save-retry" class="cta" data-i18n="be.retry" hidden></button>
    <fieldset id="budget-fields" class="stack" style="border:0;padding:0;margin:0;min-width:0" disabled>
    <div class="chart-controls" role="tablist" aria-label="Budget">
      <button type="button" id="budget-tab-budget" role="tab" aria-controls="budget-panel-budget" aria-selected="true" aria-pressed="true" data-section="budget" data-i18n="be.budget"></button>
      <button type="button" id="budget-tab-funding" role="tab" aria-controls="budget-panel-funding" aria-selected="false" aria-pressed="false" tabindex="-1" data-section="funding" data-i18n="be.funding"></button>
      <button type="button" id="budget-tab-months" role="tab" aria-controls="budget-panel-months" aria-selected="false" aria-pressed="false" tabindex="-1" data-section="months" data-i18n="be.months"></button>
    </div>
    <div id="budget-panel-budget" role="tabpanel" aria-labelledby="budget-tab-budget" class="stack">
    <div class="card stack">
      <div class="field">
        <label for="monthly" data-i18n="be.permission">Սովորական ամսվա գումար</label>
        <div class="row">
          <input id="monthly" name="monthly" inputmode="decimal" placeholder="0" required>
          <select id="budget-currency" name="budget-currency" class="narrow">
            <option value="AMD" selected>AMD</option>
            <option value="USD">USD</option>
            <option value="EUR">EUR</option>
            <option value="RUB">RUB</option>
          </select>
        </div>
        <p class="hint" data-i18n="be.permissionHint"></p>
        <p class="error" id="e-monthly"></p>
        <p class="hint" id="b-note" hidden></p>
      </div>
      <div class="field">
        <label for="payday" data-i18n="budget.payday">Երբ է գումարը հասանելի</label>
        <div class="in unit-w"><input id="payday" name="payday" inputmode="numeric" placeholder="1–31" required><span class="unit" data-i18n="unit.day">ամսի օր</span></div>
        <p class="hint" data-i18n="budget.payday.hint"></p>
        <p class="error" id="e-payday"></p>
      </div>
    </div>

    </div>
    <div id="budget-panel-funding" role="tabpanel" aria-labelledby="budget-tab-funding" class="stack" hidden>
    <div class="card stack">${fundingHTML}</div>
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

    </div>
    <div id="budget-panel-months" role="tabpanel" aria-labelledby="budget-tab-months" class="stack" hidden>
    <div class="card stack">
      <div class="field">
        <span class="lbl" data-i18n="budget.months">Տարբեր ամիսներ</span>
        <p class="hint" style="margin:0 0 8px" data-i18n="budget.months.hint"></p>
        <div id="override-list" class="stack" style="gap:8px"></div>
        <p class="error" id="e-overrides"></p>
      </div>
      <button class="alink" type="button" id="override-add" data-i18n="budget.months.add">Ավելացնել ամիս</button>
    </div>

    </div>
    <button class="cta" type="submit" id="budget-save" data-i18n="budget.save">Պահպանել բյուջեն</button>
    </fieldset>
  </form>
`;

const $ = (id) => document.getElementById(id);
let required = null, funding, policyMode = false;
let busy = false, loading = false, loaded = false, dirty = false, conflict = false;
let pending = null;
let version = null, revision = 0, requestID = 0, rowID = 0;
let loadedCurrency = "AMD", loadedExponent = 2, loadedAsOf = null;
const currencies = new Set(["AMD", "USD", "EUR", "RUB"]);
const exponent = () => $("budget-currency").value === loadedCurrency ? loadedExponent : 2;

function showSection(section, focus = false) {
  for (const button of $("budget-form").querySelectorAll("[data-section]")) {
    const active = button.dataset.section === section;
    button.setAttribute("aria-selected", String(active));
    button.setAttribute("aria-pressed", String(active));
    button.tabIndex = active ? 0 : -1;
    $("budget-panel-" + button.dataset.section).hidden = !active;
    if (active && focus) button.focus();
  }
}
function controls() {
  $("budget-fields").disabled = busy || loading || !!pending || !loaded;
  $("budget-save").disabled = busy || loading || !!pending || !loaded || conflict;
  $("budget-reload").disabled = busy || loading || !!pending;
  $("budget-save-retry").hidden = !pending;
  $("budget-save-retry").disabled = busy || loading;
  $("budget-form").setAttribute("aria-busy", String(busy || loading));
}
function changed() { if(pending||busy)return; dirty = true; revision++; validate(); }
function errorAt(id, message) {
  $("e-" + id).textContent = message;
  $(id).setAttribute("aria-invalid", String(!!message));
}
function overrideRow(month = "", amount = "") {
  const row = document.createElement("div"); row.className = "row override";
  const m = document.createElement("input"); m.type = "month"; m.className = "o-month"; m.value = month;
  const a = document.createElement("input"); a.inputMode = "decimal"; a.className = "o-amount"; a.placeholder = "0"; a.value = String(amount);
  m.setAttribute("aria-label", T("be.months")); a.setAttribute("aria-label", T("be.permission"));
  const error = document.createElement("p"); error.className = "error"; error.id = "override-error-" + rowID++;
  m.setAttribute("aria-describedby", error.id); a.setAttribute("aria-describedby", error.id);
  const del = document.createElement("button"); del.type = "button"; del.className = "alink quiet";
  del.style.width = "44px"; del.style.flexShrink = "0"; del.textContent = "✕";
  del.setAttribute("aria-label", T("budget.months.remove"));
  const wrapper = document.createElement("div"); wrapper.append(row, error);
  del.onclick = () => { haptic.tap(); wrapper.remove(); changed(); };
  row.append(m, a, del); return wrapper;
}
function readOverrides() {
  const out = {}, seen = new Set(); let ok = true;
  const rows = [...$("override-list").children];
  $("e-overrides").textContent = rows.length > 36 ? T("budget.months.limit") : "";
  if (rows.length > 36) ok = false;
  for (const row of rows) {
    const m = row.querySelector(".o-month"), a = row.querySelector(".o-amount");
    let error = "", amount;
    const badMonth = !validMonth(m.value) || seen.has(m.value);
    if (badMonth) error = T(seen.has(m.value) ? "budget.months.duplicate" : "budget.months.bad");
    seen.add(m.value);
    let badAmount = false;
    try { amount = majorAmount(a.value, exponent()); } catch (e) { error ||= e.message; badAmount = true; }
    m.setAttribute("aria-invalid", String(badMonth)); a.setAttribute("aria-invalid", String(badAmount));
    row.querySelector(".error").textContent = error;
    if (error) ok = false; else out[m.value] = amount;
  }
  return { value: out, ok };
}

// Validate the full response before changing any field; never partially load a document.
function checkDocument(b) {
  if (!b || typeof b !== "object" || Array.isArray(b)) throw new Error("budget");
  if(!validDate(b.today))throw new Error("statement date");
  const existing = b.monthly_major != null;
  const v = b.version ?? (existing ? null : 0);
  if (!Number.isSafeInteger(v) || v < 0) throw new Error("version");
  const currency = b.currency ?? "AMD", exp = b.currency_exponent ?? (existing ? null : 2);
  if (!currencies.has(currency) || !Number.isInteger(exp) || exp < 0 || exp > 6) throw new Error("currency");
  const checkMajor = (value) => {
    if (typeof value !== "number" || !Number.isFinite(value)) throw new Error("amount");
    majorAmount(String(value), exp);
  };
  if (existing) checkMajor(b.monthly_major);
  if (b.base_monthly_major != null) checkMajor(b.base_monthly_major);
  for (const key of ["opening_major", "reserve_major", "required_major"]) if (b[key] != null) checkMajor(b[key]);
  if (b.pay_day != null && (!Number.isInteger(b.pay_day) || b.pay_day < 0 || b.pay_day > 31)) throw new Error("payday");
  if (b.overrides != null && (typeof b.overrides !== "object" || Array.isArray(b.overrides))) throw new Error("months");
  const overrides = Object.entries(b.overrides || {});
  if (overrides.length > 36) throw new Error("months");
  for (const [month, amount] of overrides) { if (!validMonth(month)) throw new Error("month"); checkMajor(amount); }
  if (b.funding != null) {
    const f = b.funding;
    minorText(f.monthly_minor, exp); minorText(f.spent_minor, exp);
    if (f.events != null && !Array.isArray(f.events)) throw new Error("events");
    if ((f.events || []).length > 36) throw new Error("events");
    for (const event of f.events || []) {
      if (!event || !validDate(event.on) || typeof event.expected !== "boolean") throw new Error("event");
      minorText(event.minor, exp); if (event.minor === 0) throw new Error("event amount");
    }
  }
  return { version: v, currency, exponent: exp };
}
async function load(discard = false) {
  if (pending) {controls();return;}
  if (busy || loading || (dirty && !discard) || (conflict && !discard)) return;
  const id = ++requestID, startRevision = revision;
  loading = true; controls();
  $("budget-status").textContent = T("loading");
  try {
    const res = await api("api/budget", { cache: "no-store" });
    if (!res.ok) throw new Error("load");
    const b = await res.json(), meta = checkDocument(b);
    policyMode = false;
    let today = b.today, targets = [];
    if (b.monthly_major != null) {
      const [policies, loans] = await Promise.all([
        api("api/budget/policies", { cache: "no-store" }), api("api/loans", { cache: "no-store" }),
      ]);
      if (!policies.ok || !loans.ok) throw new Error("funding context");
      const policyDocument = await policies.json();
      targets = (await loans.json()).loans;
      if (!Array.isArray(targets)) throw new Error("loans");
      policyMode = (policyDocument.policies || []).length > 0;
      today = policyDocument.today;
    }
    if (id !== requestID || currentScreen() !== "budget-edit") return;
    if (revision !== startRevision) {
      $("budget-status").textContent = T("be.load"); $("budget-reload").hidden = false;
      return;
    }
    loadedCurrency = meta.currency; loadedExponent = meta.exponent; loadedAsOf = b.today;
    $("budget-currency").value = meta.currency;
    $("monthly").value = b.monthly_major == null ? "" : String(b.base_monthly_major ?? b.monthly_major);
    $("payday").value = b.pay_day > 0 ? String(b.pay_day) : "";
    $("opening").value = String(b.opening_major ?? 0); $("reserve").value = String(b.reserve_major ?? 0);
    $("override-list").replaceChildren(...Object.entries(b.overrides || {}).sort().map(([month, amount]) => overrideRow(month, amount)));
    funding.load(b.funding, { today, loans: targets.filter(l => l.currency === meta.currency && !l.optional_excluded && l.balance_major > 0), locked: policyMode });
    for (const id of ["monthly", "opening", "reserve"]) $(id).readOnly = policyMode;
    $("budget-currency").disabled = policyMode;
    $("budget-panel-months").querySelectorAll("input,button").forEach(el => { el.disabled = policyMode; });
    required = b.required_major != null ? { major: b.required_major, currency: b.currency } : null;
    version = meta.version; loaded = true; dirty = false; conflict = false;
    $("budget-status").textContent = policyMode ? T("be.policyActive") : ""; $("budget-reload").hidden = true;
    validate();
  } catch {
    if (id === requestID) {
      loaded = false;
      $("budget-status").textContent = T("be.load"); $("budget-reload").hidden = false;
    }
  } finally {
    if (id === requestID) { loading = false; controls(); }
  }
}
function validate() {
  if (!loaded) return { ok: false };
  const values = {}; let ok = true;
  for (const [id, positive] of [["monthly", true], ["opening", false], ["reserve", false]]) {
    let message = "";
    try { values[id] = majorAmount($(id).value, exponent(), { positive, blankZero: !positive }); }
    catch (e) { message = e.message; ok = false; }
    errorAt(id, message);
  }
  const day = $("payday").value.trim();
  const pd = Number(day), pdError = !/^\d{1,2}$/.test(day) || pd < 1 || pd > 31;
  errorAt("payday", pdError ? T(day ? "err.day" : "err.required") : "");
  if (pdError) ok = false;
  if ($("funding-mode").value === "legacy" && values.reserve > values.opening) { errorAt("reserve", T("budget.reserve.too_high")); ok = false; }
  const cur = $("budget-currency").value;
  if (!currencies.has(cur)) ok = false;
  $("u-opening").textContent = cur; $("u-reserve").textContent = cur;
  // These are entered cash and reserve, not a new projection.
  $("b-usable-row").hidden = values.opening == null || values.reserve == null || values.reserve > values.opening;
  if (!$("b-usable-row").hidden) $("b-usable").textContent = fmtMoney(values.opening - values.reserve, cur);
  const note = $("b-note"); note.hidden = !(required && required.currency === cur && values.monthly != null);
  if (!note.hidden) {
    const diff = values.monthly - required.major;
    note.textContent = sub("budget.against", { r: fmtMoney(required.major, cur) }) + " " + T(diff < 0 ? "budget.low" : diff === 0 ? "budget.exact" : "budget.ok");
    note.style.color = diff < 0 ? "var(--danger)" : "";
  }
  const overrides = readOverrides(), f = funding.read();
  $("override-add").disabled = policyMode || $("override-list").children.length >= 36;
  return { ok: ok && overrides.ok && f.ok, body: {
    monthly_major: values.monthly, currency: cur, pay_day: pd,
    opening_major: values.opening, reserve_major: values.reserve,
    overrides: overrides.value, funding: f.value, expected_version: version, as_of: loadedAsOf,
  } };
}
async function save(e) {
  e.preventDefault();
  if (busy || loading || !loaded || conflict) return;
  if (!pending) {
    const result = validate();
    if (!result.ok) {
      haptic.bad();
      const invalid = [...$("budget-form").querySelectorAll('[aria-invalid="true"]')].find(el => !el.closest("#funding-separate")?.hidden);
      if (invalid) { showSection(invalid.closest('[role="tabpanel"]').id.replace("budget-panel-", "")); invalid.focus(); }
      return;
    }
    const body = policyMode ? {
      currency: result.body.currency, expected_version: version, pay_day: result.body.pay_day,
      monthly_minor: result.body.funding.monthly_minor, events: result.body.funding.events,
    } : result.body;
    // Freeze the endpoint, aggregate version, values and key together. A retry
    // must not consult changed form fields or select a different save route.
    pending = {path: policyMode ? "api/budget/funding" : "api/budget",
      body: JSON.stringify({...body, idempotency_key: crypto.randomUUID()})};
  }
  busy = true; controls(); $("budget-status").textContent = "";
  try {
    const res = await api(pending.path, {method:"POST", body:pending.body});
    if (!res.ok) {
      if (res.status >= 400 && res.status < 500) {
        pending = null; dirty = true;
        conflict = res.status === 409;
        $("budget-status").textContent = T(conflict ? "be.conflict" : "be.rejected");
        $("budget-reload").hidden = false;
        haptic.bad(); return;
      }
      throw new Error("uncertain save");
    }
    pending = null; dirty = false; loaded = false; version = null;
    invalidate("api/"); haptic.ok(); toast(T("saved"));
    if (currentScreen() === "budget-edit") go("budget");
  } catch { haptic.bad(); $("budget-status").textContent = T("be.uncertain"); }
  finally { busy = false; controls(); }
}
register({
  id: "budget-edit", parent: "budget", titleKey: "budget.editing", html: HTML,
  onMount() {
    const form = $("budget-form"); funding = createFunding(form, changed, exponent);
    form.querySelector('[role="tablist"]').setAttribute("aria-label", T("budget.editing"));
    for (const id of ["monthly", "payday", "opening", "reserve"]) $(id).setAttribute("aria-describedby", "e-" + id);
    form.addEventListener("input", changed); form.addEventListener("change", changed);
    const tabs = [...form.querySelectorAll("[data-section]")];
    tabs.forEach((button, index) => {
      button.onclick = () => showSection(button.dataset.section);
      button.onkeydown = (e) => {
        let target;
        if (e.key === "ArrowRight") target = (index + 1) % tabs.length;
        if (e.key === "ArrowLeft") target = (index + tabs.length - 1) % tabs.length;
        if (e.key === "Home") target = 0;
        if (e.key === "End") target = tabs.length - 1;
        if (target != null) { e.preventDefault(); showSection(tabs[target].dataset.section, true); }
      };
    });
    $("override-add").onclick = () => {
      if ($("override-list").children.length >= 36) return;
      const row = overrideRow(); $("override-list").append(row); changed(); row.querySelector("input").focus();
    };
    $("budget-reload").onclick = () => load(true);
    $("budget-save-retry").onclick = save;
    form.addEventListener("submit", save);
  },
  onShow(_el, params) { if (params?.section === "funding") showSection("funding"); return load(); },
});
