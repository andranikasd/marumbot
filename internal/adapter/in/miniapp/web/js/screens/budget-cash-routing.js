"use strict";
import { T, addStrings } from "../i18n.js";

addStrings({
 "br.mode":"Գումարի նշանակությունը", "br.pool":"Բաշխել ըստ պլանի", "br.loan":"Միայն մեկ վարկի համար", "br.split":"Բաժանել վարկերի միջև", "br.hold":"Պահպանել մինչև պայմանը",
 "br.target":"Վարկ", "br.add":"Ավելացնել բաժին", "br.remove":"Հեռացնել բաժինը", "br.amount":"Այս վարկին հատկացված գումար",
 "br.until":"Օգտագործել այս օրվանից (ոչ պարտադիր)", "br.minimum":"Կուտակման շեմ՝ յուրաքանչյուր վարկի համար (ոչ պարտադիր)",
 "br.opening":"Այս գումարն արդեն ներառված է հիմա առկա գումարի մեջ", "br.hint":"Նշանակված գումարը միայն հավելյալ վճարների համար է։ Այն չի ծածկում պարտադիր վճարները։ Բաժինների գումարը պետք է հավասար լինի մուտքին։ Պահման երկու պայմանն էլ պետք է կատարվեն։",
 "br.invalid":"Ստուգեք վարկերը, բաժինների ճշգրիտ գումարը և պահման պայմանները։", "br.stale":"Այս նշանակման օրը անցել է։ Նշեք այսօր մնացած գումարն ու այսօրվա օրը կամ բացահայտ հեռացրեք նշանակումը։ Մի՛ կրկնեք հին մուտքը որպես նոր գումար։", "br.choose":"Ընտրեք վարկը",
}, {
 "br.mode":"Use this cash", "br.pool":"Allocate according to the plan", "br.loan":"Earmark for one loan", "br.split":"Split between loans", "br.hold":"Hold before allocating",
 "br.target":"Loan", "br.add":"Add split", "br.remove":"Remove split", "br.amount":"Amount reserved for this loan",
 "br.until":"Use on or after (optional)", "br.minimum":"Accumulated threshold per loan (optional)",
 "br.opening":"This amount is already included in cash available now", "br.hint":"Restricted cash funds optional payments only. It cannot cover required payments. Split amounts must equal the receipt. Both hold conditions must be met when specified.",
 "br.invalid":"Check the loan targets, exact split total, and hold conditions.", "br.stale":"This routing date has passed. Declare the amount still held with today's date, or explicitly remove this restriction. Do not repeat an old receipt as new cash.", "br.choose":"Choose a loan",
});

// Exact decimal parsing is injected from the funding form; split sums use
// integer BigInt, never rounded major-unit arithmetic.
export function routingValue({mode, loanID, splits, until, minimum, fromOpening}, {minor, on, expected}, today, parse, eligible) {
 if (mode === "pool") {
  if (fromOpening) throw new Error(T("br.invalid"));
  return {};
 }
 const routing = {};
 if (mode === "loan") {
  if (!eligible.has(loanID)) throw new Error(T("br.invalid"));
  routing.loan_id = loanID;
 } else if (mode === "split") {
  const seen = new Set(); let total = 0n;
  routing.splits = splits.map(split => {
   if (!eligible.has(split.loanID) || seen.has(split.loanID)) throw new Error(T("br.invalid"));
   seen.add(split.loanID);
   const amount = parse(split.amount, {positive:true}); total += BigInt(amount);
   return {loan_id:split.loanID, minor:amount};
  });
  if (!routing.splits.length || total !== BigInt(minor)) throw new Error(T("br.invalid"));
 } else if (mode !== "hold") throw new Error(T("br.invalid"));
 if (until) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(until) || until < on || Number.isNaN(Date.parse(until+"T00:00:00Z")) || new Date(until+"T00:00:00Z").toISOString().slice(0,10)!==until) throw new Error(T("br.invalid"));
  routing.hold_until = until;
 }
 if (minimum.trim()) routing.hold_minimum_minor = parse(minimum, {positive:true});
 if (mode === "hold" && !until && routing.hold_minimum_minor == null) throw new Error(T("br.invalid"));
 if (fromOpening && (expected || on !== today)) throw new Error(T("br.invalid"));
 return {routing, ...(fromOpening ? {from_opening:true} : {})};
}

export function createCashRouting(root, id, event, context, changed, parse, format) {
 const block=document.createElement("div"); block.className="stack";
 const input=(suffix,key,type="text")=>`<label for="${id}-${suffix}">${T(key)}</label><input id="${id}-${suffix}" type="${type}" class="routing-${suffix}">`;
 block.innerHTML=`<label for="${id}-mode">${T("br.mode")}</label><select id="${id}-mode" class="routing-mode"></select>
 <div class="routing-target-field" hidden><label for="${id}-target">${T("br.target")}</label><select id="${id}-target" class="routing-target"></select></div>
 <div class="routing-split-fields stack" hidden><div class="routing-splits stack"></div><button type="button" class="alink routing-add">${T("br.add")}</button></div>
 <div class="routing-holds stack" hidden>${input("until","br.until","date")}${input("minimum","br.minimum")}<label class="row" for="${id}-opening"><input type="checkbox" id="${id}-opening" class="routing-opening">${T("br.opening")}</label><p class="hint">${T("br.hint")}</p></div><p class="hint routing-stale" hidden>${T("br.stale")}</p>`;
 root.append(block);
 const $=cls=>block.querySelector(".routing-"+cls);
 for (const [value,key] of [["pool","br.pool"],["loan","br.loan"],["split","br.split"],["hold","br.hold"]]) {
  const option=document.createElement("option");option.value=value;option.textContent=T(key);$("mode").append(option);
 }
 const eligible=new Set(context.loans.map(l=>l.id));
 function targets(select, selected="") {
  const blank=document.createElement("option");blank.value="";blank.textContent=T("br.choose");select.append(blank);
  for (const loan of context.loans) {const option=document.createElement("option");option.value=loan.id;option.textContent=loan.name;select.append(option);}
  // Never substitute a different loan when a former target is unavailable.
  select.value=selected;
 }
 targets($("target"),event?.routing?.loan_id);
 let next=0;
 function splitRow(value={}) {
  const row=document.createElement("div");row.className="stack";
  const sid=id+"-split-"+next++;
  row.innerHTML=`<label for="${sid}-loan">${T("br.target")}</label><select id="${sid}-loan"></select><label for="${sid}-amount">${T("br.amount")}</label><input id="${sid}-amount" inputmode="decimal"><button class="alink quiet" type="button">${T("br.remove")}</button>`;
  targets(row.querySelector("select"),value.loan_id);
  row.querySelector("input").value=value.minor==null?"":format(value.minor);
  row.querySelector("button").onclick=()=>{row.remove();changed();};
  $("splits").append(row);
 }
 $("add").onclick=()=>{if($("splits").children.length<context.loans.length){splitRow();changed();}};
 const r=event?.routing;
 $("mode").value=r?.loan_id?"loan":r?.splits?.length?"split":r?"hold":"pool";
 for(const split of r?.splits||[]) splitRow(split);
 $("until").value=r?.hold_until||"";
 $("minimum").value=r?.hold_minimum_minor==null?"":format(r.hold_minimum_minor);
 $("minimum").inputMode="decimal";
 $("opening").checked=!!event?.from_opening;
 $("stale").hidden=!(r && event.on<context.today);
 function sync(){const mode=$("mode").value;$("target-field").hidden=mode!=="loan";$("split-fields").hidden=mode!=="split";$("holds").hidden=mode==="pool";if(mode==="split"&&!$("splits").children.length)splitRow();}
 $("mode").onchange=sync;sync();
 return {
  read(base){return routingValue({mode:$("mode").value,loanID:$("target").value,splits:[...$("splits").children].map(row=>({loanID:row.querySelector("select").value,amount:row.querySelector("input").value})),until:$("until").value,minimum:$("minimum").value,fromOpening:$("opening").checked},base,context.today,parse,eligible);},
 };
}
