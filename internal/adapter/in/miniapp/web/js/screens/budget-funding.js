"use strict";
import { T, addStrings } from "../i18n.js";
import { createCashRouting } from "./budget-cash-routing.js";

addStrings({
  "bf.mode": "Վարկերի համար գումար", "bf.legacy": "Գումարը դեռ նշված չէ",
  "bf.separate": "Առանձին նշել գումարը", "bf.monthly": "Վարկերի ամսական գումար",
  "bf.spent": "Արդեն վճարված", "bf.events": "Լրացուցիչ գումար",
  "bf.date": "Մուտքի ամսաթիվ", "bf.amount": "Գումար", "bf.expected": "Սպասվող, դեռ չհաստատված",
  "bf.add": "Ավելացնել լրացուցիչ գումար", "bf.remove": "Հեռացնել գումարը",
  "bf.hint": "Նշեք՝ վարկերի համար ինչ գումար է հասանելի և երբ։ Բյուջեն սահմանում է՝ որքան ծախսել։",
  "bf.monthlyHint": "Գումար, որն ամեն ամիս ունեք վարկերի վճարման համար։",
  "bf.spentHint": "Ընթացիկ բյուջետային ժամանակահատվածում վարկերին արդեն վճարած գումարը։ Գումարի մուտքի օրը այն չի զրոյանում։",
  "bf.eventsHint": "Գումար՝ բացի սովորական ամսական մուտքից։ Սպասվող գումարը չի օգտագործվում, մինչև այն չհաստատեք։",
  "bf.money": "Նշեք ոչ բացասական գումար՝ արժույթի թույլատրած ճշտությամբ և անվտանգ սահմաններում։ Տասնորդական բաժանիչը կետ կամ ստորակետ է։",
  "bf.dateError": "Ընտրեք այսօրվա կամ ապագա ամսաթիվ՝ ձեր ժամային գոտով։",
  "bf.limit": "Կարելի է նշել առավելագույնը 36 մուտք։",
}, {
  "bf.mode": "Money for loans", "bf.legacy": "Money not entered yet",
  "bf.separate": "Enter money separately", "bf.monthly": "Money set aside each month",
  "bf.spent": "Already paid", "bf.events": "Extra money",
  "bf.date": "Available on", "bf.amount": "Amount", "bf.expected": "Expected, not yet confirmed",
  "bf.add": "Add extra money", "bf.remove": "Remove extra money",
  "bf.hint": "Enter what money is available for loans and when. Your budget sets how much to spend.",
  "bf.monthlyHint": "Money you regularly have for loan payments.",
  "bf.spentHint": "Loan payments already made in the current budget period. Payday does not reset this.",
  "bf.eventsHint": "Money in addition to your regular monthly amount. Expected money is excluded until confirmed.",
  "bf.money": "Enter a non-negative amount within safe limits and the currency’s precision. Use a dot or comma for decimals.",
  "bf.dateError": "Choose today or a future date in your account timezone.", "bf.limit": "You can specify at most 36 receipts.",
});

const MAX_MINOR = 9007199254740991n;
export function minorAmount(raw, exponent, { blankZero = false, positive = false } = {}) {
  if (!Number.isInteger(exponent) || exponent < 0 || exponent > 6) throw new Error(T("bf.money"));
  let value = String(raw).trim();
  if (!value && blankZero) value = "0";
  // Spaces may group thousands; punctuation is always a decimal separator.
  if (!/^(?:\d+|\d{1,3}(?:[ \u00a0\u202f]\d{3})+)(?:[.,]\d+)?$/.test(value)) throw new Error(T("bf.money"));
  const [whole, fraction = ""] = value.replace(/[ \u00a0\u202f]/g, "").replace(",", ".").split(".");
  if (fraction.length > exponent || whole.length > 20) throw new Error(T("bf.money"));
  const minor = BigInt(whole) * 10n ** BigInt(exponent) + BigInt(fraction.padEnd(exponent, "0") || "0");
  if (minor > MAX_MINOR || (positive && minor === 0n)) throw new Error(T("bf.money"));
  return Number(minor);
}
export function minorText(value, exponent) {
  if (!Number.isSafeInteger(value) || value < 0 || !Number.isInteger(exponent) || exponent < 0 || exponent > 6) throw new Error(T("bf.money"));
  const digits = String(value).padStart(exponent + 1, "0");
  return exponent ? digits.slice(0, -exponent) + "." + digits.slice(-exponent) : digits;
}
export function majorAmount(raw, exponent, options) {
  const minor = minorAmount(raw, exponent, options);
  const value = Number(minorText(minor, exponent));
  if (minorAmount(String(value), exponent) !== minor || Math.round(value * 10 ** exponent) !== minor) throw new Error(T("bf.money"));
  return value;
}
export function validMonth(value) { return /^(?!0000)\d{4}-(0[1-9]|1[0-2])$/.test(value); }
export function validDate(value) {
  if (!/^(?!0000)\d{4}-(0[1-9]|1[0-2])-\d{2}$/.test(value)) return false;
  const parsed = new Date(value + "T00:00:00Z");
  return Number.isFinite(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}

export const fundingHTML = `
  <div class="field" hidden><label for="funding-mode" data-i18n="bf.mode"></label>
    <select id="funding-mode"><option value="legacy" data-i18n="bf.legacy"></option><option value="separate" data-i18n="bf.separate"></option></select>
    <p class="hint" data-i18n="bf.hint"></p></div>
  <div id="funding-separate" class="stack" hidden>
    <div class="field" id="funding-monthly-field"><label for="funding-monthly" data-i18n="bf.monthly"></label><input id="funding-monthly" inputmode="decimal" aria-describedby="h-funding-monthly e-funding-monthly"><p class="hint" id="h-funding-monthly" data-i18n="bf.monthlyHint"></p><p class="error" id="e-funding-monthly"></p></div>
    <div class="field" id="funding-spent-field"><label for="funding-spent" data-i18n="bf.spent"></label><input id="funding-spent" inputmode="decimal" aria-describedby="h-funding-spent e-funding-spent"><p class="hint" id="h-funding-spent" data-i18n="bf.spentHint"></p><p class="error" id="e-funding-spent"></p></div>
    <span class="lbl" data-i18n="bf.events"></span><p class="hint" data-i18n="bf.eventsHint"></p><div id="funding-events" class="stack"></div>
    <p class="error" id="e-funding-events" role="alert"></p>
    <button class="alink" type="button" id="funding-add" data-i18n="bf.add"></button>
  </div>`;

export function createFunding(root, changed, exponent) {
  const $ = (id) => root.querySelector("#" + id);
  let nextID = 0, cashThrough="", spentPeriodStart="";
  let context={today:"",loans:[],locked:false};
  const routingRows=new WeakMap();
  const sync = () => { $("funding-separate").hidden = $("funding-mode").value !== "separate"; };
  function row(event) {
    const el = document.createElement("div"); el.className = "card stack";
    const id = "funding-event-" + nextID++;
    el.innerHTML = `<label for="${id}-date">${T("bf.date")}</label><input type="date" id="${id}-date" class="funding-date">
      <label for="${id}-amount">${T("bf.amount")}</label><input id="${id}-amount" class="funding-amount" inputmode="decimal">
      <label class="row" for="${id}-expected"><input type="checkbox" id="${id}-expected" class="funding-expected" style="width:24px;flex-shrink:0">${T("bf.expected")}</label>
      <p class="error" id="${id}-error"></p><button type="button" class="alink quiet">${T("bf.remove")}</button>`;
    const date = el.querySelector(".funding-date"), amount = el.querySelector(".funding-amount");
    date.min = context.today;
    date.setAttribute("aria-describedby", id + "-error"); amount.setAttribute("aria-describedby", id + "-error");
    if (event) { date.value = event.on; amount.value = minorText(event.minor, exponent()); el.querySelector(".funding-expected").checked = event.expected; }
    el.querySelector("button").onclick = () => { el.remove(); changed(); };
    routingRows.set(el, {
      id: event?.id || crypto.randomUUID(),
      controller: createCashRouting(el, id, event, context, changed,
        (raw, options) => minorAmount(raw, exponent(), options), n => minorText(n, exponent())),
    });
    return el;
  }
  $("funding-mode").addEventListener("change", sync);
  $("funding-add").onclick = () => {
    if ($("funding-events").children.length >= 36) { $("e-funding-events").textContent = T("bf.limit"); return; }
    const el = row(); $("funding-events").append(el); changed(); el.querySelector("input").focus();
  };
  return {
    load(funding, options = {}) {
      context={today:options.today||"",loans:options.loans||[],locked:!!options.locked};
      spentPeriodStart=funding?.spent_period_start||"";
      $("funding-mode").disabled=context.locked;
      $("funding-spent").readOnly=context.locked;
      cashThrough=funding?.cash_through||"";
      $("funding-mode").value = "separate";
      $("funding-monthly").value = funding ? minorText(funding.monthly_minor, exponent()) : "";
      $("funding-spent").value = funding ? minorText(funding.spent_minor, exponent()) : "";
      $("funding-events").replaceChildren(...(funding?.events || []).map(row)); sync();
    },
    read() {
      if ($("funding-mode").value === "legacy") return { value: null, ok: true };
      let ok = true;
      const value = { monthly_minor: 0, spent_minor: 0, events: [], cash_through:cashThrough, ...(spentPeriodStart?{spent_period_start:spentPeriodStart}:{}) };
      for (const [id, key] of [["funding-monthly", "monthly_minor"], ["funding-spent", "spent_minor"]]) {
        let error = "";
        try { value[key] = minorAmount($(id).value, exponent()); } catch (e) { error = e.message; ok = false; }
        $("e-" + id).textContent = error; $(id).setAttribute("aria-invalid", String(!!error));
      }
      const rows = [...$("funding-events").children];
      $("e-funding-events").textContent = rows.length > 36 ? T("bf.limit") : "";
      if (rows.length > 36) ok = false;
      for (const el of rows) {
        const date = el.querySelector(".funding-date"), amount = el.querySelector(".funding-amount");
        let error = "", minor;
        const badDate = !validDate(date.value) || !validDate(context.today) || date.value < context.today;
        if (badDate) error = T("bf.dateError");
        let badAmount = false;
        try { minor = minorAmount(amount.value, exponent(), { positive: true }); } catch (e) { error ||= e.message; badAmount = true; }
        const event={on:date.value,minor,expected:el.querySelector(".funding-expected").checked};
        const routing=routingRows.get(el);
        if (!error) {
          try { Object.assign(event, {id:routing.id}, routing.controller.read(event)); }
          catch(e) {error=e.message;}
        }
        date.setAttribute("aria-invalid", String(badDate)); amount.setAttribute("aria-invalid", String(badAmount));
        el.querySelector(".error").textContent = error;
        if (error) ok = false;
        else value.events.push(event);
      }
      return { value, ok };
    },
  };
}
