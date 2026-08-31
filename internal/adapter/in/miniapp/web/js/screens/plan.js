// The plan, told as a timeline. Most months of a repayment are the same
// month repeated; showing them all is why sheets are unreadable. The
// timeline shows the months where something happens — the first, a payment
// that changes, a loan that closes, the last — and collapses every
// identical stretch into one line that can be opened. Every shown month
// lists the loans by name: whom to pay, how much, what remains.
"use strict";
import { haptic, toast, fmtMoney, fmtDate, esc } from "../core.js";
import { T, sub } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";
import { register } from "../nav.js";

const HTML = `
  <h1 data-i18n="plan.title">Իմ պլանը</h1>
  <div class="goalseg" id="plan-goals">
    <button type="button" data-goal="cheapest">💸 <span data-i18n="plan.g.cheapest">Ամենաքիչ տոկոս</span></button>
    <button type="button" data-goal="soonest">🏁 <span data-i18n="plan.g.soonest">Ամենաշուտ</span></button>
    <button type="button" data-goal="first">🎯 <span data-i18n="plan.g.first">Առաջին վարկը</span></button>
  </div>
  <p class="goal-caption" id="plan-caption"></p>
  <div id="plan-loading" class="lede" data-i18n="plan.loading" hidden>Հաշվում եմ…</div>
  <div id="plan-empty" hidden>
    <p class="lede" data-i18n="plan.empty">Պլանի համար պետք են վարկեր և բյուջե։</p>
    <button class="cta" type="button" data-go="add" data-i18n="plan.empty.add">Ավելացնել վարկ</button>
  </div>
  <div id="plan-body" hidden>
    <div class="hero">
      <div class="k" data-i18n="plan.hero_k">Ազատ պարտքից</div>
      <div class="v" id="pl-finish"></div>
      <div class="sub" id="pl-cost"></div>
      <div class="save" id="pl-save" hidden></div>
      <div class="plan-progress"><i id="pl-bar"></i></div>
      <div class="plan-progress-cap"><span data-i18n="plan.today">այսօր</span><span id="pl-finish-short"></span></div>
      <div><span class="plan-approved" id="pl-approved" hidden data-i18n="plan.approved_badge">✅ Հաստատված պլան</span></div>
    </div>
    <button class="cta" type="button" id="pl-approve" data-i18n="plan.approve" hidden>✅ Հաստատել այս պլանը</button>
    <ol class="tl" id="pl-months"></ol>
  </div>
  <div id="plan-error" hidden>
    <p class="lede" data-i18n="plan.error">Չստացվեց հաշվել պլանը։</p>
    <button class="cta" type="button" id="plan-retry" data-i18n="retry">Փորձել նորից</button>
  </div>
`;

const $ = (id) => document.getElementById(id);
let goal = new URLSearchParams(location.search).get("goal") || "";

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
      if (d.empty) { $("plan-body").hidden = true; $("plan-empty").hidden = false; return; }
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

function render(d) {
  const cur = d.currency;
  const m = (minor) => fmtMoney(minor / 100, cur);
  const s = d.summary;
  $("pl-finish").textContent = sub("plan.finish", { d: fmtDate(s.payoff_date), n: s.months });
  let cost = sub("plan.cost", { i: m(s.interest_minor) });
  if (s.fees_minor > 0) cost += sub("plan.cost_fees", { f: m(s.fees_minor) });
  $("pl-cost").textContent = cost;
  const save = $("pl-save");
  if (s.saved_minor > 0) {
    save.textContent = sub("plan.save", { s: m(s.saved_minor), m: s.saved_months });
    save.hidden = false;
  } else save.hidden = true;
  $("pl-finish-short").textContent = fmtDate(s.payoff_date);
  $("pl-approved").hidden = !d.approved;
  $("pl-approve").hidden = d.approved;

  const months = d.months || [];
  const paidOf = (mo) => mo.required_minor + mo.extra_minor + mo.fees_minor;
  const milestone = (mo, i) => {
    if (i === 0 || i === months.length - 1 || mo.cleared) return true;
    const prev = months[i - 1];
    if (prev.cleared) return true; // the month after a payoff: the new rhythm
    return Math.abs(paidOf(mo) - paidOf(prev)) > Math.max(1000, paidOf(prev) * 0.02);
  };

  const list = $("pl-months");
  list.textContent = "";
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
  const paid = mo.required_minor + mo.extra_minor + mo.fees_minor;
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
    (mo.cleared ? '<div class="win">' + sub("plan.win", { n: esc(mo.cleared) }) + "</div>" : "") +
    '<div class="mfoot">' + (mo.owed_minor > 0 ? sub("plan.owed", { o: m(mo.owed_minor) }) : T("plan.done_row")) + "</div>" +
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
  icon: "📄",
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
