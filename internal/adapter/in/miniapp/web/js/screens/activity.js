"use strict";
import {register,go} from '../nav.js';
import {getJSON,api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {fmtMoney,fmtFull,esc,confirmDialog} from '../core.js';
import {icon} from '../icons.js';
addStrings({'tab.activity':'Պատմություն','activity.balance':'Մնացորդի գրառում','activity.empty':'Գրառումներ դեռ չկան','activity.latest':'Գրառումներ','activity.more':'Ավելի հին գրառումներ','activity.all':'Բոլորը','activity.payments':'Վճարումներ'},{'tab.activity':'Activity','activity.balance':'Balance statement','activity.empty':'No records yet','activity.latest':'Records','activity.more':'Older records','activity.all':'All','activity.payments':'Payments'});
let facts=[],busy=false,nextCursor="";
function render(){
 const filter=document.getElementById('activity-filter').value;
 document.getElementById('activity-list').innerHTML=facts.filter(f=>filter==='all'||f.kind!=='balance_snapshot').map((f)=>{
 const payment=f.kind==='payment_reported'||f.kind==='prepayment_reported';
 const label=f.kind==='balance_snapshot'?T('activity.balance'):T('payment.kind.'+f.kind);
 const amount=f.kind==='balance_snapshot'?f.principal_minor:f.amount_minor;
 return `<article class="card stack"><button class="alink" data-go="loan" data-arg="${esc(f.loan_id)}">${esc(f.loan)}</button><span>${esc(label)} · ${esc(fmtFull(f.as_of))}</span>${f.kind==='entry_voided'?'':`<strong>${esc(fmtMoney(amount/10**f.currency_exponent,f.currency))}</strong>`}${payment?`<span class="pill">${esc(T('payment.status.'+(f.voided?'voided':f.status)))}</span><span class="hint">${esc(T('payment.reported'))}</span>`:''}${payment&&!f.voided?`<div class="pair"><button class="alink" data-correct="${esc(f.id)}">${esc(T('payment.correct'))}</button><button class="alink red" data-void="${esc(f.id)}">${esc(T('payment.void'))}</button></div>`:''}</article>`;
 }).join('')||esc(T('activity.empty'));
}
async function load(more=false){ const d=await getJSON('api/activity'+(more?'?after='+encodeURIComponent(nextCursor):''));facts=more?[...facts,...(d.facts||[])]:d.facts||[];nextCursor=d.next_cursor||'';document.getElementById('activity-more').hidden=!nextCursor;render(); }
register({id:'activity',icon:icon('document'),labelKey:'tab.activity',html:`<div class="card field"><label for="activity-filter" data-i18n="activity.latest"></label><select id="activity-filter"><option value="all" data-i18n="activity.all"></option><option value="payments" data-i18n="activity.payments"></option></select></div><p id="activity-error" class="error" role="alert"></p><div id="activity-list" class="stack"></div><button id="activity-more" class="alink" data-i18n="activity.more" hidden></button>`,onMount(el){
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
   invalidate('api/');await load();
  }catch(err){document.getElementById('activity-error').textContent=err.message||T('err.save');}
  finally{busy=false;button.disabled=false;}
 });
},async onShow(){document.getElementById('activity-error').textContent='';try{await load();}catch{document.getElementById('activity-error').textContent=T('err.load');}}});
