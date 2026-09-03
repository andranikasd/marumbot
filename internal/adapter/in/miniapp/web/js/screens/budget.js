// The budget, read. One hero — the monthly amount and whether it covers
// what the loans require — and one list of everything else the plan uses:
// payday, cash on hand, the untouched reserve, the stated months. Editing
// is a sub-screen reached from the app bar.
"use strict";
import { haptic, fmtMoney, fmtMonth, fmtFull } from "../core.js";
import { T, sub, addStrings } from "../i18n.js";
import { getJSON } from "../api.js";
import { register, go, setAction, currentScreen } from "../nav.js";

addStrings({'budget.cashdate':'Գումարի ամսաթիվը','budget.cashdate.unknown':'Ամսաթիվը նշված չէ'},{'budget.cashdate':'Cash as of','budget.cashdate.unknown':'Date not supplied'});
import { minorText } from "./budget-funding.js";
import { budgetHelpHTML } from "./budget-help.js";
addStrings({"budget.moneyMonthly":"Վարկերի ամսական գումար"},{"budget.moneyMonthly":"Money available each month"});
addStrings({"budget.limitOnly":"Սահմանում մնացող տեղը հասանելի կանխիկ գումար չէ։ Պլանը ստուգում է նաև գումարի մուտքի օրը։"},{"budget.limitOnly":"Room in your limit is not cash available. The plan also checks when money arrives."});
const HTML = `
  <div id="budget-view" class="stack" hidden>
    <div class="hero">
      <div class="k"><span data-i18n="budget.monthly">Ամսական բյուջե</span><span class="pill" id="bo-state"></span></div>
      <div class="v num" id="bo-monthly">—</div>
      <div class="sub" id="bo-status"></div>
      <div class="kv">
        <div><span data-i18n="budget.required">Պարտադիր վճարումներ</span><b class="num" id="bo-required">—</b></div>
        <div><span data-i18n="budget.safe_extra">Անվտանգ հավելյալ</span><b class="num gold" id="bo-extra">—</b></div>
      </div>
    </div>
    <div class="card kv" id="bo-facts"></div>
    <button class="cta" type="button" data-go="budget-edit" data-i18n="budget.edit"></button>
    <details class="fold"><summary data-i18n="bp.title"></summary><div class="fold-body">
    <button class="alink" type="button" data-go="budget-policy" data-i18n="bp.title"></button>
    </div></details>
    ${budgetHelpHTML}
    <p class="hint" data-i18n="budget.limitOnly"></p>
    <button class="cta ghost" type="button" data-go="plan" data-i18n="budget.plan">Տեսնել պլանը</button>
  </div>
  <div id="budget-loading" hidden><div class="skel hero-skel"></div><div class="skel" style="margin-top:12px"></div></div>
  <div class="state" id="budget-none" hidden>
    <div class="tile lg">֏</div>
    <b data-i18n="budget.none.title">Բյուջե դեռ չկա</b>
    <span data-i18n="budget.none">Նշեք՝ որքան կարող եք ամսական հատկացնել վարկերին։</span>
    <button class="cta" type="button" data-go="budget-edit" data-i18n="budget.set">Նշել բյուջեն</button>
  </div>
  <div class="state" id="budget-error" hidden>
    <b data-i18n="err.load">Չհաջողվեց բեռնել։</b>
    <button class="alink" type="button" id="budget-retry" data-i18n="retry">Փորձել նորից</button>
  </div>
`;

const $ = (id) => document.getElementById(id);
let known = false, displayedBudget = null;

const row = (label, value, cls) => {
  const d = document.createElement("div");
  const s = document.createElement("span"); s.textContent = label;
  const b = document.createElement("b"); b.textContent = value; if (cls) b.className = cls;
  d.append(s, b);
  return d;
};

function render(b) {
  displayedBudget = b;
  const set = b.monthly_major != null;
  $("budget-view").hidden = !set;
  $("budget-none").hidden = set;
  if(currentScreen()==="budget")setAction(set ? T("budget.edit") : null, () => go("budget-edit"));
  if (!set) return;
  const cur = b.currency;
  const monthly = b.monthly_major;
  const required = b.required_major;
  $("bo-monthly").textContent = fmtMoney(monthly, cur);
  const surplus = required == null ? null : monthly - required;
  const state = $("bo-state");
  if (surplus == null) { state.textContent = ""; state.className = "pill"; $("bo-status").textContent = ""; }
  else if (surplus < 0) { state.textContent = T("budget.short"); state.className = "pill bad"; $("bo-status").textContent = T("budget.not_covered"); }
  else { state.textContent = T("budget.ok.pill"); state.className = "pill"; $("bo-status").textContent = T(surplus === 0 ? "budget.exact" : "budget.covered"); }
  $("bo-required").textContent = required == null ? "—" : fmtMoney(required, cur);
  $("bo-extra").textContent = surplus == null ? "—" : fmtMoney(Math.max(0, surplus), cur);

  const facts = $("bo-facts"); facts.textContent = "";
  let monthlyMoney = T("budget.unset");
  if (b.funding) {
    const [whole, fraction = ""] = minorText(b.funding.monthly_minor, b.currency_exponent).split(".");
    const decimals = fraction.replace(/0+$/, "");
    monthlyMoney = cur + " " + whole.replace(/\B(?=(\d{3})+(?!\d))/g, " ") + (decimals ? "." + decimals : "");
  }
  facts.append(row(T("budget.moneyMonthly"), monthlyMoney, b.funding ? "num" : "mute"));
  facts.append(row(T("budget.payday"), b.pay_day > 0 ? sub("budget.payday.day", { d: b.pay_day }) : T("budget.unset"), b.pay_day > 0 ? "" : "mute"));
  const opening = b.opening_major || 0, reserve = b.reserve_major || 0;
  const declared = !!b.opening_as_of || opening > 0;
  facts.append(row(T("budget.opening"), declared ? fmtMoney(opening, cur) : T("budget.unset"), declared ? "num" : "mute"));
  if(declared) facts.append(row(T("budget.cashdate"), b.opening_as_of ? fmtFull(b.opening_as_of) : T("budget.cashdate.unknown"), "mute"));
  if (reserve > 0) facts.append(row(T("budget.reserve"), fmtMoney(reserve, cur), "num"));
  if (opening > 0) facts.append(row(T("budget.usable.label"), fmtMoney(Math.max(0, opening - reserve), cur), "num ok"));
  const months = Object.entries(b.overrides || {}).sort();
  if (months.length === 1) facts.append(row(T("budget.months"), fmtMonth(months[0][0] + "-01") + " · " + fmtMoney(months[0][1], cur), "num"));
  else if (months.length > 1) facts.append(row(T("budget.months"), sub("budget.months.n", { n: months.length })));
}

async function load() {
  $("budget-error").hidden = true;
  $("budget-loading").hidden = known;
  try {
    await getJSON("api/budget", (b) => { known = true; render(b); });
  } catch {
    if (!known) { $("budget-view").hidden = true; $("budget-none").hidden = true; $("budget-error").hidden = false; }
  } finally {
    $("budget-loading").hidden = true;
  }
}

register({
  id: "budget",
  parent:"home",
  icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="6" width="18" height="13" rx="2"/><path d="M3 10h18"/></svg>',
  labelKey: "tab.budget",
  titleKey: "budget.title",
  html: HTML,
  onMount() {
    $("budget-retry").addEventListener("click", () => { haptic.tap(); load(); });
  },
  onShow() { load(); },
  // Relabel the already displayed snapshot without claiming a fresh read.
  onLanguage() { if (displayedBudget) render(displayedBudget); },
});
