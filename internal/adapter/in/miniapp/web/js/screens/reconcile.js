"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {minorAmount,minorText,validDate} from './budget-funding.js';

addStrings({
 'reconcile.title':'Համադրել','reconcile.next':'Շարունակել','reconcile.back':'Հետ',
 'reconcile.principal':'Այսօրվա մնացած մայր գումարը՝ բանկի քաղվածքից',
 'reconcile.due':'Հաջորդ չվճարված վճարման ամսաթիվը',
 'reconcile.payment':'Հաջորդ չվճարված վճարման գումարը',
 'reconcile.cash':'Վարկերի համար հիմա մնացած գումարը՝ վճարումներից ՀԵՏՈ',
 'reconcile.spent':'Այս ամսվա՝ վարկերի համար ԸՆԴՀԱՆՈՒՐ ծախսը',
 'reconcile.spentHint':'Ընդհանուր գումարն արդեն ներառում է բոլոր վճարումները։ Սա հավելյալ գումար չէ։',
 'reconcile.confirm':'Հաստատում եմ, որ նշված մնացորդներն ու ամսվա ընդհանուր ծախսը ներառում են բանկում գրանցված բոլոր վճարումները։',
 'reconcile.pending':'Եթե վճարումը դեռ սպասում է բանկի գրանցմանը, նախ Պատմություն բաժնում թարմացրեք դրա գրանցման կարգավիճակը։',
 'reconcile.funding':'Համադրումից առաջ բյուջեում առանձին նշեք ֆինանսավորումը։',
 'reconcile.budget':'Խմբագրել բյուջեն',
 'reconcile.required':'Հաստատեք, որ բանկում գրանցված բոլոր վճարումները ներառված են։',
 'reconcile.dateError':'Նշեք հաջորդ չվճարված վճարման վավեր ամսաթիվը։',
 'reconcile.retry':'Պահպանումը հաստատված չէ։ Դաշտերը կողպված են։ Կրկին պահպանումը կուղարկի նույն հարցումը։',
 'reconcile.conflict':'Վարկը կամ բյուջեն փոխվել է։ Վերաբացեք այս էջը և ստուգեք տվյալները։'
},{
 'reconcile.title':'Reconcile','reconcile.next':'Continue','reconcile.back':'Back',
 'reconcile.principal':'Remaining principal today, from your bank statement',
 'reconcile.due':'Next unpaid payment due date',
 'reconcile.payment':'Next unpaid payment amount',
 'reconcile.cash':'Cash left for loans now, AFTER payments',
 'reconcile.spent':'TOTAL debt spending this month',
 'reconcile.spentHint':'This total already includes all payments. It is not an additional amount.',
 'reconcile.confirm':'I confirm the balances and monthly spending total include all posted payments.',
 'reconcile.pending':'For pending payments, first update their bank posting status in Activity.',
 'reconcile.funding':'Set explicit funding in your budget before reconciling.',
 'reconcile.budget':'Edit budget',
 'reconcile.required':'Confirm that all posted payments are included.',
 'reconcile.dateError':'Enter a valid next unpaid payment due date.',
 'reconcile.retry':'Save is unconfirmed. Fields stay locked. Save again to retry the exact same request.',
 'reconcile.conflict':'The loan or budget changed. Reopen this page and review the latest values.'
});

const $=id=>document.getElementById(id);
const unresolved=new Map(); // Exact requests survive navigation, but never leave memory.
let context=null, generation=0;
function here(view){return context===view&&currentScreen()==='reconcile';}
function step(funding){$('rec-balance-block').hidden=funding;$('rec-cash-block').hidden=!funding;$('rec-save').hidden=!funding;}
function paidOff(){
 try{return minorAmount($('rec-principal').value,context.meta.currency_exponent)===0;}
 catch{return false;}
}
function controls(){
 const locked=!context||!context.funding||!!context.entry;
 for(const el of $('reconcile-form').querySelectorAll('input'))el.disabled=locked;
 const zero=!!context&&paidOff();
 $('rec-due').required=!zero;$('rec-payment').required=!zero;
 $('rec-due').disabled=locked||zero;$('rec-payment').disabled=locked||zero;
 $('rec-save').disabled=!context||!context.funding||!!context.entry?.busy||!!context.conflict;
}
function restore(view){
 context=view;step(!!view.entry);
 $('rec-loan').textContent=view.meta.loan;
 $('rec-currency').textContent=view.meta.currency;
 $('rec-today').textContent=view.entry?.body.as_of||view.meta.today;
 $('rec-funding').hidden=!!view.funding;
 if(view.entry){
  const b=view.entry.body,exp=view.meta.currency_exponent;
  for(const [id,key] of [['principal','principal_minor'],['payment','next_payment_minor'],['cash','cash_minor'],['spent','spent_minor']])$('rec-'+id).value=minorText(b[key],exp);
  $('rec-due').value=b.next_due;$('rec-confirm').checked=true;
  $('rec-error').textContent=T('reconcile.retry');
 }
 controls();
}
async function submit(event){
 event.preventDefault();
 const view=context;
 if(!view||!here(view)||!view.funding||view.entry?.busy||view.conflict)return;
 $('rec-error').textContent='';
 if(!view.entry){
  if(!$('rec-confirm').checked){$('rec-error').textContent=T('reconcile.required');return;}
  try{
   const exp=view.meta.currency_exponent,principal=minorAmount($('rec-principal').value,exp);
   const due=principal===0?'':$('rec-due').value;
   if(principal!==0&&!validDate(due)){$('rec-error').textContent=T('reconcile.dateError');return;}
   const body={idempotency_key:crypto.randomUUID(),expected_version:view.meta.version,budget_version:view.budgetVersion,
    as_of:view.meta.today,principal_minor:principal,next_due:due,
    next_payment_minor:principal===0?0:minorAmount($('rec-payment').value,exp,{positive:true}),
    cash_minor:minorAmount($('rec-cash').value,exp),spent_minor:minorAmount($('rec-spent').value,exp),include_posted:true};
   view.entry={body,meta:view.meta,budgetVersion:view.budgetVersion,busy:false};
   unresolved.set(view.id,view.entry);
  }catch{$('rec-error').textContent=T('err.number');return;}
 }
 const entry=view.entry;
 entry.busy=true;controls();
 // A reopened view may share this request; a different loan never does.
 const visible=()=>currentScreen()==='reconcile'&&context?.id===view.id&&context.entry===entry;
 try{
  const res=await api('api/loans/'+encodeURIComponent(view.id)+'/reconcile',{method:'POST',body:JSON.stringify(entry.body)});
  if(res.ok){
   unresolved.delete(view.id);invalidate('api/');
   if(visible()){context.entry=null;go('activity');}
  }else if(res.status>=400&&res.status<500){
   unresolved.delete(view.id);
   if(visible()){
    context.entry=null;context.conflict=res.status===409;
    $('rec-error').textContent=T(res.status===409?'reconcile.conflict':'err.save');
   }
  }else if(visible())$('rec-error').textContent=T('reconcile.retry');
 }catch{if(visible())$('rec-error').textContent=T('reconcile.retry');}
 finally{entry.busy=false;if(currentScreen()==='reconcile'&&context?.id===view.id)controls();}
}
register({id:'reconcile',parent:'activity',titleKey:'reconcile.title',html:`
 <form id="reconcile-form" class="card stack">
 <b id="rec-loan"></b><div class="hint"><span id="rec-today"></span> · <span id="rec-currency"></span></div>
 <p class="hint" data-i18n="reconcile.pending"></p>
 <div id="rec-funding" hidden><p data-i18n="reconcile.funding"></p><button type="button" class="alink" data-go="budget-edit" data-i18n="reconcile.budget"></button></div>
 <div id="rec-balance-block" class="stack"><div class="field"><label for="rec-principal" data-i18n="reconcile.principal"></label><input id="rec-principal" inputmode="decimal" required></div>
 <div class="pair"><div class="field"><label for="rec-due" data-i18n="reconcile.due"></label><input id="rec-due" type="date" required></div><div class="field"><label for="rec-payment" data-i18n="reconcile.payment"></label><input id="rec-payment" inputmode="decimal" required></div></div>
 <button type="button" id="rec-next" class="cta" data-i18n="reconcile.next"></button></div><div id="rec-cash-block" class="stack" hidden><div class="field"><label for="rec-cash" data-i18n="reconcile.cash"></label><input id="rec-cash" inputmode="decimal" required></div>
 <div class="field"><label for="rec-spent" data-i18n="reconcile.spent"></label><input id="rec-spent" inputmode="decimal" aria-describedby="rec-spent-hint" required><p id="rec-spent-hint" class="hint" data-i18n="reconcile.spentHint"></p></div>
 <label class="row" style="align-items:flex-start;gap:10px"><input style="width:24px;min-height:24px;height:24px;flex:none" id="rec-confirm" type="checkbox" required> <span data-i18n="reconcile.confirm"></span></label>
 <button type="button" id="rec-back" class="alink" data-i18n="reconcile.back"></button></div><p id="rec-error" class="error" role="alert"></p><button id="rec-save" class="cta" type="submit" data-i18n="save" disabled></button>
 </form>`,onMount(){
 $('reconcile-form').addEventListener('submit',submit);
 $('rec-principal').addEventListener('input',controls);
 $('rec-back').addEventListener('click',()=>step(false));
 $('rec-next').addEventListener('click',()=>{
  if(!context||!context.funding)return;
  try{minorAmount($('rec-principal').value,context.meta.currency_exponent);if(!paidOff()){minorAmount($('rec-payment').value,context.meta.currency_exponent,{positive:true});if(!validDate($('rec-due').value))throw new Error('date');}}
  catch{$('rec-error').textContent=T('err.number');return;}
  $('rec-error').textContent='';step(true);
 });
 },async onShow(_,params){
 const stamp=++generation,id=params?.id;
 context=null;step(false);$('reconcile-form').reset();$('rec-error').textContent='';
 $('rec-loan').textContent='';$('rec-currency').textContent='';$('rec-today').textContent='';$('rec-funding').hidden=true;controls();
 const entry=unresolved.get(id);
 if(entry){restore({id,meta:entry.meta,budgetVersion:entry.budgetVersion,funding:true,entry});return;}
 const active=()=>stamp===generation&&currentScreen()==='reconcile';
 try{
  if(!id)throw new Error('loan');
  // A versioned write must not start from the API cache's offline fallback.
  const read=async path=>{const res=await api(path,{cache:'no-store'});if(!res.ok)throw new Error('load');return res.json();};
  const [meta,budget]=await Promise.all([read('api/loans/'+encodeURIComponent(id)+'/payments'),read('api/budget')]);
  if(!active())return;
  if(meta.loan_id!==id||!validDate(meta.today)||!Number.isSafeInteger(meta.version)||meta.version<0||!Number.isSafeInteger(budget.version)||budget.version<0)throw new Error('metadata');
  minorText(0,meta.currency_exponent);
  const funding=budget.currency===meta.currency&&budget.funding!==null&&typeof budget.funding==='object'&&!Array.isArray(budget.funding);
  restore({id,meta,budgetVersion:budget.version,funding,entry:null});
 }catch{if(active())$('rec-error').textContent=T('err.load');}
 }});
