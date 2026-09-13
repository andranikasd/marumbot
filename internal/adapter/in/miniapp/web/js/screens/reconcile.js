"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {minorAmount,minorText,validDate} from './budget-funding.js';

addStrings({
 'reconcile.title':'Ստուգել բանկի մնացորդները','reconcile.next':'Շարունակել','reconcile.back':'Հետ',
 'reconcile.principal':'Այս վարկից որքա՞ն է մնացել մարելու',
 'reconcile.due':'Հաջորդ չվճարված վճարման ամսաթիվը',
 'reconcile.payment':'Հաջորդ չվճարված վճարման գումարը',
 'reconcile.cash':'ԲՈԼՈՐ վարկերի համար հիմա որքա՞ն գումար ունեք',
 'reconcile.spent':'Այս շրջանում որքա՞ն եք արդեն վճարել ԲՈԼՈՐ վարկերին',
 'reconcile.spentHint':'Ներառեք այս վճարումն ու մյուս վարկերի վճարումները։ Նշեք ամբողջ շրջանի ընդհանուր գումարը, ոչ միայն վերջին վճարումը։ Այն կրկին չենք հանի մնացած գումարից։',
 'reconcile.confirm':'Հաստատում եմ, որ նշված մնացորդներն ու ամսվա ընդհանուր ծախսը ներառում են բանկում գրանցված բոլոր վճարումները։',
 'reconcile.pending':'Եթե վճարումը դեռ սպասում է բանկի գրանցմանը, նախ Պատմություն բաժնում թարմացրեք դրա գրանցման կարգավիճակը։',
 'reconcile.funding':'Նախ կարգավորեք վարկերի բյուջեն, ապա վերադարձեք՝ վճարումներից հետո մնացած գումարները ստուգելու համար։',
 'reconcile.budget':'Խմբագրել բյուջեն',
 'reconcile.required':'Հաստատեք, որ բանկում գրանցված բոլոր վճարումները ներառված են։',
 'reconcile.dateError':'Նշեք հաջորդ չվճարված վճարման վավեր ամսաթիվը։',
 'reconcile.retry':'Պահպանումը հաստատված չէ։ Դաշտերը կողպված են։ Կրկին պահպանումը կուղարկի նույն հարցումը։',
 'reconcile.conflict':'Վարկը կամ բյուջեն փոխվել է։ Վերաբացեք այս էջը և ստուգեք տվյալները։'
},{
 'reconcile.title':'Check bank balances','reconcile.next':'Continue','reconcile.back':'Back',
 'reconcile.principal':'How much of this loan is left to repay?',
 'reconcile.due':'Next unpaid payment due date',
 'reconcile.payment':'Next unpaid payment amount',
 'reconcile.cash':'How much money is left for ALL your loans?',
 'reconcile.spent':'How much have you already paid toward ALL loans this period?',
 'reconcile.spentHint':'Include this payment and payments to your other loans. Enter the whole period total, not just the latest payment. We will not subtract it from your money left again.',
 'reconcile.confirm':'I confirm the balances and monthly spending total include all posted payments.',
 'reconcile.pending':'For pending payments, first update their bank posting status in Activity.',
 'reconcile.funding':'First, set up your loan budget so we know which money these payments came from. Then return here to check the updated balances.',
 'reconcile.budget':'Edit budget',
 'reconcile.required':'Confirm that all posted payments are included.',
 'reconcile.dateError':'Enter a valid next unpaid payment due date.',
 'reconcile.retry':'Save is unconfirmed. Fields stay locked. Save again to retry the exact same request.',
 'reconcile.conflict':'The loan or budget changed. Reopen this page and review the latest values.'
});

addStrings({
 'reconcile.currency':'Այս վարկի և բյուջեի արժույթները տարբեր են։ Բյուջեում ընտրեք վարկի արժույթը, ապա վերադարձեք։','reconcile.intro':'Բացեք բանկի հավելվածը։ Նշեք վճարումից հետո այնտեղ երևացող տվյալները։ Marum-ը գումար չի փոխանցում։',
 'reconcile.step1':'1. Վարկի թարմ տվյալները','reconcile.step2':'2. Ձեր մնացած գումարը','reconcile.step3':'3. Ստուգել և պահպանել',
 'reconcile.principalHint':'Նշեք միայն մնացած մայր գումարը՝ առանց ապագա տոկոսների։ Եթե վարկը ամբողջությամբ մարված է, նշեք 0։',
 'reconcile.cashHint':'Նշեք այսօր վարկերի համար նախատեսված և դեռ չծախսված գումարը՝ այս վճարումից ՀԵՏՈ։ Մի ներառեք ապագա աշխատավարձը։',
 'reconcile.review':'Ստուգել տվյալները','reconcile.save':'Հաստատել մնացորդները','reconcile.period':'Այս շրջանի սկիզբը՝',
 'reconcile.zero':'Վարկն ամբողջությամբ մարված է։ Հաջորդ վճարման տվյալներ պետք չեն։',
 'reconcile.load':'Չհաջողվեց ստանալ թարմ տվյալները։ Ստուգեք կապը և փորձեք նորից։','reconcile.reload':'Նորից բեռնել',
 'reconcile.pendingAction':'Բացել վճարումների պատմությունը','reconcile.invalid':'Ստուգեք գումարները և հաջորդ վճարման օրը։ Ամսվա ընդհանուր վճարումները պետք է ներառեն բանկում գրանցված բոլոր վճարումները։',
 'reconcile.future':'Հաջորդ չվճարված վճարումը պետք է լինի այսօրվանից հետո։ Եթե ժամկետանց վճարում կա, նախ ճշտեք բանկից։',
 'reconcile.confirm':'Ստուգել եմ բանկի թարմ տվյալները։ Մնացած գումարն ու շրջանի վճարումների ընդհանուր գումարը ներառում են արդեն կատարված վճարումները։'
},{
 'reconcile.currency':'This loan and your budget use different currencies. Choose the loan currency in your budget, then return here.','reconcile.intro':'Open your banking app. Use the figures it shows AFTER your payment. Marum does not move money.',
 'reconcile.step1':'1. Updated loan figures','reconcile.step2':'2. Your money left','reconcile.step3':'3. Review and save',
 'reconcile.principalHint':'Use the remaining principal only, without future interest. Enter 0 if the loan is fully repaid.',
 'reconcile.cashHint':'Money set aside for loans that you still have today, AFTER this payment. Do not include a future payday.',
 'reconcile.review':'Review these figures','reconcile.save':'Confirm updated balances','reconcile.period':'This period starts:',
 'reconcile.zero':'This loan is fully repaid. No next payment is needed.',
 'reconcile.load':'We could not get the latest figures. Check your connection and try again.','reconcile.reload':'Reload latest figures',
 'reconcile.pendingAction':'Open payment history','reconcile.invalid':'Check the amounts and next payment date. Your period total must include all recorded bank-processed payments.',
 'reconcile.future':'The next unpaid payment must be after today. If a payment is overdue, check with your bank first.',
 'reconcile.confirm':'I checked the current bank figures. The money left and the period payment total already include the payments I made.'
});
const $=id=>document.getElementById(id);
const drafts=new Map();
const draftFields=['principal','due','payment','cash','spent'];
function rememberDraft(){
 if(!context?.meta||context.entry||context.conflict||!context.funding)return;
 drafts.set(context.id,{view:{...context},values:Object.fromEntries(draftFields.map(id=>[id,$('rec-'+id).value])),confirmed:$('rec-confirm').checked});
}
const unresolved=new Map(); // Exact requests survive navigation, but never leave memory.
let context=null, generation=0;
function here(view){return context===view&&currentScreen()==='reconcile';}
function step(value){
 const index=value===true?1:value===false?0:value;
 $('rec-balance-block').hidden=index!==0;$('rec-cash-block').hidden=index!==1;$('rec-review-block').hidden=index!==2;$('rec-save').hidden=index!==2;
 $('rec-step').textContent=T(['reconcile.step1','reconcile.step2','reconcile.step3'][index]);
 if(context)context.step=index;
}
function review(){
 for(const id of ['principal','payment','cash','spent'])$('rec-review-'+id).textContent=$( 'rec-'+id).value+' '+context.meta.currency;
 $('rec-review-due').textContent=paidOff()?T('reconcile.zero'):$('rec-due').value;
 $('rec-review-payment-row').hidden=paidOff();
}
function dateValid(){return validDate($('rec-due').value)&&$('rec-due').value>context.meta.today;}
function paidOff(){
 try{return minorAmount($('rec-principal').value,context.meta.currency_exponent)===0;}
 catch{return false;}
}
function controls(){
 const locked=!context||!context.funding||!!context.entry||!!context.conflict;
 for(const el of $('reconcile-form').querySelectorAll('input'))el.disabled=locked;
 const zero=!!context&&paidOff();
 $('rec-due').required=!zero;$('rec-payment').required=!zero;
 $('rec-due').disabled=locked||zero;$('rec-payment').disabled=locked||zero;
 $('rec-next').disabled=locked;$('rec-review').disabled=locked;$('rec-zero').hidden=!zero;
 $('rec-save').disabled=!context||!context.funding||!!context.entry?.busy||!!context.conflict;
}
function restore(view){
 context=view;step(view.entry?2:0);
 $('rec-loan').textContent=view.meta.loan;
 $('rec-currency').textContent=view.meta.currency;
 $('rec-today').textContent=view.entry?.body.as_of||view.meta.today;
 $('rec-funding').hidden=!!view.funding;$('rec-funding-message').textContent=T(view.currencyMismatch?'reconcile.currency':'reconcile.funding');$('rec-period').textContent=view.spentPeriodStart||view.entry?.body.spent_period_start||view.meta.today.slice(0,7)+'-01';
 if(view.entry){
  const b=view.entry.body,exp=view.meta.currency_exponent;
  for(const [id,key] of [['principal','principal_minor'],['payment','next_payment_minor'],['cash','cash_minor'],['spent','spent_minor']])$('rec-'+id).value=minorText(b[key],exp);
  $('rec-due').value=b.next_due;$('rec-confirm').checked=true;
  $('rec-error').textContent=T('reconcile.retry');review();
 }
 controls();
}
async function submit(event){
 event.preventDefault();
 const view=context;
 if(!view||!here(view)||!view.funding||view.entry?.busy||view.conflict)return;
 $('rec-error').textContent='';
 if(!view.entry&&view.step<2){$(view.step===0?'rec-next':'rec-review').click();return;}
 if(!view.entry){
  if(!$('rec-confirm').checked){$('rec-error').textContent=T('reconcile.required');return;}
  try{
   const exp=view.meta.currency_exponent,principal=minorAmount($('rec-principal').value,exp);
   const due=principal===0?'':$('rec-due').value;
   if(principal!==0&&!dateValid()){$('rec-error').textContent=T('reconcile.dateError');return;}
   const body={idempotency_key:crypto.randomUUID(),expected_version:view.meta.version,budget_version:view.budgetVersion,spent_period_start:view.spentPeriodStart||view.meta.today.slice(0,7)+'-01',
    as_of:view.meta.today,principal_minor:principal,next_due:due,
    next_payment_minor:principal===0?0:minorAmount($('rec-payment').value,exp,{positive:true}),
    cash_minor:minorAmount($('rec-cash').value,exp),spent_minor:minorAmount($('rec-spent').value,exp),include_posted:true};
   view.entry={body,meta:view.meta,budgetVersion:view.budgetVersion,busy:false};
   drafts.delete(view.id);
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
   unresolved.delete(view.id);drafts.delete(view.id);invalidate('api/');
   if(visible()){context=null;go('activity');}
  }else if(res.status>=400&&res.status<500){
   unresolved.delete(view.id);
   if(visible()){
    let body={};try{body=await res.json();}catch{}
    if(!visible())return;
    context.entry=null;context.conflict=res.status===409;
    $('rec-error').textContent=T(res.status===409?'reconcile.conflict':body.error==='payment_reconciliation_required'?'reconcile.pending':'reconcile.invalid');
    $('rec-reload').hidden=res.status!==409;$('rec-history').hidden=body.error!=='payment_reconciliation_required';
   }
  }else if(visible())$('rec-error').textContent=T('reconcile.retry');
 }catch{if(visible())$('rec-error').textContent=T('reconcile.retry');}
 finally{entry.busy=false;if(currentScreen()==='reconcile'&&context?.id===view.id)controls();}
}
register({id:'reconcile',parent:'activity',titleKey:'reconcile.title',html:`
 <form id="reconcile-form" class="card stack" novalidate>
 <b id="rec-loan"></b><div class="hint"><span id="rec-today"></span> · <span id="rec-currency"></span></div>
 <p class="hint" data-i18n="reconcile.intro"></p><b id="rec-step"></b>
 <div id="rec-funding" hidden><p id="rec-funding-message"></p><button type="button" class="alink" data-go="budget-edit" data-i18n="reconcile.budget"></button></div>
 <div id="rec-balance-block" class="stack"><div class="field"><label for="rec-principal" data-i18n="reconcile.principal"></label><input id="rec-principal" inputmode="decimal" aria-describedby="rec-principal-hint" required><p class="hint" id="rec-principal-hint" data-i18n="reconcile.principalHint"></p><p id="rec-zero" class="hint" data-i18n="reconcile.zero" hidden></p></div>
 <div class="pair"><div class="field"><label for="rec-due" data-i18n="reconcile.due"></label><input id="rec-due" type="date" required></div><div class="field"><label for="rec-payment" data-i18n="reconcile.payment"></label><input id="rec-payment" inputmode="decimal" required></div></div>
 <button type="button" id="rec-next" class="cta" data-i18n="reconcile.next"></button></div><div id="rec-cash-block" class="stack" hidden><div class="field"><label for="rec-cash" data-i18n="reconcile.cash"></label><input id="rec-cash" inputmode="decimal" aria-describedby="rec-cash-hint" required><p id="rec-cash-hint" class="hint" data-i18n="reconcile.cashHint"></p></div><p class="hint"><span data-i18n="reconcile.period"></span> <span id="rec-period"></span></p>
 <div class="field"><label for="rec-spent" data-i18n="reconcile.spent"></label><input id="rec-spent" inputmode="decimal" aria-describedby="rec-spent-hint" required><p id="rec-spent-hint" class="hint" data-i18n="reconcile.spentHint"></p></div>
 <button type="button" id="rec-review" class="cta" data-i18n="reconcile.review"></button><button type="button" id="rec-back" class="alink" data-i18n="reconcile.back"></button></div><div id="rec-review-block" class="stack" hidden><p class="hint" data-i18n="reconcile.spentHint"></p><dl><dt data-i18n="reconcile.principal"></dt><dd id="rec-review-principal"></dd><dt data-i18n="reconcile.due"></dt><dd id="rec-review-due"></dd><div id="rec-review-payment-row"><dt data-i18n="reconcile.payment"></dt><dd id="rec-review-payment"></dd></div><dt data-i18n="reconcile.cash"></dt><dd id="rec-review-cash"></dd><dt data-i18n="reconcile.spent"></dt><dd id="rec-review-spent"></dd></dl><label class="row" style="align-items:flex-start;gap:10px"><input style="width:24px;min-height:24px;height:24px;flex:none" id="rec-confirm" type="checkbox" required> <span data-i18n="reconcile.confirm"></span></label>
 <button type="button" id="rec-edit" class="alink" data-i18n="reconcile.back"></button></div><p id="rec-error" class="error" role="alert"></p><button id="rec-save" class="cta" type="submit" data-i18n="reconcile.save" disabled></button><button id="rec-reload" type="button" class="alink" data-i18n="reconcile.reload" hidden></button><button id="rec-history" type="button" class="alink" data-go="activity" data-i18n="reconcile.pendingAction" hidden></button>
 </form>`,onMount(){
 $('reconcile-form').addEventListener('submit',submit);
 $('rec-principal').addEventListener('input',controls);
 $('rec-back').addEventListener('click',()=>{if(!context?.entry)step(0);});
 $('rec-edit').addEventListener('click',()=>{if(!context?.entry)step(1);});
 $('rec-reload').addEventListener('click',()=>{if(context?.id){const id=context.id;drafts.delete(id);context=null;go('reconcile',{id});}});
 $('rec-review').addEventListener('click',()=>{
  if(!context||context.entry||!context.funding)return;
  try{minorAmount($('rec-cash').value,context.meta.currency_exponent);minorAmount($('rec-spent').value,context.meta.currency_exponent);}
  catch{$('rec-error').textContent=T('err.number');return;}
  $('rec-error').textContent='';$('rec-confirm').checked=false;review();step(2);
 });
 $('rec-next').addEventListener('click',()=>{
  if(!context||!context.funding||context.entry||context.conflict)return;
  try{minorAmount($('rec-principal').value,context.meta.currency_exponent);if(!paidOff()){minorAmount($('rec-payment').value,context.meta.currency_exponent,{positive:true});if(!dateValid())throw new Error('date');}}
  catch(error){$('rec-error').textContent=T(error.message==='date'?'reconcile.future':'err.number');return;}
  $('rec-error').textContent='';step(1);
 });
 },onLanguage(){if(context?.meta){step(context.step||0);if(context.step===2)review();}},async onShow(_,params){
 rememberDraft();
 const stamp=++generation,id=params?.id;
 context=null;step(false);$('reconcile-form').reset();$('rec-error').textContent='';
 $('rec-loan').textContent='';$('rec-currency').textContent='';$('rec-today').textContent='';$('rec-funding').hidden=true;$('rec-reload').hidden=true;$('rec-history').hidden=true;controls();
 const entry=unresolved.get(id);
 if(entry){restore({id,meta:entry.meta,budgetVersion:entry.budgetVersion,spentPeriodStart:entry.body.spent_period_start,funding:true,entry});return;}
 const draft=drafts.get(id);
 if(draft){
  restore({...draft.view});
  for(const field of draftFields)$('rec-'+field).value=draft.values[field];
  $('rec-confirm').checked=draft.confirmed;step(draft.view.step||0);
  if(draft.view.step===2)review();controls();return;
 }
 const active=()=>stamp===generation&&currentScreen()==='reconcile';
 try{
  if(!id)throw new Error('loan');
  // A versioned write must not start from the API cache's offline fallback.
  const read=async path=>{const res=await api(path,{cache:'no-store'});if(!res.ok)throw new Error('load');return res.json();};
  const [meta,budget]=await Promise.all([read('api/loans/'+encodeURIComponent(id)+'/payments'),read('api/budget')]);
  if(!active())return;
  if(meta.loan_id!==id||!validDate(meta.today)||!Number.isSafeInteger(meta.version)||meta.version<0||(budget.version!==undefined&&(!Number.isSafeInteger(budget.version)||budget.version<0)))throw new Error('metadata');
  minorText(0,meta.currency_exponent);
  const funding=Number.isSafeInteger(budget.version)&&budget.version>0&&budget.currency===meta.currency&&budget.funding!==null&&typeof budget.funding==='object'&&!Array.isArray(budget.funding);
  restore({id,meta,budgetVersion:budget.version,currencyMismatch:!!budget.currency&&budget.currency!==meta.currency,spentPeriodStart:budget.spent_period_start||budget.funding?.spent_period_start||meta.today.slice(0,7)+'-01',funding,entry:null});
 }catch{if(active()){$('rec-error').textContent=T('reconcile.load');context={id,meta:null,funding:false};$('rec-reload').hidden=false;}}
 }});
