"use strict";
import {register,go} from '../nav.js';
import {getJSON,api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {fmtMoney,fmtFull,esc,confirmDialog} from '../core.js';
import {icon} from '../icons.js';
addStrings({'tab.activity':'Պատմություն','activity.balance':'Մնացորդի գրառում','activity.empty':'Գրառումներ դեռ չկան','activity.latest':'Գրառումներ','activity.more':'Ավելի հին գրառումներ','activity.all':'Բոլորը','activity.payments':'Վճարումներ'},{'tab.activity':'Activity','activity.balance':'Balance statement','activity.empty':'No records yet','activity.latest':'Records','activity.more':'Older records','activity.all':'All','activity.payments':'Payments'});
addStrings({'payment.status.reconciled':'Համադրված','activity.reconcile':'Համադրել'},{'payment.status.reconciled':'Reconciled','activity.reconcile':'Reconcile'});
addStrings({'activity.actuals':'Ամսվա փաստացի վճարումներ','activity.basis':'Ըստ վճարման ամսվա․ միայն գրանցված փոխանցումներ։ Անհայտ բաշխումը զրո չէ։','activity.paid':'Վճարված','activity.coverage':'Հայտնի / անհայտ / սպասող գրառումներ','activity.known':'Հայտնի բաշխումների գումարներ','activity.unknown':'Անհայտ','activity.actuals.error':'Ամսական գումարները հասանելի չեն'}, {'activity.actuals':'Monthly recorded payments','activity.basis':'By transaction month; recorded transfers only. Unknown allocation is not zero.','activity.paid':'Paid','activity.coverage':'Known / unknown / pending records','activity.known':'Known allocation subtotals','activity.unknown':'Unknown','activity.actuals.error':'Monthly totals unavailable'});
function exactMoney(minor,exponent,currency){
 if(minor==null)return T('activity.unknown');
 const value=BigInt(minor),sign=value<0n?'-':'';
 const digits=(value<0n?-value:value).toString().padStart(exponent+1,'0');
 return sign+(exponent?digits.slice(0,-exponent)+'.'+digits.slice(-exponent):digits)+' '+currency;
}
function actualsChart(a){
 const total=BigInt(a.paid_minor);if(total<=0n)return '';
 const parts=[['payment.principal',a.principal_minor,'var(--accent, #347a63)'],['payment.interest',a.interest_minor,'#cb8a25'],['payment.fees',a.fees_minor,'#9969b3'],['activity.unknown',a.unknown_paid_minor,'#8b929c']];
 const label=parts.map(([key,value])=>T(key)+': '+exactMoney(value,a.currency_exponent,a.currency)).join(' · ');
 return `<div role="img" aria-label="${esc(label)}" style="display:flex;height:16px;border-radius:8px;overflow:hidden;background:#e5e7eb">${parts.filter(([,value])=>value!=null&&BigInt(value)>0n).map(([key,value,color])=>{const width=BigInt(value)*10000n/total;return `<span title="${esc(T(key))}" style="width:${width/100n}.${(width%100n).toString().padStart(2,'0')}%;background:${color}"></span>`;}).join('')}</div><span class="hint">${esc(label)}</span>`;
}
let actualsGeneration=0;
async function loadActuals(){
 const generation=++actualsGeneration,el=document.getElementById('activity-actuals');
 el.textContent=T('loading');
 try{
 const month=document.getElementById('activity-month').value;
 const d=await getJSON('api/payment-actuals'+(month?'?month='+encodeURIComponent(month):''));
 if(generation!==actualsGeneration)return;
 document.getElementById('activity-month').value=d.month;
 el.innerHTML=(d.totals||[]).map(a=>`<article class="card stack"><strong>${esc(a.currency)} · ${esc(T('activity.paid'))}: ${esc(exactMoney(a.paid_minor,a.currency_exponent,a.currency))}</strong><span>${esc(T('activity.coverage'))}: ${esc(a.known_count)} / ${esc(a.unknown_count)} / ${esc(a.pending_count)}</span>${actualsChart(a)}<span>${esc(T('activity.known'))}</span>${['principal','interest','fees'].map(k=>`<span>${esc(T('payment.'+k))}: ${esc(exactMoney(a[k+'_minor'],a.currency_exponent,a.currency))}</span>`).join('')}</article>`).join('')||esc(T('activity.empty'));
 }catch{if(generation===actualsGeneration){el.textContent=T('activity.actuals.error');document.getElementById('activity-retry').hidden=false;}}
}
addStrings({'progress.title':'Ակտիվ պլան և գրանցված վճարումներ','progress.help':'Բանկի գրանցման ամսաթվով՝ հաստատման օրվանից հետո կատարված փոխանցումները։ Սա համադրում կամ վճարման ստուգում չէ։','progress.none':'Ակտիվ պլան չկա','progress.error':'Պլանի համեմատությունը հասանելի չէ','progress.window':'Համեմատման միջակայք','progress.empty':'Այս ամսում համեմատման ավարտված միջակայք չկա','progress.planned':'Պլանավորված','progress.posted':'Գրանցված բանկում՝ ըստ ձեր տվյալների','progress.delta':'Գրանցվածի և պլանի տարբերություն','progress.pending':'Սպասում է բանկի գրանցմանը','progress.excluded':'Հաստատման օրվա կամ ավելի վաղ փոխանցումներ՝ բացառված','progress.outside':'Պլանում չներառված վարկերի գրառումներ','progress.amount':'Գումարները տարբերվում են','progress.date':'Վճարման ամսաթվերը տարբերվում են','progress.fee':'Հայտնի վճարները տարբերվում են պլանից','progress.allocation':'Բանկի բաշխումը մասամբ կամ ամբողջությամբ անհայտ է','progress.missing':'Ժամկետին համապատասխան գրանցված վճարման տվյալ չկա','progress.schedule':'Այս միջակայքում պլանավորված վճարում չկա','progress.evidence':'Պլանի ամսաթվեր / բանկի գրանցման ամսաթվեր','progress.known':'Միայն հայտնի բանկային մասերի գումարներ','progress.feeDelta':'Վճարների տարբերություն','progress.missingAllocation':'Անհայտ բաշխմամբ գրառումներ'}, {'progress.title':'Active plan and recorded payments','progress.help':'Compared by reported bank value date, for transfers after the activation day. This does not verify or reconcile payments.','progress.none':'No active plan','progress.error':'Plan comparison unavailable','progress.window':'Comparison window','progress.empty':'No elapsed comparison window in this month','progress.planned':'Planned','progress.posted':'Reported bank-posted','progress.delta':'Recorded minus planned','progress.pending':'Pending bank posting','progress.excluded':'Activation-day or earlier transfers excluded','progress.outside':'Records for loans outside this baseline','progress.amount':'Amounts differ','progress.date':'Payment-date totals differ','progress.fee':'Known fees differ from plan','progress.allocation':'Bank allocation is partly or wholly unknown','progress.missing':'No posted payment record for the scheduled amount','progress.schedule':'No planned payment in this window','progress.evidence':'Planned dates / reported value dates','progress.known':'Known bank-reported component subtotals only','progress.feeDelta':'Fee difference','progress.missingAllocation':'Records with unknown allocation'});
function progressChart(row,c){
 const planned=BigInt(row.planned_minor),posted=row.posted_minor==null?null:BigInt(row.posted_minor);
 const maximum=posted!=null&&posted>planned?posted:planned;
 return [['progress.planned',planned,'#8b929c'],['progress.posted',posted,'var(--accent, #347a63)']].map(([key,value,color])=>{const width=value==null||maximum===0n?0n:value*10000n/maximum;const label=T(key)+': '+exactMoney(value,c.currency_exponent,c.currency);return `<div class="stack"><span>${esc(label)}</span>${value==null?'':`<div role="img" aria-label="${esc(label)}" style="height:12px;background:#e5e7eb;border-radius:6px;overflow:hidden"><div style="height:100%;width:${width/100n}.${(width%100n).toString().padStart(2,'0')}%;background:${color}"></div></div>`}</div>`;}).join('');
}
let progressGeneration=0;
async function loadPlanActuals(){
 const generation=++progressGeneration,el=document.getElementById('activity-plan-actuals');el.textContent=T('loading');
 try{
  const month=document.getElementById('activity-month').value;
  const d=await getJSON('api/plan-actuals'+(month?'?month='+encodeURIComponent(month):''));
  if(generation!==progressGeneration)return;
  el.innerHTML=(d.comparisons||[]).map(c=>`<article class="card stack"><strong>${esc(c.currency)}</strong><span>${esc(T('progress.window'))}: ${esc(c.from)} — ${esc(c.through)}</span><span class="hint">${esc(T('progress.pending'))}: ${esc(c.pending_count)} · ${esc(T('progress.excluded'))}: ${esc(c.excluded_before_activation_count)} · ${esc(T('progress.outside'))}: ${esc(c.outside_baseline_count)}</span>${c.empty_window?`<p>${esc(T('progress.empty'))}</p>`:(c.rows||[]).map(row=>`<section class="stack"><button class="alink" data-go="loan" data-arg="${esc(row.loan_id)}">${esc(row.loan)}</button>${progressChart(row,c)}<span>${esc(T('progress.delta'))}: ${esc(exactMoney(row.amount_delta_minor,c.currency_exponent,c.currency))}</span><span>${esc(T('progress.evidence'))}: ${(row.planned_dates||[]).map(x=>esc(x.on)+' ('+esc(exactMoney(x.amount_minor,c.currency_exponent,c.currency))+')').join(', ')} / ${(row.posted_dates||[]).map(x=>esc(x.on)+' ('+esc(exactMoney(x.amount_minor,c.currency_exponent,c.currency))+')').join(', ')||esc(T('activity.unknown'))}</span><span class="hint">${esc(T('progress.known'))}</span>${['principal','interest','fee'].map(k=>`<span>${esc(T('payment.'+(k==='fee'?'fees':k)))}: ${esc(exactMoney(row['known_'+k+'_minor'],c.currency_exponent,c.currency))}</span>`).join('')}<span>${esc(T('progress.feeDelta'))}: ${esc(exactMoney(row.fee_delta_minor,c.currency_exponent,c.currency))}</span><span>${esc(T('progress.missingAllocation'))}: ${esc(row.missing_allocation_count)}</span>${(row.causes||[]).map(cause=>`<span class="hint">${esc(T('progress.'+cause))}</span>`).join('')}</section>`).join('')}</article>`).join('')||esc(T('progress.none'));
 }catch{if(generation===progressGeneration){el.textContent=T('progress.error');document.getElementById('activity-retry').hidden=false;}}
}
let facts=[],busy=false,nextCursor="";
function render(){
 const filter=document.getElementById('activity-filter').value;
 document.getElementById('activity-list').innerHTML=facts.filter(f=>filter==='all'||f.kind!=='balance_snapshot').map((f)=>{
 const payment=f.kind==='payment_reported'||f.kind==='prepayment_reported';
 const label=f.kind==='balance_snapshot'?T('activity.balance'):T('payment.kind.'+f.kind);
 const amount=f.kind==='balance_snapshot'?f.principal_minor:f.amount_minor;
 return `<article class="card stack"><button class="alink" data-go="loan" data-arg="${esc(f.loan_id)}">${esc(f.loan)}</button><span>${esc(label)} · ${esc(fmtFull(f.as_of))}</span>${f.kind==='entry_voided'?'':`<strong>${esc(fmtMoney(amount/10**f.currency_exponent,f.currency))}</strong>`}${payment?`<span class="pill">${esc(T('payment.status.'+(f.voided?'voided':f.status)))}</span><span class="hint">${esc(T('payment.reported'))}</span>`:''}${payment?`<span class="hint">${f.allocation?['principal','interest','fees'].map(k=>esc(T('payment.'+k))+': '+esc(exactMoney(f.allocation[k+'_minor'],f.currency_exponent,f.currency))).join(' · '):esc(T('payment.allocation.unknown'))}</span>`:''}${payment&&!f.voided?`<div class="pair"><button class="alink" data-correct="${esc(f.id)}">${esc(T('payment.correct'))}</button><button class="alink red" data-void="${esc(f.id)}">${esc(T('payment.void'))}</button></div>`:''}${payment&&!f.voided&&f.value_date&&f.status!=='reconciled'?`<button class="alink" data-go="reconcile" data-arg="${esc(f.loan_id)}">${esc(T('activity.reconcile'))}</button>`:''}</article>`;
 }).join('')||esc(T('activity.empty'));
}
async function load(more=false){ if(!more)document.getElementById('activity-list').textContent=T('loading');const d=await getJSON('api/activity'+(more?'?after='+encodeURIComponent(nextCursor):''));facts=more?[...facts,...(d.facts||[])]:d.facts||[];nextCursor=d.next_cursor||'';document.getElementById('activity-more').hidden=!nextCursor;render(); }
register({id:'activity',icon:icon('document'),labelKey:'tab.activity',html:`<div class="card stack"><label for="activity-month" data-i18n="activity.actuals"></label><input id="activity-month" type="month"><p class="hint" data-i18n="activity.basis"></p><div id="activity-actuals" class="stack"></div></div><div class="card stack"><b data-i18n="progress.title"></b><p class="hint" data-i18n="progress.help"></p><div id="activity-plan-actuals" class="stack"></div></div><div class="card field"><label for="activity-filter" data-i18n="activity.latest"></label><select id="activity-filter"><option value="all" data-i18n="activity.all"></option><option value="payments" data-i18n="activity.payments"></option></select></div><p id="activity-error" class="error" role="alert"></p><button class="alink" type="button" id="activity-retry" data-i18n="retry" hidden></button><div id="activity-list" class="stack"></div><button id="activity-more" class="alink" data-i18n="activity.more" hidden></button>`,onMount(el){
 document.getElementById('activity-retry').onclick=()=>go('activity');
 document.getElementById('activity-month').addEventListener('change',()=>Promise.all([loadActuals(),loadPlanActuals()]));
 document.getElementById('activity-more').addEventListener('click',async()=>{if(busy)return;busy=true;try{await load(true);}catch{document.getElementById('activity-error').textContent=T('err.load');}finally{busy=false;}});
 document.getElementById('activity-filter').addEventListener('change',render);
 el.addEventListener('click',async e=>{
  const button=e.target.closest('[data-correct],[data-void]');if(!button||busy)return;
  const fact=facts.find(f=>f.id===(button.dataset.correct||button.dataset.void));if(!fact)return;
  if(button.dataset.correct){go('payment',{id:fact.loan_id,fact});return;}
  if(!await confirmDialog(T('payment.void.confirm')))return;
  busy=true;button.disabled=true;
  try{
   const meta=await getJSON('api/loans/'+encodeURIComponent(fact.loan_id)+'/payments');const on=meta.today;
   // Keep the retry identity on this record if the response is lost.
   fact.voidRequest ||= {idempotency_key:crypto.randomUUID(),expected_version:fact.version,amount_minor:0,transaction_date:on,value_date:'',replaces:fact.id,void_only:true};
   const res=await api('api/loans/'+encodeURIComponent(fact.loan_id)+'/payments',{method:'POST',body:JSON.stringify(fact.voidRequest)});
   if(!res.ok) throw new Error(res.status===409?T('payment.conflict'):T('err.save'));
   invalidate('api/');await Promise.all([load(),loadActuals(),loadPlanActuals()]);
  }catch(err){document.getElementById('activity-error').textContent=err.message||T('err.save');}
  finally{busy=false;button.disabled=false;}
 });
},async onShow(){document.getElementById('activity-error').textContent='';document.getElementById('activity-retry').hidden=true;try{await Promise.all([load(),loadActuals(),loadPlanActuals()]);}catch{document.getElementById('activity-list').textContent='';document.getElementById('activity-error').textContent=T('err.load');document.getElementById('activity-retry').hidden=false;}}});
