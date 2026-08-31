// My loans: the summary, then each loan as a card with rename and remove.
// Contract terms are deliberately not editable — changing a rate rewrites
// what every past balance meant, and the schema versions contracts for that.
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
  const next = document.createElement("p"); next.className = "next";
  if (loan.next_due && loan.next_payment_major != null) {
    const b = document.createElement("b"); b.textContent = fmtMoney(loan.next_payment_major, loan.currency);
    next.append(document.createTextNode(T("manage.nextpay") + " "), b, document.createTextNode(" · " + fmtDate(loan.next_due)));
  } else next.style.margin = "0 0 12px";
  const actions = document.createElement("div"); actions.className = "actions";
  const edit = document.createElement("button"); edit.type = "button"; edit.textContent = T("manage.edit");
  edit.onclick = () => { haptic.tap(); el.classList.add("editing"); };
  const remove = document.createElement("button"); remove.type = "button"; remove.className = "danger";
  remove.textContent = T("manage.remove");
  remove.onclick = async () => {
    haptic.tap();
    if (!(await confirmDialog(T("manage.confirm")))) return;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), { method: "DELETE" });
    if (res.ok) { el.remove(); haptic.ok(); invalidate("api/"); load(); }
    else haptic.bad();
  };
  actions.append(edit, remove);
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
  const nameIn = document.createElement("input"); nameIn.value = loan.name; nameIn.maxLength = 60;
  const descIn = document.createElement("input"); descIn.value = loan.description || "";
  descIn.maxLength = 200; descIn.placeholder = T("description");
  const row = document.createElement("div"); row.className = "actions";
  const save = document.createElement("button"); save.type = "button"; save.textContent = T("manage.save");
  save.onclick = async () => {
    if (!nameIn.value.trim()) { nameIn.setAttribute("aria-invalid", "true"); haptic.bad(); return; }
    save.disabled = true;
    const res = await api("api/loans/" + encodeURIComponent(loan.id), {
      method: "PATCH",
      body: JSON.stringify({ name: nameIn.value.trim(), description: descIn.value.trim() }),
    });
    save.disabled = false;
    if (!res.ok) { haptic.bad(); return; }
    loan.name = nameIn.value.trim(); loan.description = descIn.value.trim();
    name.textContent = loan.name;
    meta.firstChild.textContent = metaText();
    el.classList.remove("editing");
    haptic.ok();
    invalidate("api/");
    toast(T("saved"));
  };
  const cancel = document.createElement("button"); cancel.type = "button"; cancel.textContent = T("manage.cancel");
  cancel.onclick = () => { nameIn.value = loan.name; descIn.value = loan.description || ""; el.classList.remove("editing"); };
  row.append(save, cancel);
  form.append(nameIn, descIn, row);
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
  icon: "📋",
  labelKey: "tab.loans",
  html: HTML,
  onMount() {
    $("manage-retry").addEventListener("click", () => { haptic.tap(); load(); });
  },
  onShow() { load(); decorate(); },
});
