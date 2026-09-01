// The plan, told as three facts: what to pay now, when the first loan dies,
// when it all ends. The hero answers everything at once — pay this much,
// free on this date, saving this much. Below it only two things remain: this
// month's payments and the milestones. The month-by-month sheet is one tap
// away ("all months"), not the default reading: most months of a repayment
// are the same month repeated, and a reader asked to study them stops
// reading.
"use strict";
import { haptic, toast, fmtMoney, fmtDate, esc, lang } from "../core.js";
import { T, sub } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register } from "../nav.js";

const HTML = `
  <h1 data-i18n="plan.title">Իմ պլանը</h1>
  <div class="goalseg" id="plan-goals">
    <button type="button" data-goal="cheapest"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M19 5 5 19"/><circle cx="6.5" cy="6.5" r="2.5"/><circle cx="17.5" cy="17.5" r="2.5"/></svg><span data-i18n="plan.g.cheapest">Ամենաքիչ տոկոս</span></button>
    <button type="button" data-goal="soonest"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 21V4M6 4h11l-2.5 3.5L17 11H6"/></svg><span data-i18n="plan.g.soonest">Ամենաշուտ</span></button>
    <button type="button" data-goal="first"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="4.5"/><circle cx="12" cy="12" r="1"/></svg><span data-i18n="plan.g.first">Առաջին վարկը</span></button>
  </div>
  <p class="goal-caption" id="plan-caption"></p>
  <div id="plan-loading" hidden><div class="skel hero-skel"></div><div class="skel"></div><div class="skel"></div></div>
  <div id="plan-blocked" hidden>
    <div class="card"><b data-i18n="plan.blocked">Բյուջեն չի բավականացնում</b>
      <p class="meta" id="plan-blocked-why"></p>
      <button class="cta" type="button" data-go="budget" data-i18n="plan.blocked.fix">💰 Բարձրացնել բյուջեն</button>
    </div>
  </div>
  <div id="plan-empty" hidden>
    <p class="lede" data-i18n="plan.empty">Պլանի համար պետք են վարկեր և բյուջե։</p>
    <button class="cta" type="button" data-go="add" data-i18n="plan.empty.add">Ավելացնել վարկ</button>
  </div>
  <div id="plan-body" hidden>
    <div class="hero">
      <div class="k" id="pl-pay"></div>
      <div class="v" id="pl-date"></div>
      <div class="sub" id="pl-cost"></div>
      <div class="save" id="pl-save" hidden></div>
      <div class="plan-progress"><i id="pl-bar"></i></div>
      <div class="plan-progress-cap"><span data-i18n="plan.today">այսօր</span><span id="pl-finish-short"></span></div>
      <div><span class="plan-approved" id="pl-approved" hidden data-i18n="plan.approved_badge">✅ Հաստատված պլան</span></div>
    </div>
    <div class="card">
      <p class="cardlbl" id="pl-month-lbl"></p>
      <div id="pl-month-rows"></div>
    </div>
    <ol class="ms" id="pl-ms"></ol>
    <button class="alink" type="button" id="pl-all" hidden></button>
    <ol class="tl" id="pl-list" hidden></ol>
    <div class="approve-space"></div>
    <div class="approve-pin"><button class="cta" type="button" id="pl-approve" data-i18n="plan.approve" hidden>✅ Հաստատել այս պլանը</button></div>
  </div>
  <div id="plan-error" hidden>
    <p class="lede" data-i18n="plan.error">Չստացվեց հաշվել պլանը։</p>
    <button class="cta" type="button" id="plan-retry" data-i18n="retry">Փորձել նորից</button>
  </div>
`;

const $ = (id) => document.getElementById(id);
let goal = new URLSearchParams(location.search).get("goal") || "";

// A payoff or milestone sits years out, so it reads as month + year; fmtDate
// (day + month) is for dates inside the running year.
const fmtMonth = (iso) => {
  const d = new Date(iso + "T00:00:00");
  return Number.isNaN(d.getTime()) ? iso
    : new Intl.DateTimeFormat(lang === "en" ? "en-GB" : "hy-AM", { month: "short", year: "numeric" }).format(d);
};
// A full date with its year, for facts that live outside the running year —
// a balance stated long ago, for one.
const fmtFull = (iso) => {
  const d = new Date(iso + "T00:00:00");
  return Number.isNaN(d.getTime()) ? iso
    : new Intl.DateTimeFormat(lang === "en" ? "en-GB" : "hy-AM", { day: "numeric", month: "short", year: "numeric" }).format(d);
};

function chips(active) {
  for (const b of document.querySelectorAll("#plan-goals button")) {
    b.classList.toggle("on", b.dataset.goal === active);
  }
  $("plan-caption").textContent = T("plan.g." + active + ".desc");
}

async function load(g) {
  $("plan-error").hidden = true;
  $("plan-empty").hidden = true;
  $("plan-loading").hidden = !$("plan-body").hidden;
  try {
    await getJSON("api/plan" + (g ? "?goal=" + encodeURIComponent(g) : ""), (d) => {
      $("plan-loading").hidden = true;
      $("plan-blocked").hidden = true;
      if (d.empty) { $("plan-body").hidden = true; $("plan-empty").hidden = false; return; }
      // The blocked card serves two facts-with-a-fix: a short budget and a
      // stale balance. Both branches set every mutable part — title, reason,
      // button — so whichever fired last cannot leak into the other.
      if (d.blocked === "balance_stale") {
        $("plan-body").hidden = true;
        $("plan-blocked").querySelector("b").textContent = T("plan.stale");
        // A stale as-of is a year-scale fact: the date keeps its year.
        $("plan-blocked-why").textContent = sub("plan.stale.why", { d: fmtFull(d.as_of) });
        const fix = $("plan-blocked").querySelector("button");
        fix.textContent = T("plan.stale.fix");
        fix.dataset.go = "loans";
        $("plan-blocked").hidden = false;
        return;
      }
      if (d.blocked) {
        $("plan-body").hidden = true;
        $("plan-blocked").querySelector("b").textContent = T("plan.blocked");
        $("plan-blocked-why").textContent = sub("plan.blocked.why", { d: fmtDate(d.on), r: fmtMoney(d.required_major, "AMD"), s: fmtMoney(d.short_major, "AMD") });
        const fix = $("plan-blocked").querySelector("button");
        fix.textContent = T("plan.blocked.fix");
        fix.dataset.go = "budget";
        $("plan-blocked").hidden = false;
        return;
      }
      goal = g || (d.goal === "fastest" ? "soonest" : d.goal === "first_win" ? "first" : "cheapest");
      chips(goal);
      render(d);
      $("plan-body").hidden = false;
    });
  } catch {
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

  // The hero: the whole promise in two lines and a badge.
  $("pl-pay").textContent = months.length > 0
    ? sub("plan.paymo", { p: m(paidOf(months[0])) })
    : T("plan.hero_k");
  $("pl-date").textContent = fmtMonth(s.payoff_date);
  $("pl-cost").textContent = sub("plan.cost", { i: m(s.interest_minor) }) +
    (s.fees_minor > 0 ? sub("plan.cost_fees", { f: m(s.fees_minor) }) : "") +
    " · " + sub("plan.nmonths", { n: s.months });
  const save = $("pl-save");
  if (s.saved_minor > 0) {
    save.textContent = sub("plan.save", { s: m(s.saved_minor), m: s.saved_months });
    save.hidden = false;
  } else save.hidden = true;
  $("pl-finish-short").textContent = fmtMonth(s.payoff_date);
  $("pl-approved").hidden = !d.approved;
  $("pl-approve").hidden = d.approved;

  // This month: whom to pay and how much. One card, no dates per row — the
  // cycle has one date and it sits in the label.
  const rows = $("pl-month-rows");
  rows.textContent = "";
  if (months.length > 0) {
    $("pl-month-lbl").textContent = T("plan.thismonth") + " · " + fmtDate(months[0].on);
    for (const ln of months[0].loans || []) {
      if (ln.paid_minor <= 0) continue;
      const row = document.createElement("div");
      row.className = "prow" + (ln.extra_minor > 0 ? " extra" : "");
      const who = document.createElement("span"); who.className = "who"; who.textContent = ln.name;
      const amt = document.createElement("span"); amt.className = "amt num"; amt.textContent = m(ln.paid_minor);
      row.append(who, amt);
      rows.append(row);
    }
  }

  // The milestones: today, each payoff with what it frees, the end. The
  // long "loan clears, frees X" sentence became a name and a brass pill.
  const ms = $("pl-ms");
  ms.textContent = "";
  const li = (when, whatHTML, gold) => {
    const el = document.createElement("li");
    if (gold) el.className = "gold";
    el.innerHTML = '<div class="when">' + esc(when) + '</div><div class="what">' + whatHTML + "</div>";
    ms.append(el);
  };
  li(T("plan.today"), esc(T("plan.same")), false);
  for (const mo of months) {
    if (!mo.cleared) continue;
    let pill = "";
    for (const ln of mo.loans || []) {
      if (ln.cleared && ln.freed_minor > 0) {
        pill = ' <span class="pill">' + esc(sub("plan.freed", { f: m(ln.freed_minor) })) + "</span>";
      }
    }
    li(fmtMonth(mo.on), esc(sub("plan.paidoff", { n: mo.cleared })) + pill, true);
  }
  li(fmtMonth(s.payoff_date), esc(T("plan.debtfree")) + " 🏁", true);

  // The sheet, one tap away: the timeline renders lazily on first open.
  const all = $("pl-all");
  const list = $("pl-list");
  list.hidden = true;
  list.textContent = "";
  all.hidden = months.length === 0;
  all.textContent = "📄 " + sub("plan.allmonths", { n: s.months });
  all.onclick = () => {
    haptic.tap();
    if (list.textContent === "") renderTimeline(list, months, m, s);
    list.hidden = !list.hidden;
  };
}

// The full timeline, unchanged in spirit: months where something happens,
// identical stretches folded into one openable line.
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
  flag.innerHTML = '<div class="finish-flag">🏁 ' + sub("plan.debt_free", { d: fmtDate(s.payoff_date) }) + "</div>";
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
    '<div class="mhead"><span class="mwhen">' + fmtDate(mo.on) + '</span><span class="mpay">' + m(paid) + "</span></div>" +
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
  html: HTML,
  onMount() {
    for (const b of document.querySelectorAll("#plan-goals button")) {
      b.addEventListener("click", () => { haptic.tap(); load(b.dataset.goal); });
    }
    $("plan-retry").addEventListener("click", () => { haptic.tap(); load(goal); });
    $("pl-approve").addEventListener("click", approve);
  },
  onShow() { load(goal); },
});
