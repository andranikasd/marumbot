// My loans: the summary, then each loan as a card with edit and remove.
// Contract terms ARE editable — the server never rewrites the current
// version, it files a NEW contract version with its own effective date, so
// every past balance keeps meaning what it meant. The currency alone is
// fixed: re-denominating a ledger is archive-and-refile, not an edit.
"use strict";
import { haptic, toast, fmtMoney, fmtDate, confirmDialog } from "../core.js";
import { T } from "../i18n.js";
import { api, getJSON, invalidate } from "../api.js";

import { register } from "../nav.js";

const HTML = `
<h1 data-i18n="manage.title">Իմ վարկերը</h1>
  <div class="hero" id="manage-summary" hidden>
    <div class="k" data-i18n="manage.owed">Ընդհանուր պարտք</div>
    <div class="v" id="m-owed">—</div>
    <div class="row">
      <div class="cell"><div class="k" data-i18n="manage.required">Այս ամիս</div><div class="v" id="m-required">—</div></div>
      <div class="cell"><div class="k" data-i18n="manage.next">Հաջորդը</div><div class="v" id="m-next">—</div></div>
      <div class="cell" id="m-savecell" hidden><div class="k" data-i18n="manage.saving">Խնայում եք</div><div class="v gold" id="m-saving">—</div></div>
    </div>
    <div id="m-track" hidden>
      <div class="plan-progress"><i></i></div>
      <div class="plan-progress-cap"><span data-i18n="plan.today">այսօր</span><span id="m-free"></span></div>
    </div>
  </div>
<div id="manage-list"></div>
  <div id="manage-loading" hidden><div class="skel hero-skel"></div><div class="skel"></div><div class="skel"></div></div>
  <div class="state" id="manage-error" hidden>
    <span data-i18n="err.load">Չհաջողվեց բեռնել։</span><br>
    <button class="cta ghost" type="button" id="manage-retry" data-i18n="retry">Փորձել նորից</button>
  </div>
  <div class="empty" id="manage-empty" hidden>
    <div class="ic">📋</div>
    <p data-i18n="manage.empty">Դուք դեռ վարկ չեք ավելացրել։</p>
    <button class="cta" type="button" data-go="add" data-i18n="manage.add_first">Ավելացնել առաջինը</button>
  </div>
`;

const $ = (id) => document.getElementById(id);

// mainCurrency is the currency the summary is totalled in; a loan outside it
// carries a chip instead of silently vanishing from the totals.
let mainCurrency = "";

function loanCard(loan) {
  const el = document.createElement("div");
  el.className = "card"; el.dataset.id = loan.id;

  const view = document.createElement("div"); view.className = "view";
  const h = document.createElement("h2");
  const name = document.createElement("span"); name.className = "name"; name.textContent = loan.name;
  const bal = document.createElement("span"); bal.className = "bal"; bal.textContent = loan.balance;
  h.append(name, bal);
  const meta = document.createElement("p"); meta.className = "meta";
  const metaText = () => (loan.description ? loan.description + " · " : "") + T("manage.until") + " " + fmtDate(loan.maturity);
  meta.append(document.createTextNode(metaText()));
  if (!loan.confirmed) {
    const tag = document.createElement("span"); tag.className = "tag yours"; tag.textContent = T("manage.yours");
    meta.append(tag);
  }
  if (mainCurrency && loan.currency !== mainCurrency && loan.balance_major > 0) {
    const tag = document.createElement("span"); tag.className = "tag cur";
    tag.textContent = loan.currency + " · " + T("manage.excluded");
    meta.append(tag);
  }
  const next = document.createElement("p"); next.className = "next";
  if (loan.next_due && loan.next_payment_major != null) {
    const b = document.createElement("b"); b.textContent = fmtMoney(loan.next_payment_major, loan.currency);
    next.append(document.createTextNode(T("manage.nextpay") + " "), b, document.createTextNode(" · " + fmtDate(loan.next_due)));
  } else next.style.margin = "0 0 12px";
  // The read view keeps one calm verb. Removal is a contract-level act and
  // lives inside edit mode, away from the everyday path.
  const actions = document.createElement("div"); actions.className = "actions";
  const edit = document.createElement("button"); edit.type = "button"; edit.textContent = T("manage.edit");
  edit.onclick = () => { haptic.tap(); el.classList.add("editing"); };
  actions.append(edit);
  if (loan.original_major) {
    const bar = document.createElement("div"); bar.className = "paybar";
    const fill = document.createElement("i");
    const share = Math.max(2, Math.min(98, Math.round((1 - loan.balance_major / loan.original_major) * 100)));
    fill.style.width = share + "%";
    bar.appendChild(fill);
    view.append(h, meta, bar, next, actions);
  } else {
    view.append(h, meta, next, actions);
  }

  const form = document.createElement("div"); form.className = "edit";
  const field = (labelKey, input) => {
    const wrap = document.createElement("label"); wrap.className = "efield";
    const cap = document.createElement("span"); cap.textContent = T(labelKey);
    wrap.append(cap, input);
    return wrap;
  };
  const nameIn = document.createElement("input"); nameIn.value = loan.name; nameIn.maxLength = 60;
  const descIn = document.createElement("input"); descIn.value = loan.description || "";
  descIn.maxLength = 200; descIn.placeholder = T("description");
  const rateIn = document.createElement("input"); rateIn.inputMode = "decimal";
  rateIn.value = loan.rate_percent != null ? String(loan.rate_percent) : "";
  const dayIn = document.createElement("input"); dayIn.inputMode = "numeric";
  dayIn.value = loan.payment_day != null ? String(loan.payment_day) : "";
  const startIn = document.createElement("input"); startIn.type = "date"; startIn.value = loan.start || "";
  const matIn = document.createElement("input"); matIn.type = "date"; matIn.value = loan.maturity || "";
  const methodIn = document.createElement("select");
  for (const [v, k] of [["annuity", "method.annuity"], ["declining", "method.declining"]]) {
    const o = document.createElement("option"); o.value = v; o.textContent = T(k); methodIn.append(o);
  }
  methodIn.value = loan.method || "annuity";
  const prepayIn = document.createElement("select");
  for (const [v, k] of [["", "prepay.unsure"], ["shorten_term", "prepay.shorten"], ["reduce_instalment", "prepay.reduce"]]) {
    const o = document.createElement("option"); o.value = v; o.textContent = T(k); prepayIn.append(o);
  }
  prepayIn.value = loan.prepay_effect || "";
  const balIn = document.createElement("input"); balIn.inputMode = "decimal";
  balIn.placeholder = T("manage.balance_keep");
  const row = document.createElement("div"); row.className = "actions";
  const save = document.createElement("button"); save.type = "button"; save.textContent = T("manage.save");
  save.onclick = async () => {
    if (!nameIn.value.trim()) { nameIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    const rate = Number(rateIn.value);
    const day = Number(dayIn.value);
    if (Number.isNaN(rate) || rate < 0 || rate > 200) { rateIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    if (!Number.isInteger(day) || day < 1 || day > 31) { dayIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    if (!startIn.value || !matIn.value || matIn.value <= startIn.value) { matIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    const bal = balIn.value.trim() ? Number(balIn.value) : 0;
    if (balIn.value.trim() && (Number.isNaN(bal) || bal < 0)) { balIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    save.disabled = true;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), {
      method: "PATCH",
      body: JSON.stringify({
        name: nameIn.value.trim(), description: descIn.value.trim(),
        rate_percent: rate, payment_day: day,
        start_date: startIn.value, maturity_date: matIn.value,
        method: methodIn.value, prepay_effect: prepayIn.value,
        balance_major: bal,
      }),
    });
    save.disabled = false;
    if (!res.ok) { haptic.bad(); return; }
    el.classList.remove("editing");
    haptic.ok();
    invalidate("api/");
    toast(T("saved"));
    load(); // terms changed: the card's balance, schedule and dates all may follow
  };
  const cancel = document.createElement("button"); cancel.type = "button"; cancel.textContent = T("manage.cancel");
  cancel.onclick = () => {
    nameIn.value = loan.name; descIn.value = loan.description || "";
    rateIn.value = loan.rate_percent != null ? String(loan.rate_percent) : "";
    dayIn.value = loan.payment_day != null ? String(loan.payment_day) : "";
    startIn.value = loan.start || ""; matIn.value = loan.maturity || "";
    methodIn.value = loan.method || "annuity"; prepayIn.value = loan.prepay_effect || "";
    balIn.value = "";
    el.classList.remove("editing");
  };
  row.append(save, cancel);
  const remove = document.createElement("button"); remove.type = "button"; remove.className = "danger";
  remove.textContent = T("manage.remove");
  remove.onclick = async () => {
    haptic.tap();
    if (!(await confirmDialog(T("manage.confirm")))) return;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "DELETE" });
    if (res.ok) { el.remove(); haptic.ok(); invalidate("api/"); load(); }
    else haptic.bad();
  };
  row.append(remove);
  form.append(
    field("title.field", nameIn), field("description", descIn),
    field("rate", rateIn), field("day", dayIn),
    field("start", startIn), field("maturity", matIn),
    field("method", methodIn), field("prepay", prepayIn),
    field("manage.balance", balIn),
    row,
  );
  el.append(view, form);
  return el;
}

function summarise(loans) {
  const box = $("manage-summary");
  const live = loans.filter((l) => l.balance_major > 0);
  if (live.length === 0) { box.hidden = true; return; }
  const cur = live[0].currency;
  let owed = 0, required = 0, next = null;
  for (const l of live) {
    if (l.currency !== cur) continue;
    owed += l.balance_major;
    if (l.next_payment_major != null) required += l.next_payment_major;
    if (l.next_due && (!next || l.next_due < next.next_due)) next = l;
  }
  $("m-owed").textContent = fmtMoney(owed, cur);
  $("m-required").textContent = required > 0 ? fmtMoney(required, cur) : "—";
  $("m-next").textContent = next ? fmtDate(next.next_due) + " · " + next.name : "—";
  box.hidden = false;
}

function renderList(loans) {
  const list = $("manage-list");
  list.textContent = "";
  const live = loans.filter((l) => l.balance_major > 0);
  mainCurrency = live.length > 0 ? live[0].currency : "";
  for (const l of loans) list.append(loanCard(l));
  summarise(loans);
  $("manage-empty").hidden = loans.length > 0;
}

// The freedom track on the loans hero comes from the plan, which boot has
// already prefetched; when it is not there yet, the hero simply lacks the
// track until the next visit. Never a second spinner for a garnish.
function decorate() {
  getJSON("api/plan", (d) => {
    if (d && !d.empty && !d.blocked && d.summary) {
      $("m-free").textContent = fmtDate(d.summary.payoff_date);
      $("m-track").hidden = false;
      if (d.summary.saved_minor > 0) {
        $("m-saving").textContent = fmtMoney(d.summary.saved_minor / 100, d.currency);
        $("m-savecell").hidden = false;
      }
    }
  }).catch(() => { /* the track is optional */ });
}

async function load() {
  const list = $("manage-list");
  $("manage-error").hidden = true;
  $("manage-empty").hidden = true;
  $("manage-loading").hidden = list.children.length > 0; // silent refresh when something is on screen
  try {
    await getJSON("api/loans", (body) => renderList(body.loans || []));
  } catch {
    $("manage-summary").hidden = true;
    $("manage-error").hidden = false;
  } finally {
    $("manage-loading").hidden = true;
  }
}

register({
  id: "loans",
  icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 6h13M4 12h13M4 18h9"/><circle cx="20" cy="18" r="1.6"/></svg>',
  labelKey: "tab.loans",
  html: HTML,
  onMount() {
    $("manage-retry").addEventListener("click", () => { haptic.tap(); load(); });
  },
  onShow() { load(); decorate(); },
});
