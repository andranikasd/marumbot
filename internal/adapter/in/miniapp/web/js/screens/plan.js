// The plan. The answer comes first — the debt-free date, the monthly
// amount, the saving — then the strategy as three rows with their
// consequence, then this month's payments, then the milestones. The
// month-by-month sheet is one tap away, not the default reading: most
// months of a repayment are the same month repeated.
"use strict";
import { haptic, toast, fmtMoney, fmtDate, fmtMonth, fmtFull, esc } from "../core.js";
import { T, sub } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register } from "../nav.js";

const HTML = `
  <div id="plan-loading" hidden><div class="skel hero-skel"></div><div class="skel" style="margin-top:12px"></div><div class="skel" style="margin-top:10px"></div></div>
  <div id="plan-blocked" class="card stack" hidden>
    <b id="plan-blocked-title"></b>
    <p class="lede" id="plan-blocked-why" style="margin:0"></p>
    <button class="cta" type="button" id="plan-blocked-fix"></button>
  </div>
  <div class="state" id="plan-empty" hidden>
    <div class="tile lg">M</div>
    <b data-i18n="plan.empty.title">Պլան դեռ չկա</b>
    <span data-i18n="plan.empty">Պլանի համար պետք են վարկեր և բյուջե։</span>
    <button class="cta" type="button" data-go="add" data-i18n="plan.empty.add">Ավելացնել վարկ</button>
  </div>
  <div id="plan-body" class="stack" hidden>
    <div class="hero">
      <div class="k"><span data-i18n="plan.debtfree">Ազատ պարտքից</span><span class="pill" id="pl-approved" hidden data-i18n="plan.approved_badge">Հաստատված պլան</span></div>
      <div class="v" id="pl-date"></div>
      <div class="sub" id="pl-sub"></div>
      <div class="kv">
        <div id="pl-save-row" hidden><span data-i18n="plan.saved">Խնայված տոկոս</span><b class="num gold" id="pl-save"></b></div>
        <div id="pl-sooner-row" hidden><span data-i18n="plan.sooner">Ավելի շուտ</span><b id="pl-sooner"></b></div>
        <div><span data-i18n="plan.interest">Ընդհանուր տոկոս</span><b class="num" id="pl-cost"></b></div>
      </div>
      <div class="track"><i id="pl-bar"></i></div>
      <div class="track-cap"><span data-i18n="plan.today">այսօր</span><span id="pl-finish-short"></span></div>
    </div>
    <div class="card" id="plan-goals">
      <button type="button" class="opt" data-goal="cheapest"><i></i><span><b data-i18n="plan.g.cheapest">Ամենաքիչ տոկոս</b><small data-i18n="plan.g.cheapest.desc"></small></span><em></em></button>
      <button type="button" class="opt" data-goal="soonest"><i></i><span><b data-i18n="plan.g.soonest">Ամենաշուտ</b><small data-i18n="plan.g.soonest.desc"></small></span><em></em></button>
      <button type="button" class="opt" data-goal="first"><i></i><span><b data-i18n="plan.g.first">Առաջին հաղթանակ</b><small data-i18n="plan.g.first.desc"></small></span><em></em></button>
    </div>
    <div class="card">
      <p class="sec" id="pl-month-lbl" style="margin:0 0 2px"></p>
      <div class="stack-bar" id="pl-stack" hidden><i class="req" id="pl-stack-req"></i><i class="extra" id="pl-stack-extra"></i></div>
      <div class="kv" id="pl-month-rows"></div>
    </div>
    <ol class="ms" id="pl-ms"></ol>
    <button class="alink" type="button" id="pl-all" hidden></button>
    <ol class="tl" id="pl-list" hidden></ol>
    <div class="pin-space"></div>
    <div class="pin"><button class="cta gold" type="button" id="pl-approve" data-i18n="plan.approve" hidden>Հաստատել պլանը</button></div>
  </div>
  <div class="state" id="plan-error" hidden>
    <b data-i18n="plan.error">Չստացվեց հաշվել պլանը։</b>
    <button class="alink" type="button" id="plan-retry" data-i18n="retry">Փորձել նորից</button>
  </div>
`;

const $ = (id) => document.getElementById(id);
let goal = new URLSearchParams(location.search).get("goal") || "";
let loadVersion = 0;
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
  for (const g of goals) {
    getJSON("api/plan?goal=" + g, (d) => {
      const em = document.querySelector('#plan-goals .opt[data-goal="' + g + '"] em');
      if (!em || !d || d.empty || d.blocked || !d.summary) return;
      const m = (minor) => fmtMoney(minor / 100, d.currency);
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
      if (d.blocked === "balance_stale") {
        blocked("plan.stale", sub("plan.stale.why", { d: fmtFull(d.as_of) }), "plan.stale.fix", "loans");
        return;
      }
      if (d.blocked) {
        blocked("plan.blocked", sub("plan.blocked.why", { d: fmtDate(d.on), r: fmtMoney(d.required_major, d.currency), s: fmtMoney(d.short_major, d.currency) }), "plan.blocked.fix", "budget");
        return;
      }
      goal = g || normalise(d.goal);
      chips(goal);
      render(d);
      $("plan-body").hidden = false;
      consequences();
    });
  } catch {
    if (version !== loadVersion) return;
    $("plan-loading").hidden = true;
    if ($("plan-body").hidden) $("plan-error").hidden = false;
  }
}

const paidOf = (mo) => mo.required_minor + mo.extra_minor + mo.fees_minor;

function render(d) {
  const cur = d.currency;
  const m = (minor) => fmtMoney(minor / 100, cur);
  const s = d.summary;
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
      const base = ln.paid_minor - ln.extra_minor;
      if (base > 0) {
        const row = document.createElement("div");
        const who = document.createElement("span"); who.textContent = ln.name;
        const amt = document.createElement("b"); amt.className = "num"; amt.textContent = m(base);
        row.append(who, amt); rows.append(row);
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
    const res = await api("api/plan/approve", { method: "POST", body: JSON.stringify({ goal: goal || "cheapest" }) });
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
    for (const b of document.querySelectorAll("#plan-goals .opt")) {
      b.addEventListener("click", () => { haptic.pick(); chips(b.dataset.goal); load(b.dataset.goal); });
    }
    $("plan-retry").addEventListener("click", () => { haptic.tap(); load(goal); });
    $("pl-approve").addEventListener("click", approve);
  },
  onShow() { load(goal); },
});
