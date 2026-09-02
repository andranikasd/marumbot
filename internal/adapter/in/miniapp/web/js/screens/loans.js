// Loans: the hero answers how much, when next, and how far along; each loan
// is a card with the same three facts in the same place. Nothing edits here.
// Tapping a card opens the loan, where the everyday act — restating the
// balance after a payment — is the first button.
"use strict";
import {icon} from "../icons.js";
import { haptic, fmtMoney, fmtDate, mask } from "../core.js";
import { T, sub } from "../i18n.js";
import { getJSON } from "../api.js";
import { register } from "../nav.js";

const EYE = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12z"/><circle cx="12" cy="12" r="2.5"/></svg>';

const HTML = `
  <div class="hero" id="manage-summary" hidden>
    <div class="k"><span data-i18n="manage.owed">Ընդհանուր պարտք</span>
      <button type="button" class="eye" id="manage-mask" aria-pressed="false">${EYE}</button></div>
    <div class="v num" id="m-owed">—</div>
    <div class="sub" id="m-across"></div>
    <div class="kv">
      <div><span data-i18n="manage.required">Այս ամիս</span><b class="num" id="m-required">—</b></div>
      <div><span data-i18n="manage.next">Հաջորդ վճարումը</span><b id="m-next">—</b></div>
    </div>
    <div class="track" id="m-track" hidden><i id="m-track-fill"></i></div>
  </div>
  <p class="sec" id="manage-sec" hidden data-i18n="manage.yours">Ձեր վարկերը</p>
  <div id="manage-list" style="display:grid;gap:10px"></div>
  <button class="cta ghost" type="button" id="manage-add" data-go="add" data-i18n="manage.add" hidden>Ավելացնել վարկ</button>
  <div id="manage-loading" hidden><div class="skel hero-skel"></div><div class="skel" style="margin-top:12px"></div><div class="skel" style="margin-top:10px"></div></div>
  <div class="state" id="manage-error" hidden>
    <b data-i18n="err.load">Չհաջողվեց բեռնել։</b>
    <button class="alink" type="button" id="manage-retry" data-i18n="retry">Փորձել նորից</button>
  </div>
  <div class="state" id="manage-empty" hidden>
    <div class="tile lg">M</div>
    <b data-i18n="manage.empty.title">Դեռ վարկ չկա</b>
    <span data-i18n="manage.empty">Ավելացրեք առաջինը, և պլանը կհայտնվի։</span>
    <button class="cta" type="button" data-go="add" data-i18n="manage.add_first">Ավելացնել առաջինը</button>
  </div>
`;

const $ = (id) => document.getElementById(id);

// mainCurrency is the currency the summary is totalled in; a loan outside it
// carries a chip instead of silently vanishing from the totals.
let mainCurrency = "";

function loanCard(loan) {
  const el = document.createElement("button");
  el.type = "button";
  el.className = "card loan";
  el.dataset.go = "loan";
  el.dataset.arg = loan.id;

  const tile = document.createElement("span"); tile.className = "tile"; tile.innerHTML = icon(loan.icon);
  const nm = document.createElement("span"); nm.className = "nm"; nm.textContent = loan.name;
  const meta = document.createElement("small");
  const bits = [];
  if (loan.rate_percent != null) bits.push(loan.rate_percent + "%");
  bits.push(T(loan.method === "declining" ? "method.declining" : "method.annuity").toLowerCase());
  bits.push(fmtDate(loan.balance_as_of));
  if (loan.optional_excluded) bits.push(T("loan.noextra"));
  if (mainCurrency && loan.currency !== mainCurrency && loan.balance_major > 0) bits.push(loan.currency + " · " + T("manage.excluded"));
  meta.textContent = bits.join(" · ");
  nm.append(meta);
  const bal = document.createElement("span"); bal.className = "bal num"; bal.textContent = fmtMoney(loan.balance_major, loan.currency);
  el.append(tile, nm, bal);

  const two = document.createElement("span"); two.className = "two";
  const due = document.createElement("span");
  due.append(document.createTextNode(T("manage.nextdue")));
  const dueB = document.createElement("b"); dueB.textContent = loan.next_due ? fmtDate(loan.next_due) : "—";
  due.append(dueB);
  const pay = document.createElement("span");
  pay.append(document.createTextNode(T("manage.payment")));
  const payB = document.createElement("b"); payB.className = "num";
  payB.textContent = loan.next_payment_major != null ? fmtMoney(loan.next_payment_major, loan.currency) : "—";
  pay.append(payB);
  const chev = document.createElement("span"); chev.className = "chev"; chev.textContent = "›";
  two.append(due, pay, chev);
  el.append(two);

  if (loan.original_major) {
    const bar = document.createElement("span"); bar.className = "pb";
    const fill = document.createElement("i");
    fill.style.width = Math.max(2, Math.min(98, Math.round((1 - loan.balance_major / loan.original_major) * 100))) + "%";
    bar.append(fill);
    el.append(bar);
  }
  return el;
}

function summarise(loans) {
  const box = $("manage-summary");
  const live = loans.filter((l) => l.balance_major > 0);
  if (live.length === 0) { box.hidden = true; return; }
  const cur = live[0].currency;
  let owed = 0, required = 0, next = null, original = 0, counted = 0;
  for (const l of live) {
    if (l.currency !== cur) continue;
    counted++;
    owed += l.balance_major;
    if (l.next_payment_major != null) required += l.next_payment_major;
    if (l.next_due && (!next || l.next_due < next.next_due)) next = l;
    if (l.original_major) original += l.original_major; else original += l.balance_major;
  }
  $("m-owed").textContent = fmtMoney(owed, cur);
  const share = original > 0 ? Math.round((1 - owed / original) * 100) : 0;
  $("m-across").textContent = sub(counted === 1 ? "manage.across.one" : "manage.across", { n: counted, p: share });
  $("m-required").textContent = required > 0 ? fmtMoney(required, cur) : "—";
  $("m-next").textContent = next ? fmtDate(next.next_due) + " · " + next.name : "—";
  $("m-track").hidden = share <= 0;
  $("m-track-fill").style.width = Math.max(2, Math.min(98, share)) + "%";
  box.hidden = false;
}

function renderList(loans) {
  const list = $("manage-list");
  list.textContent = "";
  const live = loans.filter((l) => l.balance_major > 0);
  mainCurrency = live.length > 0 ? live[0].currency : "";
  for (const l of loans) list.append(loanCard(l));
  summarise(loans);
  $("manage-sec").hidden = loans.length === 0;
  $("manage-add").hidden = loans.length === 0;
  $("manage-empty").hidden = loans.length > 0;
}

async function load() {
  const list = $("manage-list");
  $("manage-error").hidden = true;
  $("manage-empty").hidden = true;
  $("manage-loading").hidden = list.children.length > 0; // silent refresh when something is on screen
  try {
    await getJSON("api/loans", (body) => renderList(body.loans || []));
  } catch {
    if (list.children.length === 0) {
      $("manage-summary").hidden = true;
      $("manage-error").hidden = false;
    }
  } finally {
    $("manage-loading").hidden = true;
  }
}

register({
  id: "loans",
  icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 6h13M4 12h13M4 18h9"/></svg>',
  labelKey: "tab.loans",
  titleKey: "manage.title",
  html: HTML,
  onMount() {
    $("manage-retry").addEventListener("click", () => { haptic.tap(); load(); });
    const eye = $("manage-mask");
    eye.setAttribute("aria-label", T("manage.mask"));
    eye.setAttribute("aria-pressed", String(mask.on()));
    eye.addEventListener("click", () => { haptic.tap(); eye.setAttribute("aria-pressed", String(mask.toggle())); });
  },
  onShow() { load(); },
});
