"use strict";
import { T, addStrings } from "../i18n.js";
import { api, invalidate } from "../api.js";
import { register, currentScreen } from "../nav.js";
import { minorAmount, minorText, validDate, validMonth } from "./budget-funding.js";

addStrings({
 "bp.carry":"Չօգտագործված գումար", "bp.noCarry":"Հավելյալ գումարը թողնել տնային բյուջեում", "bp.carryCash":"Պահպանել վարկերի համար", "bp.batch":"Կուտակել մինչև նվազագույն հավելյալ վճար", "bp.until":"Պահպանել մինչև ամսաթիվ", "bp.minimum":"Նվազագույն հավելյալ մայր գումար", "bp.untilDate":"Հավելյալ վճարի ամենավաղ ամսաթիվ", "bp.release":"Բանկի հաստատած վճարի նվազումից հետո", "bp.rollAll":"Պահպանել ամբողջ սահմանաչափը", "bp.releaseAll":"Ազատել նվազած վճարն ամբողջությամբ", "bp.rollAmount":"Պահպանել հաստատուն մաս՝ յուրաքանչյուր նվազումից", "bp.rollPercent":"Պահպանել տոկոս՝ յուրաքանչյուր նվազումից", "bp.retain":"Պահպանվող գումար կամ տոկոս", "bp.releaseHint":"Միայն կանոնը հաստատելուց հետո բանկի ստուգված քաղվածքները կարող են նվազեցնել հաջորդ շրջանի սահմանաչափը։ Աճը կիրառվում է սկզբնական սահմանաչափին, ապա հանվում են ազատված վճարները։ Ամսվա փոխարինումը մնում է վերջնական սահմանաչափը։",
 "bp.title":"Բյուջեի կանոններ", "bp.effective":"Ուժի մեջ է մտնում", "bp.limit":"Պարբերական սահմանաչափ",
 "bp.cycle":"Հաշվարկային շրջանի սկիզբը (ամսվա օր)", "bp.growth":"Աճ", "bp.none":"Առանց աճի",
 "bp.fixed":"Հաստատուն հավելում", "bp.percent":"Տոկոսային աճ", "bp.increase":"Աճի չափը",
 "bp.frequency":"Ամիսների միջակայք", "bp.start":"Առաջին աճի ամսաթիվ", "bp.end":"Աճի ավարտ (ոչ պարտադիր)",
 "bp.cap":"Առավելագույն սահմանաչափ (ոչ պարտադիր)", "bp.adjust":"Ամսվա փոփոխություններ", "bp.replace":"Փոխարինել սահմանաչափը",
 "bp.delta":"Ավելացնել կամ նվազեցնել", "bp.add":"Ավելացնել ամիս", "bp.remove":"Հեռացնել", "bp.save":"Հաստատել նոր տարբերակը",
 "bp.note":"Կանոնը փոխում է ծախսի թույլտվությունը։ Այն չի ավելացնում հասանելի գումար և չի զրոյացնում արդեն ծախսվածը։",
 "bp.rules":"Չօգտագործված ֆինանսավորված գումարը պահպանվում է։ Վարկի մարումից հետո ընդհանուր սահմանաչափը պահպանվում է։",
 "bp.assumption":"Աճը ենթադրություն է։ Համեմատեք նաև առանց աճի պլանը։ Աճի ամսաթիվը պետք է լինի հաշվարկային շրջանի սկիզբը։",
 "bp.funding":"Նախ առանձին նշեք ֆինանսավորումը բյուջեի ձևում։", "bp.reload":"Վերբեռնել և հեռացնել ձևի փոփոխությունները",
 "bp.goalUnsupported":"Նպատակին հասնելուց հետո գումարի ազատումը դեռ հասանելի չէ․ նպատակը և դրա հաստատման աղբյուրը սահմանված չեն։ Ընտրեք ցուցադրված ազատման կանոններից մեկը։",
 "bp.unsupported":"Այս կանոնը չի աջակցվում։ Ընտրեք ցուցադրված տարբերակներից մեկը և ստուգեք կանոնի պարամետրերը։",
 "bp.retry":"Կրկնել պահպանումը", "bp.uncertain":"Պահպանման արդյունքը հայտնի չէ։ Կրկնեք նույն հարցումը՝ նախքան փոփոխելը։",
 "bp.conflict":"Բյուջեն փոխվել է։ Վերբեռնեք պատմությունը նախքան կրկին հաստատելը։",
 "bp.invalid":"Ստուգեք դաշտերը։ Այս կանոնը չի ընդունվել։", "bp.history":"Հաստատված տարբերակներ", "bp.saved":"Տարբերակը պահպանվել է։",
}, {
 "bp.carry":"Unused cash", "bp.noCarry":"Return optional cash to household", "bp.carryCash":"Keep cash for debt", "bp.batch":"Wait for a minimum prepayment", "bp.until":"Hold until a date", "bp.minimum":"Minimum extra principal", "bp.untilDate":"Earliest optional payment date", "bp.release":"After a bank-confirmed payment reduction", "bp.rollAll":"Keep the full spending limit", "bp.releaseAll":"Release the full reduction", "bp.rollAmount":"Retain an amount per reduction", "bp.rollPercent":"Retain a percentage per reduction", "bp.retain":"Retained amount or percentage", "bp.releaseHint":"Only verified bank statements captured after approval reduce a future period’s limit. Growth applies to the original limit, then released payments are deducted. A period replacement remains its final limit.",
 "bp.title":"Budget rules", "bp.effective":"Effective from", "bp.limit":"Recurring spending limit",
 "bp.cycle":"Cycle start (day of month)", "bp.growth":"Growth", "bp.none":"No growth",
 "bp.fixed":"Fixed increase", "bp.percent":"Percentage increase", "bp.increase":"Increase",
 "bp.frequency":"Every N months", "bp.start":"First increase date", "bp.end":"Growth end (optional)",
 "bp.cap":"Maximum limit (optional)", "bp.adjust":"Period adjustments", "bp.replace":"Replace limit",
 "bp.delta":"Add or subtract", "bp.add":"Add period", "bp.remove":"Remove", "bp.save":"Approve new version",
 "bp.note":"This changes spending permission. It adds no available cash and does not reset spending already made.",
 "bp.rules":"Unused funded cash carries forward. The total spending limit stays unchanged after a loan closes.",
 "bp.assumption":"Growth is an assumption. Compare the no-growth plan too. Increases must start at a budget-cycle boundary.",
 "bp.funding":"First declare funding separately in the budget form.", "bp.reload":"Reload and discard form changes",
 "bp.goalUnsupported":"Release after reaching a goal is not available yet: the target, scope and confirming source are not defined. Choose one of the listed release rules.",
 "bp.unsupported":"This rule is unsupported. Choose a listed option and check the rule parameters.",
 "bp.retry":"Retry save", "bp.uncertain":"The save outcome is unknown. Retry the same request before editing.",
 "bp.conflict":"The budget changed. Reload the history before approving again.",
 "bp.invalid":"Check the fields. This rule was not accepted.", "bp.history":"Approved versions", "bp.saved":"Version saved.",
});

const field = (id, key, type = "text") => `<div class="field"><label for="bp-${id}" data-i18n="bp.${key}"></label><input id="bp-${id}" type="${type}"></div>`;
const options=(items)=>items.map(([value,key])=>`<option value="${value}" data-i18n="bp.${key}"></option>`).join("");
const html = `<form id="bp-form" class="stack">
 <p data-i18n="bp.note"></p><p id="bp-status" role="status"></p>
 <button type="button" id="bp-reload" class="alink" data-i18n="bp.reload"></button>
 <button type="button" id="bp-retry" class="cta" data-i18n="bp.retry" hidden></button>
 <fieldset id="bp-fields" class="card stack" disabled>
 ${field("effective","effective","date")}${field("limit","limit")}${field("cycle","cycle","number")}
 <div class="field"><label for="bp-growth" data-i18n="bp.growth"></label><select id="bp-growth">
 <option value="none" data-i18n="bp.none"></option><option value="fixed" data-i18n="bp.fixed"></option><option value="percent" data-i18n="bp.percent"></option></select></div>
 <div id="bp-growth-fields" class="stack" hidden>${field("increase","increase")}${field("frequency","frequency","number")}${field("start","start","date")}${field("end","end","date")}${field("cap","cap")}<p data-i18n="bp.assumption"></p></div>
 <span data-i18n="bp.adjust"></span><div id="bp-adjustments" class="stack"></div>
 <button type="button" id="bp-add" class="alink" data-i18n="bp.add"></button>
 <label for="bp-carry" data-i18n="bp.carry"></label><select id="bp-carry">${options([["carry_cash","carryCash"],["no_carry","noCarry"],["batch_until","batch"],["carry_to_date","until"]])}</select>
 <div id="bp-minimum-fields" hidden>${field("minimum","minimum")}</div><div id="bp-until-fields" hidden>${field("until","untilDate","date")}</div>
 <label for="bp-release" data-i18n="bp.release"></label><select id="bp-release">${options([["roll_all","rollAll"],["release_all","releaseAll"],["roll_amount","rollAmount"],["roll_percent","rollPercent"]])}</select>
 <div id="bp-retain-fields" hidden>${field("retain","retain")}</div><p data-i18n="bp.releaseHint"></p><p class="hint" data-i18n="bp.goalUnsupported"></p><button type="submit" class="cta" data-i18n="bp.save"></button>
 </fieldset><h3 data-i18n="bp.history"></h3><div id="bp-history" class="stack"></div></form>`;
const $ = id => document.getElementById("bp-" + id);
let documentState, busy = false, dirty = false, conflict = false, pending = null;
function controls(){
 $("fields").disabled=busy||!!pending||conflict||!documentState?.explicit_funding;
 $("reload").disabled=busy||!!pending;
 $("retry").hidden=!pending;
 $("retry").disabled=busy;
 $("form").setAttribute("aria-busy",String(busy));
}
function percentText(ppb) {
 if (!Number.isSafeInteger(ppb) || ppb<0) throw new Error(T("bp.invalid"));
 const digits=String(ppb).padStart(8,"0"); return digits.slice(0,-7)+"."+digits.slice(-7);
}
function adjustment(value = {}) {
 const row = document.createElement("div"); row.className = "stack";
 const month = document.createElement("input"); month.type = "month"; month.value = value.month || ""; month.setAttribute("aria-label",T("bp.adjust"));
 const operation = document.createElement("select");
 for (const [value,key] of [["replacement_minor","replace"],["delta_minor","delta"]]) {
 const option = document.createElement("option"); option.value=value; option.textContent=T("bp."+key); operation.append(option);
 }
 operation.setAttribute("aria-label",T("bp.adjust"));
 const amount = document.createElement("input"); amount.inputMode="decimal"; amount.setAttribute("aria-label",T("bp.limit"));
 operation.value=value.delta_minor!=null ? "delta_minor" : "replacement_minor";
 const n=value[operation.value];
 if (n!=null) amount.value=(n<0?"-":"")+minorText(Math.abs(n),documentState.currency_exponent);
 const remove=document.createElement("button"); remove.type="button"; remove.textContent=T("bp.remove"); remove.className="alink";
 remove.onclick=()=>{row.remove();dirty=true;};
 row.append(month,operation,amount,remove); return row;
}
async function load(force = false) {
 if (pending) {controls();return;}
 if (busy || ((dirty||conflict) && !force)) return;
 busy=true; controls();
 try {
 const response=await api("api/budget/policies",{cache:"no-store"});
 if (!response.ok) throw new Error(T("err.load"));
 const b=await response.json();
 if (currentScreen()!=="budget-policy") return;
 if (!Number.isSafeInteger(b.version) || !validDate(b.today) || !Array.isArray(b.policies ?? [])) throw new Error(T("err.load"));
 minorText(b.monthly_minor,b.currency_exponent);
 documentState=b; conflict=false; pending=null; $("retry").hidden=true; dirty=false;
 const policies=b.policies || [], latest=policies.at(-1);
 $("effective").value=b.today; $("effective").min=b.today;
 $("limit").value=minorText(latest?.monthly_minor ?? b.monthly_minor,b.currency_exponent);
 $("cycle").value=latest?.cycle_day || 1; $("cycle").min=1; $("cycle").max=31;
 $("cycle").disabled=policies.length>0;
 const growth=latest?.growth;
 $("growth").value=growth ? (growth.fixed_minor!=null ? "fixed":"percent") : "none";
 $("increase").value=growth ? (growth.fixed_minor!=null?minorText(growth.fixed_minor,b.currency_exponent):percentText(growth.percent_ppb)) : "";
 $("frequency").value=growth?.every_months || 1; $("frequency").min=1;
 $("start").value=growth?.starts_on || ""; $("end").value=growth?.ends_on || "";
 $("cap").value=growth?.maximum_minor==null?"":minorText(growth.maximum_minor,b.currency_exponent);
 $("growth-fields").hidden=!growth;
 $("carry").value=latest?.carry_rule || "carry_cash";
 $("minimum").value=latest?.carry_minimum_minor==null?"":minorText(latest.carry_minimum_minor,b.currency_exponent);
 $("until").value=latest?.carry_until || "";
 $("release").value=latest?.released_payment_rule || "roll_all";
 $("retain").value=latest?.retain_minor!=null?minorText(latest.retain_minor,b.currency_exponent):latest?.retain_percent_ppb!=null?percentText(latest.retain_percent_ppb):"";
 syncRules();
 $("adjustments").replaceChildren(...(latest?.adjustments || []).map(adjustment));
 $("history").replaceChildren(...policies.map(p=>{
 const item=document.createElement("div");item.className="card";
 item.textContent=`${p.effective_from} · ${minorText(p.monthly_minor,b.currency_exponent)} ${b.currency} · #${p.version}`; return item;
 }));
 $("status").textContent=b.explicit_funding?"":T("bp.funding");
 } catch(e) { documentState=null; $("status").textContent=e.message; }
 finally {busy=false;controls();}
}
function percentage(value) {
 const raw=value.trim().replace(",", ".");
 if (!/^\d+(\.\d{1,7})?$/.test(raw)) throw new Error(T("bp.invalid"));
 const [whole,fraction=""]=raw.split(".");
 const ppb=BigInt(whole)*10000000n+BigInt(fraction.padEnd(7,"0"));
 if (ppb>9007199254740991n) throw new Error(T("bp.invalid"));
 return Number(ppb);
}
function syncRules() {
 $("minimum-fields").hidden=$("carry").value!=="batch_until";
 $("until-fields").hidden=$("carry").value!=="carry_to_date";
 $("retain-fields").hidden=!["roll_amount","roll_percent"].includes($("release").value);
}
function read() {
 const b=documentState, exp=b.currency_exponent;
 const policy={effective_from:$("effective").value,monthly_minor:minorAmount($("limit").value,exp),cycle_day:Number($("cycle").value),carry_rule:$("carry").value,released_payment_rule:$("release").value,adjustments:[]};
 if (!validDate(policy.effective_from) || policy.effective_from<b.today || !Number.isInteger(policy.cycle_day) || policy.cycle_day<1 || policy.cycle_day>31) throw new Error(T("bp.invalid"));
 if (policy.carry_rule==="batch_until") policy.carry_minimum_minor=minorAmount($("minimum").value,exp,{positive:true});
 if (policy.carry_rule==="carry_to_date") {
 policy.carry_until=$("until").value;
 if (!validDate(policy.carry_until) || policy.carry_until<policy.effective_from) throw new Error(T("bp.invalid"));
 }
 if (policy.released_payment_rule==="roll_amount") policy.retain_minor=minorAmount($("retain").value,exp);
 if (policy.released_payment_rule==="roll_percent") {
 policy.retain_percent_ppb=percentage($("retain").value);
 if (policy.retain_percent_ppb>1000000000) throw new Error(T("bp.invalid"));
 }
 const mode=$("growth").value;
 if (mode!=="none") {
 const g={every_months:Number($("frequency").value),starts_on:$("start").value};
 if (!Number.isSafeInteger(g.every_months) || g.every_months<1 || !validDate(g.starts_on) || (policy.cycle_day===1 && !g.starts_on.endsWith("-01")) || g.starts_on<policy.effective_from) throw new Error(T("bp.invalid"));
 if (mode==="fixed") g.fixed_minor=minorAmount($("increase").value,exp);
 else {
 // Percentage text -> PPB via decimal digits; no floating money arithmetic.
 g.percent_ppb=percentage($("increase").value);
 }
 if ($("end").value) {g.ends_on=$("end").value;if (!validDate(g.ends_on) || g.ends_on<g.starts_on) throw new Error(T("bp.invalid"));}
 if ($("cap").value.trim()) g.maximum_minor=minorAmount($("cap").value,exp);
 policy.growth=g;
 }
 const seen=new Set();
 if ($("adjustments").children.length>36) throw new Error(T("bp.invalid"));
 for (const row of $("adjustments").children) {
 const [month,operation,amount]=row.children;
 if (!validMonth(month.value) || seen.has(month.value)) throw new Error(T("bp.invalid"));
 seen.add(month.value);
 const raw=amount.value.trim(), negative=raw.startsWith("-");
 if (negative && operation.value!=="delta_minor") throw new Error(T("bp.invalid"));
 const minor=minorAmount(negative?raw.slice(1):raw,exp);
 policy.adjustments.push({month:month.value,[operation.value]:negative?-minor:minor});
 }
 return {currency:b.currency,expected_version:b.version,policy};
}
register({id:"budget-policy",parent:"budget",titleKey:"bp.title",html,
 onMount() {
 $("form").addEventListener("input",()=>{dirty=true;});
 $("growth").onchange=()=>{$("growth-fields").hidden=$("growth").value==="none";dirty=true;};
 $("add").onclick=()=>{if ($("adjustments").children.length<36) {$("adjustments").append(adjustment());dirty=true;}};
 $("reload").onclick=()=>load(true);
 $("carry").onchange=$("release").onchange=()=>{syncRules();dirty=true;};
 const submit=async event=>{
 event.preventDefault();if (busy || conflict || !documentState?.explicit_funding) return;
 if (!pending) {
 try {pending=JSON.stringify({...read(),idempotency_key:crypto.randomUUID()});}
 catch(e) {$("status").textContent=e.message;return;}
 }
 busy=true;controls();
 try {
 const response=await api("api/budget/policies",{method:"POST",body:pending});
 if (!response.ok && !(response.status>=400 && response.status<500)) throw new Error(T("bp.uncertain"));
 if (!response.ok) {
 pending=null;conflict=response.status===409;
 dirty=true;
 let key=response.status===409?"bp.conflict":"bp.invalid";
 try {const body=await response.json();
  if(JSON.stringify(body).includes('until_goal_then_release'))key='bp.goalUnsupported';
  else if(body.error==='unsupported'||body.error==='unsupported_policy')key='bp.unsupported';
 } catch { /* Older servers may return plain text for a definitive refusal. */ }
 throw new Error(T(key));
 }
 pending=null;conflict=false;dirty=false;invalidate("api/");documentState=null;$("status").textContent=T("bp.saved");
 } catch(e) {$("status").textContent=pending?T("bp.uncertain"):e.message;}
 finally {busy=false;controls();}
 };
 $("form").onsubmit=submit;$("retry").onclick=submit;
 },onShow(){return load();}
});
