// The plan. The answer comes first — the debt-free date, the monthly
// amount, the saving — then the strategy as three rows with their
// consequence, then this month's payments, then the milestones. The
// month-by-month sheet is one tap away, not the default reading: most
// months of a repayment are the same month repeated.
"use strict";
import "./plan-stress.js";
import {planEvidence} from "./plan-evidence.js";
import {chartHTML,drawChart} from "./plan-chart.js";
import { haptic, toast, fmtMoney, fmtDate, fmtMonth, fmtFull, esc } from "../core.js";
import { T, sub, addStrings } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register,go } from "../nav.js";

addStrings({"plan.overview":"Ընդհանուր","plan.payments":"Վճարումներ","plan.milestones":"Նպատակներ","plan.strategy":"Մեթոդ","plan.details":"Հաշվարկի մանրամասներ","plan.back":"Վերադառնալ պլանին"},{"plan.overview":"Overview","plan.payments":"Payments","plan.milestones":"Milestones","plan.strategy":"Strategy","plan.details":"Calculation details","plan.back":"Back to plan"});
addStrings({"plan.no_growth":"Առանց բյուջեի աճի","plan.no_growth_blocked":"Առանց նախատեսված աճի պլանը չի կատարվում"},{"plan.no_growth":"Without budget growth","plan.no_growth_blocked":"This plan cannot be funded without the assumed growth"});
addStrings({"plan.export":"Ներբեռնել վճարումների CSV"},{"plan.export":"Export dated payments CSV"});
addStrings({"evidence.proven_optimal":"Ծախսի նվազագույնը հաստատված է","evidence.exhaustive_static_order":"Համեմատվել են բոլոր հաստատուն հերթականությունները","evidence.bounded_heuristic":"Լավագույնը ստուգված տարբերակներից","evidence.named_strategies_only":"Ընտրված մեթոդի հաշվարկ","evidence.attempts":"Ստուգված տարբերակներ","evidence.gap":"Տարբերությունը նվազագույնից","evidence.unknown":"Հաստատված չէ"},{"evidence.proven_optimal":"Minimum cost proven","evidence.exhaustive_static_order":"All fixed orders compared","evidence.bounded_heuristic":"Best of the tested options","evidence.named_strategies_only":"Selected method calculation","evidence.attempts":"Options tested","evidence.gap":"Gap from minimum","evidence.unknown":"Not established"});
addStrings({'plan.funding':'Նշեք հասանելի գումարը','plan.funding.why':'Ծախսման սահմանը հասանելի գումար չէ։ Նշեք հաստատված ամսական ֆինանսավորումը՝ ներառյալ զրոն, և այժմ հասանելի գումարը։','plan.funding.fix':'Նշել ֆինանսավորումը'}, {'plan.funding':'Declare available cash','plan.funding.why':'Your spending limit is not cash. Enter confirmed monthly funding, including zero, and cash available now.','plan.funding.fix':'Declare funding'});
addStrings({'plan.cash_stale':'Ստուգեք պահված գումարը','plan.cash_stale.why':'Ուղղորդված մուտքը նախորդ օրվանից է։ Հաստատեք դեռ պահված գումարը կամ հստակ հեռացրեք սահմանափակումը։ Հին մուտքը մի հաշվեք որպես նոր գումար։'},{'plan.cash_stale':'Review retained cash','plan.cash_stale.why':'A routed receipt is from an earlier day. Confirm the amount still held, or explicitly remove its restriction. Do not count an old receipt as new money.'});
const HTML = `
 <div id="plan-loading" hidden><p class="hint" role="status" data-i18n="loading"></p><div class="skel hero-skel" aria-hidden="true"></div></div>
 <div id="plan-blocked" class="card stack" hidden><b id="plan-blocked-title"></b><p id="plan-blocked-why"></p><button class="cta" id="plan-blocked-fix" type="button"></button></div>
 <div class="state" id="plan-empty" hidden><b data-i18n="plan.empty.title"></b><p data-i18n="plan.empty"></p><button class="cta" data-go="add" data-i18n="plan.empty.add"></button></div>
 <div id="plan-body" class="stack" hidden>
  <div id="plan-overview" class="stack"><p id="pl-outdated" class="hint" data-i18n="history.old" hidden></p>
   <div class="hero"><div class="k"><span data-i18n="plan.debtfree"></span><span class="pill" id="pl-approved" hidden data-i18n="plan.approved_badge"></span></div><div class="v" id="pl-date"></div><div class="sub" id="pl-sub"></div><div class="kv"><div id="pl-save-row" hidden><span data-i18n="plan.saved"></span><b class="num" id="pl-save"></b></div></div></div>
   ${chartHTML}
   <div class="pair"><button class="cta ghost" data-layer="payments" data-i18n="plan.payments"></button><button class="cta ghost" data-layer="milestones" data-i18n="plan.milestones"></button></div>
   <div class="pair"><button class="alink" data-layer="strategy" data-i18n="plan.strategy"></button><button class="alink" data-layer="details" data-i18n="plan.details"></button></div>
   <button class="cta" type="button" id="pl-approve" data-i18n="plan.approve" hidden></button>
  </div>
  <div id="plan-layer" class="stack" hidden>
   <button class="alink" type="button" id="plan-layer-back" data-i18n="plan.back"></button>
   <section data-plan-layer="payments" hidden><div class="card"><p class="sec" id="pl-month-lbl"></p><div class="stack-bar" id="pl-stack" hidden><i class="req" id="pl-stack-req"></i><i class="extra" id="pl-stack-extra"></i></div><div class="kv" id="pl-month-rows"></div></div><button class="alink" id="pl-all" hidden></button><ol class="tl" id="pl-list" hidden></ol></section>
   <section data-plan-layer="milestones" hidden><ol class="ms" id="pl-ms"></ol></section>
   <section data-plan-layer="strategy" id="plan-goals" class="card" hidden><button class="alink" id="pl-scenarios" data-i18n="sc.title"></button><div class="pair"><button class="alink" data-go="plan-comparison" data-i18n="compare.title"></button><button class="alink" data-go="plan-inverse" data-i18n="inverse.title"></button></div>
    <button class="opt" type="button" data-goal="cheapest"><i></i><span><span class="l1"><b data-i18n="plan.g.cheapest"></b><em></em></span><small data-i18n="plan.g.cheapest.desc"></small></span></button>
    <button class="opt" type="button" data-goal="soonest"><i></i><span><span class="l1"><b data-i18n="plan.g.soonest"></b><em></em></span><small data-i18n="plan.g.soonest.desc"></small></span></button>
    <button class="opt" type="button" data-goal="first"><i></i><span><span class="l1"><b data-i18n="plan.g.first"></b><em></em></span><small data-i18n="plan.g.first.desc"></small></span></button>
   </section>
   <section data-plan-layer="details" class="card kv" hidden><div id="pl-sooner-row" hidden><span data-i18n="plan.sooner"></span><b id="pl-sooner"></b></div><div><span data-i18n="plan.interest"></span><b id="pl-cost"></b></div><button class="alink" data-go="plan-history" data-i18n="history.title"></button><button id="pl-stress" class="alink" data-i18n="stress.title"></button><button id="pl-export" class="alink" data-i18n="plan.export"></button><div id="pl-evidence"></div><div id="pl-certificate"></div><div id="pl-no-growth" hidden></div><div class="track" hidden><i id="pl-bar"></i></div><span id="pl-finish-short" hidden></span><button class="alink" data-go="budget-edit" data-i18n="budget.editing"></button></section>
  </div>
 </div>
 <div class="state" id="plan-error" hidden><b data-i18n="plan.error"></b><button class="alink" id="plan-retry" data-i18n="retry"></button></div>
`;

const $ = (id) => document.getElementById(id);
let goal = new URLSearchParams(location.search).get("goal") || "";
let loadVersion = 0;
let displayedPlan=null, approvalRequest=null;
const goals = ["cheapest", "soonest", "first"];
const normalise = (g) => (g === "fastest" ? "soonest" : g === "first_win" ? "first" : g === "soonest" || g === "first" ? g : "cheapest");

function chips(active) {
  for (const b of document.querySelectorAll("#plan-goals .opt")) {
    b.classList.toggle("on", b.dataset.goal === active);
    b.setAttribute("aria-pressed", String(b.dataset.goal === active));
  }
}

// Each row's consequence: the saving, the finish month, the first loan
// cleared. Fetched per goal from the plan the server already caches, and
// filled in as each answer lands; a row without an answer stays quiet.
function consequences() {
  const version=loadVersion;
  for (const g of goals) {
    const aside=document.querySelector('#plan-goals .opt[data-goal="'+g+'"] em');
    if(aside)aside.textContent="";
    getJSON("api/plan?goal=" + g, (d) => {
      if(version!==loadVersion)return;
      const em = document.querySelector('#plan-goals .opt[data-goal="' + g + '"] em');
      if (!em || !d || d.empty || d.blocked || !d.summary) return;
      const m = (minor) => fmtMoney(minor / 10**(d.currency_exponent??2), d.currency);
      if (g === "cheapest") em.textContent = d.summary.saved_minor > 0 ? sub("plan.saves", { s: m(d.summary.saved_minor) }) : fmtMonth(d.summary.payoff_date);
      else if (g === "soonest") em.textContent = fmtMonth(d.summary.payoff_date);
      else {
        const first = (d.months || []).find((mo) => mo.cleared);
        em.textContent = first ? first.cleared + ", " + fmtMonth(first.on) : fmtMonth(d.summary.payoff_date);
      }
    }).catch(() => { /* the row simply lacks its aside */ });
  }
}

function blocked(titleKey, why, fixKey, fixGo) {
  $("plan-body").hidden = true;
  $("plan-blocked-title").textContent = T(titleKey);
  $("plan-blocked-why").textContent = why;
  const fix = $("plan-blocked-fix");
  fix.textContent = T(fixKey);
  fix.onclick = null;
  fix.dataset.go = fixGo;
  $("plan-blocked").hidden = false;
}

async function load(g) {
  const version = ++loadVersion;
  $("plan-error").hidden = true;
  $("plan-empty").hidden = true;
  $("plan-blocked").hidden = true;
  $("plan-loading").hidden = !$("plan-body").hidden;
  try {
    await getJSON("api/plan" + (g ? "?goal=" + encodeURIComponent(g) : ""), (d) => {
      if (version !== loadVersion) return;
      $("plan-loading").hidden = true;
      $("plan-blocked").hidden = true;
      if (d.empty) { $("plan-body").hidden = true; $("plan-empty").hidden = false; return; }
      // The blocked card serves two facts-with-a-fix: a short budget and a
      // stale balance. A stale as-of is a year-scale fact: it keeps its year.
      if(d.blocked==='funding_required'||d.blocked==='cash_routing_stale'){
        const key=d.blocked==='cash_routing_stale'?'plan.cash_stale':'plan.funding';
        blocked(key,T(key+'.why'),'plan.funding.fix','budget-edit');
        const fix=$('plan-blocked-fix');delete fix.dataset.go;
        fix.onclick=()=>go('budget-edit',{section:'funding'});return;
      }
      if (d.blocked === "payment_reconciliation") { blocked("payment.review", T("payment.reconcile"), "tab.activity", "activity"); return; }
      if (d.blocked === "balance_stale") {
        blocked("plan.stale", sub("plan.stale.why", { d: fmtFull(d.as_of) }), "plan.stale.fix", "loans");
        return;
      }
      if (d.blocked) {
        blocked("plan.blocked", sub("plan.blocked.why", { d: fmtDate(d.on), r: fmtMoney(d.required_major, d.currency), s: fmtMoney(d.short_major, d.currency) }), "plan.blocked.fix", "budget");
        return;
      }
      goal = g || "";
      chips(normalise(d.goal));
      render(d);
      $("plan-body").hidden = false;
      if(!$("plan-layer").hidden&&!$("plan-goals").hidden)consequences();

    });
  } catch {
    if (version !== loadVersion) return;
    $("plan-loading").hidden = true;
    if ($("plan-body").hidden) $("plan-error").hidden = false;
  }
}

const paidOf = (mo) => mo.required_minor + mo.extra_minor + mo.fees_minor;

function render(d) {
  drawChart(d);
  $("pl-evidence").textContent = d.as_of+" · "+T("evidence."+d.summary.strength);
  const cur = d.currency;
  const m = (minor) => fmtMoney(minor / 10**(d.currency_exponent??2), cur);
  const s = d.summary;
  $("pl-certificate").innerHTML=planEvidence(d);
  $("pl-no-growth").hidden=!d.no_growth&&!d.no_growth_blocked;
  $("pl-no-growth").textContent=d.no_growth?T("plan.no_growth")+": "+fmtMonth(d.no_growth.payoff_date)+" · "+m(d.no_growth.interest_minor):d.no_growth_blocked?T("plan.no_growth_blocked"):"";
  const months = d.months || [];

  $("pl-date").textContent = fmtMonth(s.payoff_date);
  $("pl-sub").textContent = months.length > 0
    ? sub("plan.permonth", { n: s.months, p: m(paidOf(months[0])) })
    : sub("plan.nmonths", { n: s.months });
  $("pl-save-row").hidden = !(s.saved_minor > 0);
  $("pl-save").textContent = m(s.saved_minor);
  $("pl-sooner-row").hidden = !(s.saved_months > 0);
  $("pl-sooner").textContent = sub("plan.nmonths", { n: s.saved_months });
  $("pl-cost").textContent = m(s.interest_minor) + (s.fees_minor > 0 ? sub("plan.cost_fees", { f: m(s.fees_minor) }) : "");
  $("pl-finish-short").textContent = fmtMonth(s.payoff_date);
  $("pl-bar").style.width = "2%";
  displayedPlan=d;
  $("pl-outdated").hidden=!d.outdated;
  if(approvalRequest?.proposal!==d.proposal) approvalRequest=null;
  $("pl-approved").hidden = !d.approved;
  $("pl-approve").hidden = d.approved;

  // This month: whom to pay and how much. The bar splits required from
  // extra; the rows name each loan; the extra is brass.
  const rows = $("pl-month-rows");
  rows.textContent = "";
  if (months.length > 0) {
    const mo = months[0];
    $("pl-month-lbl").innerHTML = esc(T("plan.thismonth")) + " <span>" + esc(fmtDate(mo.on)) + "</span>";
    const total = paidOf(mo);
    $("pl-stack").hidden = total <= 0;
    $("pl-stack-req").style.width = total > 0 ? ((mo.required_minor + mo.fees_minor) * 100 / total) + "%" : "0";
    $("pl-stack-extra").style.width = total > 0 ? (mo.extra_minor * 100 / total) + "%" : "0";
    for (const ln of mo.loans || []) {
      if (ln.paid_minor <= 0) continue;
      const base = ln.paid_minor - ln.extra_minor - (ln.fees_minor||0);
      if (base > 0) {
        const row = document.createElement("div");
        const who = document.createElement("span"); who.textContent = ln.name;
        const amt = document.createElement("b"); amt.className = "num"; amt.textContent = m(base);
        row.append(who, amt); rows.append(row);
      }
      if (ln.fees_minor > 0) {
        const row=document.createElement("div");
        const label=document.createElement("span");label.textContent=ln.name+" · "+T("chart.fees");
        const amount=document.createElement("b");amount.textContent=m(ln.fees_minor);row.append(label,amount);rows.append(row);
      }
      if (ln.extra_minor > 0) {
        const row = document.createElement("div");
        const who = document.createElement("span"); who.textContent = sub("plan.extra_to", { n: ln.name });
        const amt = document.createElement("b"); amt.className = "num gold"; amt.textContent = m(ln.extra_minor);
        row.append(who, amt); rows.append(row);
      }
    }
  }

  // The milestones: today, each payoff with what it frees, the end.
  const ms = $("pl-ms");
  ms.textContent = "";
  const li = (when, whatHTML, gold) => {
    const el = document.createElement("li");
    if (gold) el.className = "gold";
    el.innerHTML = '<span class="when">' + esc(when) + '</span><span class="what">' + whatHTML + "</span>";
    ms.append(el);
  };
  li(T("plan.today"), esc(T("plan.same")), false);
  for (const mo of months) {
    if (!mo.cleared) continue;
    let pill = "";
    for (const ln of mo.loans || []) {
      if (ln.cleared && ln.freed_minor > 0) {
        pill = ' <span class="pill brass">' + esc(sub("plan.freed", { f: m(ln.freed_minor) })) + "</span>";
      }
    }
    li(fmtMonth(mo.on), esc(sub("plan.paidoff", { n: mo.cleared })) + pill, true);
  }
  li(fmtMonth(s.payoff_date), esc(T("plan.debtfree")), true);

  // The sheet, one tap away: the timeline renders lazily on first open.
  const all = $("pl-all");
  const list = $("pl-list");
  list.hidden = true;
  list.textContent = "";
  all.hidden = months.length === 0;
  all.textContent = sub("plan.allmonths", { n: s.months });
  all.onclick = () => {
    haptic.tap();
    if (list.textContent === "") renderTimeline(list, months, m, s);
    list.hidden = !list.hidden;
  };
}

// The full timeline: months where something happens, identical stretches
// folded into one openable line.
function renderTimeline(list, months, m, s) {
  const milestone = (mo, i) => {
    if (i === 0 || i === months.length - 1 || mo.cleared) return true;
    const prev = months[i - 1];
    if (prev.cleared) return true; // the month after a payoff: the new rhythm
    return Math.abs(paidOf(mo) - paidOf(prev)) > Math.max(1000, paidOf(prev) * 0.02);
  };
  let i = 0;
  while (i < months.length) {
    if (milestone(months[i], i)) {
      list.appendChild(monthCard(months[i], m));
      i++;
      continue;
    }
    let j = i;
    while (j < months.length && !milestone(months[j], j)) j++;
    if (j - i === 1) {
      list.appendChild(monthCard(months[i], m));
    } else {
      const li = document.createElement("li");
      li.className = "stretch";
      li.innerHTML = '<div class="stretchrow">' +
        sub("plan.stretch", { n: j - i, from: fmtDate(months[i].on), to: fmtDate(months[j - 1].on), x: "<b>" + m(paidOf(months[i])) + "</b>" }) +
        ' <span class="open">' + T("plan.stretch.open") + "</span></div>";
      const i0 = i, j0 = j;
      li.addEventListener("click", () => {
        haptic.tap();
        const frag = document.createDocumentFragment();
        for (let k = i0; k < j0; k++) frag.appendChild(monthCard(months[k], m));
        li.replaceWith(frag);
      });
      list.appendChild(li);
    }
    i = j;
  }
  const flag = document.createElement("li");
  flag.innerHTML = '<div class="finish">' + esc(sub("plan.debt_free", { d: fmtDate(s.payoff_date) })) + "</div>";
  list.appendChild(flag);
}

function monthCard(mo, m) {
  const li = document.createElement("li");
  const paid = paidOf(mo);
  let loans = "";
  for (const ln of mo.loans || []) {
    if (ln.paid_minor <= 0 && !ln.cleared) continue;
    loans += '<li><span class="who">' + esc(ln.name) + "</span>" +
      '<span class="amt">' + m(ln.paid_minor) +
      (ln.extra_minor > 0 ? ' <span class="x">' + sub("plan.extra", { x: m(ln.extra_minor) }) + "</span>" : "") +
      "</span></li>";
  }
  li.innerHTML = '<div class="mcard">' +
    '<div class="mhead"><span class="mwhen">' + fmtDate(mo.on) + '</span><span class="mpay num">' + m(paid) + "</span></div>" +
    (loans ? '<ul class="mloans">' + loans + "</ul>" : "") +
    (mo.cleared ? '<div class="win">' + esc(sub("plan.paidoff", { n: mo.cleared })) + "</div>" : "") +
    "</div>";
  return li;
}

async function approve() {
  haptic.tap();
  $("pl-approve").disabled = true;
  try {
    if(!displayedPlan?.proposal) throw new Error("missing proposal");
    approvalRequest ||= {proposal:displayedPlan.proposal,expected_revision:displayedPlan.active_revision,idempotency_key:crypto.randomUUID()};
    const res = await api("api/plans/activate", { method: "POST", body: JSON.stringify(approvalRequest) });
    if (!res.ok) throw new Error("http " + res.status);
    haptic.ok();
    invalidate("api/plan");
    toast(T("plan.approve_done"));
    $("pl-approve").hidden = true;
    $("pl-approved").hidden = false;
  } catch {
    toast(T("plan.approve_fail"));
  } finally {
    $("pl-approve").disabled = false;
  }
}

register({
  id: "plan",
  icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 20c5-1 3-7 8-8s3-6 8-7"/><path d="M17 3h4v4"/></svg>',
  labelKey: "tab.plan",
  titleKey: "plan.title",
  html: HTML,
  onMount() {
    for(const b of document.querySelectorAll("[data-layer]")) b.onclick=()=>{ if(b.dataset.layer==="strategy")consequences(); $("plan-overview").hidden=true;$("plan-layer").hidden=false;for(const s of document.querySelectorAll("[data-plan-layer]"))s.hidden=s.dataset.planLayer!==b.dataset.layer; };
    $("plan-layer-back").onclick=()=>{$("plan-overview").hidden=false;$("plan-layer").hidden=true;};
    for (const b of document.querySelectorAll("#plan-goals .opt")) {
      b.addEventListener("click", () => { haptic.pick(); chips(b.dataset.goal); load(b.dataset.goal); });
    }
    $("plan-retry").addEventListener("click", () => { haptic.tap(); load(goal); });
    $("pl-approve").addEventListener("click", approve);
    $("pl-stress").onclick=()=>go("plan-stress",{sheet:displayedPlan});
    $("pl-scenarios").onclick=()=>go("plan-scenarios",{sheet:displayedPlan});
    $("pl-export").onclick=async()=>{
      const b=$("pl-export");b.disabled=true;
      try{const r=await api("api/plan/timeline?format=csv&proposal="+encodeURIComponent(displayedPlan.proposal));if(!r.ok)throw new Error("export");const blob=await r.blob(),url=URL.createObjectURL(blob),a=document.createElement("a");a.href=url;a.download="marum-plan.csv";document.body.appendChild(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),60000);}catch{toast(T("err.load"));}finally{b.disabled=false;}
    };
  },
  onShow(_el,params) { $("plan-overview").hidden=false;$("plan-layer").hidden=true; if(params?.resetGoal)goal="";load(goal); },
});
