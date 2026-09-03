"use strict";
import {register,currentScreen,go} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {esc,fmtMoney,fmtDate} from '../core.js';

addStrings({
 'compare.title':'Մեթոդների համեմատություն','compare.intro':'Նույն վարկերը, դրամական միջոցները և ծախսման սահմանները։ Մեթոդները տարբերվում են սկզբնական առաջնահերթությամբ։',
 'compare.proposed':'Առաջարկված պլան','compare.highest_rate':'Ամենաբարձր տոկոսադրույքը','compare.snowball':'Ամենափոքր մարման գումարը','compare.hybrid':'Մնացորդ / տոկոսադրույք','compare.highest_required':'Ամենաբարձր պարտադիր վճարը','compare.highest_interest':'Հաջորդ շրջանի ամենաբարձր տոկոսագումարը','compare.cashflow_index':'Մարում / ազատվող պարտադիր վճար','compare.avalanche':'Ստուգված տոկոսային առաջնահերթություն','compare.utilisation':'Վարկային սահմանաչափի օգտագործում',
 'compare.loading':'Համեմատվում է…','compare.refresh':'Թարմացնել համեմատությունը','compare.select':'Ընտրել մեթոդ','compare.activate':'Ակտիվացնել ընտրված պլանը','compare.activated':'Ընտրված պլանն ակտիվացվել է։','compare.stale':'Տվյալները կամ ակտիվ պլանը փոխվել են։ Թարմացրեք համեմատությունը։','compare.failed':'Չհաջողվեց ավարտել հարցումը։ Փորձեք կրկին։','compare.unavailable':'Այս մեթոդն այս տվյալներով հասանելի չէ։','compare.limit':'Պահանջվում է ստուգված շրջանառու վարկի սահմանաչափ։','compare.infeasible':'Այս մեթոդը չի տեղավորվում հասանելի միջոցների կամ ծախսման սահմանների մեջ։','compare.avalanche_refusal':'Վճարները կամ վարկատուի կանոնները թույլ չեն տալիս ստուգված տոկոսային դասակարգում։','compare.cost':'Տոկոսներ և վճարներ','compare.interest':'Տոկոսներ','compare.fees':'Վճարներ','compare.payoff':'Պարտքերից ազատ','compare.required':'Առավելագույն պարտադիր վճար','compare.delta':'Տարբերությունն առաջարկված պլանից','compare.months':'ամիս','compare.preview':'Դիտումը չի փոխում ակտիվ պլանը։','compare.choose':'Նախ ընտրեք հասանելի մեթոդ։'
},{
 'compare.title':'Compare methods','compare.intro':'The same loans, funding and spending limits. Methods differ in their starting priority.',
 'compare.proposed':'Proposed plan','compare.highest_rate':'Highest rate','compare.snowball':'Smallest payoff first','compare.hybrid':'Balance / interest rate','compare.highest_required':'Highest required payment','compare.highest_interest':'Highest next-period interest','compare.cashflow_index':'Payoff / released payment','compare.avalanche':'Verified rate priority','compare.utilisation':'Highest credit utilisation',
 'compare.loading':'Comparing…','compare.refresh':'Refresh comparison','compare.select':'Select a method','compare.activate':'Activate selected plan','compare.activated':'The selected plan is now active.','compare.stale':'Inputs or the active plan have changed. Refresh the comparison.','compare.failed':'The request could not be completed. Please retry.','compare.unavailable':'This method is unavailable for these inputs.','compare.limit':'Requires a verified revolving credit limit.','compare.infeasible':'This method cannot meet the funding or spending constraints.','compare.avalanche_refusal':'Fees or lender rules prevent a verified rate ordering.','compare.cost':'Interest and fees','compare.interest':'Interest','compare.fees':'Fees','compare.payoff':'Debt-free date','compare.required':'Peak required payment','compare.delta':'Difference from proposed plan','compare.months':'months','compare.preview':'Previewing leaves your active plan unchanged.','compare.choose':'Select an available method first.'
});

let sheet=null,selected=null,loading=false,activating=false,epoch=0;
const requests=new Map();
const $=id=>document.getElementById(id);
const money=value=>esc(fmtMoney(value/10**sheet.currency_exponent,sheet.currency));
function refusal(row){
 if(row.strategy==='utilisation')return T('compare.limit');
 if(row.refusal==='infeasible')return T('compare.infeasible');
 if(row.strategy==='avalanche')return T('compare.avalanche_refusal');
 return T('compare.unavailable');
}
function render(){
 $('comparison-rows').innerHTML=sheet.rows.map((row,i)=>{
  const available=!!row.proposal&&!!row.summary&&!row.refusal;
  const s=row.summary;
  return `<article class="card stack"><label><input type="radio" name="comparison-method" value="${i}" ${selected===i?'checked':''} ${available?'':'disabled'}> <strong>${esc(T('compare.'+row.strategy))}</strong></label>${available?`
   <div class="kv"><div><span>${esc(T('compare.payoff'))}</span><b>${esc(fmtDate(s.payoff_date))}</b></div><div><span>${esc(T('compare.cost'))}</span><b class="num">${money(s.cost_minor)}</b></div><div><span>${esc(T('compare.interest'))}</span><b class="num">${money(s.interest_minor)}</b></div><div><span>${esc(T('compare.fees'))}</span><b class="num">${money(s.fees_minor)}</b></div><div><span>${esc(T('compare.required'))}</span><b class="num">${money(s.peak_required_minor)}</b></div></div>
   ${row.strategy==='proposed'?'':`<p class="hint">${esc(T('compare.delta'))}: ${s.cost_delta_minor>0?'+':''}${money(s.cost_delta_minor)} · ${s.months_delta>0?'+':''}${s.months_delta} ${esc(T('compare.months'))}</p>`}`:`<p class="hint">${esc(refusal(row))}</p>`}</article>`;
 }).join('');
 $('comparison-activate').disabled=selected===null||loading||activating;
}
async function read(path){
 const response=await api(path,{cache:'no-store'});
 if(!response.ok){const error=new Error('comparison request');error.status=response.status;throw error;}
 return response.json();
}
async function load(){
 if(activating)return;
 const current=++epoch;loading=true;sheet=null;selected=null;
 $('comparison-rows').textContent='';$('comparison-message').textContent=T('compare.loading');$('comparison-error').textContent='';$('comparison-activate').disabled=true;$('comparison-refresh').disabled=true;
 try{
  // Always obtain a fresh, user-scoped proposal before asking for comparisons.
  // No offline cached proposal may become an activation command.
  const proposed=await read('api/plan');
  if(!proposed.proposal)throw new Error('proposal unavailable');
  const data=await read('api/plan/comparisons?proposal='+encodeURIComponent(proposed.proposal));
  if(current!==epoch)return;
  sheet=data;$('comparison-message').textContent='';render();
 }catch(error){if(current===epoch){$('comparison-message').textContent='';$('comparison-error').textContent=T(error.status===409?'compare.stale':'compare.failed');}}
 finally{if(current===epoch){loading=false;$('comparison-refresh').disabled=false;}}
}
async function activate(){
 if(activating||loading||selected===null||!sheet)return;
 const row=sheet.rows[selected];if(!row?.proposal||!row.summary||row.refusal)return;
 const identity=row.proposal+':'+sheet.active_revision;
 if(!requests.has(identity))requests.set(identity,{proposal:row.proposal,expected_revision:sheet.active_revision,idempotency_key:crypto.randomUUID()});
 const command=requests.get(identity);
 activating=true;$('comparison-activate').disabled=true;$('comparison-refresh').disabled=true;$('comparison-error').textContent='';
 document.querySelectorAll('[name="comparison-method"]').forEach(el=>{el.disabled=true;});
 try{
  const response=await api('api/plans/activate',{method:'POST',body:JSON.stringify(command)});
  if(!response.ok){const error=new Error('activation request');error.status=response.status;throw error;}
  await response.json();invalidate('api/plan');
  // A successful command is never silently applied to a subsequently selected row.
  selected=null;sheet=null;
  if(currentScreen()==='plan-comparison'){$('comparison-message').textContent=T('compare.activated');$('comparison-rows').textContent='';go('plan',{resetGoal:true});}
 }catch(error){
  if(error.status===409){selected=null;sheet=null;$('comparison-rows').textContent='';}
  $('comparison-error').textContent=T(error.status===409?'compare.stale':'compare.failed');
 }finally{
  activating=false;$('comparison-refresh').disabled=false;if(sheet)render();else $('comparison-activate').disabled=true;
 }
}

register({
 id:'plan-comparison',parent:'plan',titleKey:'compare.title',
 html:`<p data-i18n="compare.intro" class="hint"></p><p data-i18n="compare.preview" class="hint"></p><p id="comparison-message" role="status" aria-live="polite"></p><p id="comparison-error" class="error" role="alert"></p><fieldset class="stack"><legend data-i18n="compare.select"></legend><div id="comparison-rows" class="stack"></div></fieldset><div class="stack"><button id="comparison-activate" class="primary" data-i18n="compare.activate" disabled></button><button id="comparison-refresh" class="alink" data-i18n="compare.refresh"></button></div>`,
 onMount(el){
  el.addEventListener('change',event=>{if(event.target.name!=='comparison-method'||activating||loading)return;selected=Number(event.target.value);$('comparison-activate').disabled=false;});
  $('comparison-activate').addEventListener('click',activate);$('comparison-refresh').addEventListener('click',load);
 },onShow:load
});
