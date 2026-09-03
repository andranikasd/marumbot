"use strict";
import {planEvidence} from "./plan-evidence.js";
import {register} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {esc,fmtMoney,fmtDate} from '../core.js';
addStrings({
 'sc.title':'Ի՞նչ կլինի, եթե…','sc.monthly':'Ամսական բյուջե','sc.effective':'Ուժի մեջ մտնելու օր','sc.payday':'Եկամտի օր (0–31)','sc.reserve':'Պահուստ','sc.cash':'Միանվագ գումար','sc.cashdate':'Գումարի ստացման օր','sc.confirm':'Գումարը հաստատված է','sc.preview':'Հաշվարկել','sc.save':'Պահպանել տարբերակը','sc.activate':'Հաստատել բյուջեն և պլանը','sc.saved':'Պահպանված տարբերակներ','sc.empty':'Տարբերակներ դեռ չկան','sc.hint':'Դատարկ դաշտերը պահպանում են սկզբնական արժեքները։ Փոփոխությունները գործում են միայն հաստատելուց հետո։','sc.error':'Հաշվարկը հնարավոր չէ։ Ստուգեք տվյալները։','sc.conflict':'Տվյալները փոխվել են։ Բացեք նոր պլան և փորձեք կրկին։','sc.unsupported':'Այս փոփոխությունը դեռ չի աջակցվում։','sc.infeasible':'Բյուջեն կամ դրամական միջոցները չեն ծածկում վճարումները։','sc.done':'Բյուջեն և պլանը հաստատված են։','sc.outdated':'Սկզբնական տվյալները փոխվել են․ այս տարբերակը չի կարող հաստատվել։','sc.original':'Սկզբնական պլանի օր','sc.new':'Նոր տարբերակ','sc.currency':'Գումարները՝','sc.assumed':'Հաշվարկը ենթադրում է այս գումարի ստացումը։ Հաստատման համար ստեղծեք տարբերակ հաստատված գումարով։'
},{
 'sc.title':'What-if scenarios','sc.monthly':'Monthly budget','sc.effective':'Budget effective date','sc.payday':'Payday (0–31)','sc.reserve':'Reserve','sc.cash':'One-time cash','sc.cashdate':'Cash arrival date','sc.confirm':'This cash is confirmed','sc.preview':'Calculate preview','sc.save':'Save scenario','sc.activate':'Activate budget and plan','sc.saved':'Saved scenarios','sc.empty':'No saved scenarios yet','sc.hint':'Blank fields keep the original values. Changes take effect only when you activate.','sc.error':'Unable to calculate. Check the inputs and try again.','sc.conflict':'Inputs have changed. Open a fresh plan and try again.','sc.unsupported':'This change is not supported yet.','sc.infeasible':'The spending limit or available cash cannot cover payments.','sc.done':'Budget and plan activated.','sc.outdated':'Original inputs have changed. This scenario cannot be activated.','sc.original':'Original plan date','sc.new':'New scenario','sc.currency':'Amounts in','sc.assumed':'This preview assumes the cash arrives. To activate, create a scenario with confirmed cash.'
});
const $=id=>document.getElementById(id);
let source=null,view=null,request=null,activation=null,busy=false,generation=0;
const field=(id,key,type='text')=>`<label class="stack"><span data-i18n="${key}"></span><input id="sc-${id}" type="${type}" ${type==='text'?'inputmode="decimal"':''}></label>`;
async function call(path,body){
 const response=await api(path,body===undefined?{}:{method:'POST',body:JSON.stringify(body)});
 if(!response.ok){let data={};try{data=await response.json();}catch{}const error=new Error(data.error||'request');error.status=response.status;throw error;}
 return response.json();
}
function errorText(e){return T(e.status===409?'sc.conflict':e.message==='unsupported'?'sc.unsupported':e.message==='infeasible'?'sc.infeasible':'sc.error');}
// Parse decimal money without floating-point rounding; reject unsafe JSON integers.
export function scenarioMinor(value,exponent){
 if(!/^\d+(\.\d+)?$/.test(value))throw new Error('amount');
 const [whole,fraction='']=value.split('.');if(fraction.length>exponent)throw new Error('amount');
 const minor=BigInt(whole)*10n**BigInt(exponent)+BigInt(fraction.padEnd(exponent,'0')||'0');
 if(minor>BigInt(Number.MAX_SAFE_INTEGER))throw new Error('amount');return Number(minor);
}
function changes(){
 const c={};for(const [field,key] of [['monthly','monthly_minor'],['reserve','reserve_minor']]){const value=$('sc-'+field).value.trim();if(value)c[key]=scenarioMinor(value,source.currency_exponent);}
 if($('sc-effective').value)c.effective_from=$('sc-effective').value;
 if($('sc-payday').value!==''){const value=$('sc-payday').value;if(!/^\d+$/.test(value)||Number(value)>31)throw new Error('payday');c.pay_day=Number(value);}
 if($('sc-cashdate').value&&!$('sc-cash').value.trim())throw new Error('cash');
 if($('sc-cash').value.trim()){c.one_time_cash={minor:scenarioMinor($('sc-cash').value.trim(),source.currency_exponent),on:$('sc-cashdate').value,expected:!$('sc-confirm').checked};}
 return c;
}
function clearPreview(){view=null;request=null;activation=null;$('sc-result').hidden=true;$('sc-save').hidden=true;$('sc-activate').hidden=true;}
function render(v,saved){
 view=v;activation=null;const d=v.sheet;
 const changes=v.changes||{};const descriptions=[];const amount=n=>fmtMoney(n/10**d.currency_exponent,d.currency);
 for(const [key,label] of [['monthly_minor','sc.monthly'],['reserve_minor','sc.reserve']])if(changes[key]!==undefined)descriptions.push(T(label)+': '+amount(changes[key]));
 if(changes.effective_from)descriptions.push(T('sc.effective')+': '+fmtDate(changes.effective_from));
 if(changes.pay_day!==undefined)descriptions.push(T('sc.payday')+': '+changes.pay_day);
 if(changes.one_time_cash)descriptions.push(T('sc.cash')+': '+amount(changes.one_time_cash.minor)+' · '+fmtDate(changes.one_time_cash.on));
 $('sc-result').innerHTML=`<div class="card stack"><span>${esc(T('sc.original'))}: ${esc(fmtDate(d.as_of))}</span><div class="kv"><div><span>${esc(T('plan.debtfree'))}</span><b>${esc(fmtDate(d.summary.payoff_date))}</b></div><div><span>${esc(T('plan.interest'))}</span><b>${esc(fmtMoney(d.summary.interest_minor/10**d.currency_exponent,d.currency))}</b></div></div>${descriptions.map(text=>`<p>${esc(text)}</p>`).join('')}${changes.one_time_cash?.expected?`<p>${esc(T('sc.assumed'))}</p>`:''}${v.outdated?`<p>${esc(T('sc.outdated'))}</p>`:''}</div>${planEvidence(d)}`;
 $('sc-result').hidden=false;$('sc-save').hidden=saved;$('sc-activate').hidden=!saved||v.outdated||!!changes.one_time_cash?.expected;
}
async function history(){
 const data=await call('api/scenarios');$('sc-list').innerHTML=data.scenarios.map(s=>`<button class="card opt" type="button" data-scenario="${esc(s.id)}"><span>${esc(s.currency)} · ${esc(fmtDate(s.as_of))}</span></button>`).join('')||esc(T('sc.empty'));
}
function lock(on){busy=on;for(const el of document.querySelectorAll('#sc-form input,#sc-form button,#sc-save,#sc-activate,[data-scenario],#sc-new'))el.disabled=on;}
register({id:'plan-scenarios',parent:'plan',titleKey:'sc.title',html:`
 <p id="sc-error" class="error" role="alert"></p><p id="sc-status" role="status"></p>
 <button id="sc-new" class="alink" data-i18n="sc.new"></button>
 <form id="sc-form" class="card stack"><p data-i18n="sc.hint"></p><b id="sc-currency"></b>
 ${field('monthly','sc.monthly')}${field('effective','sc.effective','date')}${field('payday','sc.payday','number')}${field('reserve','sc.reserve')}${field('cash','sc.cash')}${field('cashdate','sc.cashdate','date')}
 <label><input id="sc-confirm" type="checkbox"><span data-i18n="sc.confirm"></span></label><button class="cta" type="submit" data-i18n="sc.preview"></button></form>
 <section id="sc-result" hidden></section><button id="sc-save" class="cta" hidden data-i18n="sc.save"></button><button id="sc-activate" class="cta" hidden data-i18n="sc.activate"></button>
 <h2 data-i18n="sc.saved"></h2><div id="sc-list" class="stack"></div>`,onMount(el){
 $('sc-form').addEventListener('input',clearPreview);
 $('sc-form').onsubmit=async e=>{e.preventDefault();if(busy||!source)return;lock(true);$('sc-error').textContent='';try{request={proposal:source.proposal,changes:changes()};const v=await call('api/scenarios/preview',request);request.result_hash=v.result_hash;render(v,false);}catch(e){clearPreview();$('sc-error').textContent=errorText(e);}finally{lock(false);}};
 $('sc-save').onclick=async()=>{if(busy||!request)return;lock(true);try{const v=await call('api/scenarios',request);render(v,true);await history();}catch(e){$('sc-error').textContent=errorText(e);}finally{lock(false);}};
 $('sc-activate').onclick=async()=>{if(busy||!view)return;lock(true);try{
  // Preserve the same key and revision after an uncertain network response.
  activation??={id:view.id,idempotency_key:crypto.randomUUID(),expected_revision:view.revision};
  await call('api/scenarios/'+view.id+'/activate',activation);invalidate('api/');$('sc-status').textContent=T('sc.done');$('sc-activate').hidden=true;
 }catch(e){$('sc-error').textContent=errorText(e);}finally{lock(false);}};
 el.addEventListener('click',async e=>{const b=e.target.closest('[data-scenario]');if(!b||busy)return;lock(true);clearPreview();try{render(await call('api/scenarios/'+b.dataset.scenario),true);$('sc-form').hidden=true;}catch(e){$('sc-error').textContent=errorText(e);}finally{lock(false);}});
 $('sc-new').onclick=()=>{clearPreview();$('sc-form').hidden=false;$('sc-status').textContent='';};
},async onShow(el,params){
 const token=++generation;clearPreview();$('sc-error').textContent='';$('sc-status').textContent='';$('sc-form').reset();$('sc-form').hidden=false;source=null;lock(true);
 try{const fresh=params?.sheet||await call('api/plan'+(params?.goal?'?goal='+encodeURIComponent(params.goal):''));if(token!==generation)return;if(!fresh.proposal)throw new Error('unavailable');source=fresh;$('sc-currency').textContent=T('sc.currency')+' '+fresh.currency;await history();}catch(e){$('sc-error').textContent=errorText(e);try{await history();}catch{}}finally{if(token===generation)lock(false);}
}});
