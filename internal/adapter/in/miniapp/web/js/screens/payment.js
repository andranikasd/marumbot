"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {toast,confirmDialog} from '../core.js';

addStrings({
 'payment.pause':'Արդեն վճարե՞լ եք։ Գրանցեք վճարումն այստեղ։ Հաջորդ քայլում կնշեք բանկի թարմ տվյալները։','payment.retry':'Պահպանումը հաստատված չէ։ Կրկին սեղմեք՝ նույն գրառումը ստուգելու համար։','payment.record':'Գրանցել վճարում','payment.amount':'Վճարված գումար','payment.date':'Վճարման ամսաթիվ','payment.posting':'Բանկն արդեն մշակե՞լ է վճարումը','payment.pending':'Դեռ ոչ / վստահ չեմ','payment.posted':'Այո, այն տեսնում եմ բանկի հավելվածում','payment.value':'Ե՞րբ է բանկը մշակել վճարումը','payment.intent':'Վճարման տեսակ','payment.required':'Պարտադիր','payment.extra':'Լրացուցիչ','payment.review':'Ստուգել բանկի մնացորդները','payment.reconcile':'Պլանավորումը շարունակելու համար նշեք վճարումից հետո վարկի թարմ մնացորդը և մնացած գումարը։','payment.saved':'Վճարման գրառումը պահպանված է','payment.duplicate':'Նման վճարում արդեն կա։ Գրանցե՞լ ևս մեկ առանձին վճարում։','payment.conflict':'Գրառումները փոխվել են։ Վերաբացեք էջը և ստուգեք դրանք։','payment.notice':'Marum-ը գումար չի փոխանցում։','payment.correct':'Թարմացնել վճարման տվյալները','payment.void':'Չեղարկել գրառումը','payment.void.confirm':'Չեղարկե՞լ այս գրառումը։ Այն կմնա պատմության մեջ։','payment.status.pending_bank_posting':'Սպասում է բանկի գրանցմանը','payment.status.needs_reconciliation':'Ստուգել բանկի մնացորդները','payment.status.voided':'Չեղարկված','payment.reported':'Ձեր գրառումը','payment.kind.payment_reported':'Վճարում','payment.kind.prepayment_reported':'Լրացուցիչ վճարում','payment.kind.entry_voided':'Գրառման չեղարկում'
},{
 'payment.pause':'Already paid? Add the payment here. Next, we will ask for the updated figures from your bank.','payment.retry':'Save is unconfirmed. Retry checks the same record; fields stay locked.','payment.record':'Record a payment','payment.amount':'Amount paid','payment.date':'When did you pay?','payment.posting':'Has your bank processed it?','payment.pending':'Not yet / I am not sure','payment.posted':'Yes, it appears in my bank account','payment.value':'Date the bank processed it','payment.intent':'Payment type','payment.required':'Regular loan payment','payment.extra':'Extra payment','payment.review':'Check bank balances','payment.reconcile':'Update your loan balance and money left after this payment to continue planning.','payment.saved':'Payment record saved','payment.duplicate':'A similar payment exists. Record another separate payment?','payment.conflict':'Records changed. Reopen this page and review them before saving.','payment.notice':'Marum records payments; it does not transfer money.','payment.correct':'Update payment details','payment.void':'Remove payment record','payment.void.confirm':'Void this record? It will remain visible in history.','payment.status.pending_bank_posting':'Pending bank posting','payment.status.needs_reconciliation':'Check bank balances','payment.status.voided':'Voided','payment.reported':'User reported','payment.kind.payment_reported':'Payment','payment.kind.prepayment_reported':'Extra payment','payment.kind.entry_voided':'Record voided'
});
addStrings({'payment.allocation':'Ըստ ցանկության՝ վճարման մասերը','payment.allocation.unknown':'Չունեմ բանկի մանրամասները','payment.allocation.known':'Բանկը ցույց է տալիս բոլոր մասերը','payment.allocation.help':'Մուտքագրեք բանկի նշած բոլոր երեք մասերը, ներառյալ զրոները։ Գումարը պետք է հավասար լինի վճարմանը։','payment.principal':'Մայր գումար','payment.interest':'Տոկոս','payment.fees':'Վճարներ'}, {'payment.allocation':'Optional: payment breakdown','payment.allocation.unknown':'I do not have the breakdown','payment.allocation.known':'I have the bank breakdown','payment.allocation.help':'Enter all three bank-reported components, including explicit zeros. They must sum to the payment.','payment.principal':'Amount used to repay the loan','payment.interest':'Interest','payment.fees':'Fees'});
addStrings({'payment.done.next':'Ստուգել բանկի մնացորդները','payment.done.posted':'Վճարումը պահպանված է։ Հիմա նշեք բանկի թարմ մնացորդը և վարկերի համար մնացած գումարը։','payment.done.pending':'Վճարումը պահպանված է։ Երբ այն երևա բանկում, բացեք վճարումների պատմությունը և թարմացրեք այս գրառումը։','payment.history':'Բացել վճարումների պատմությունը','payment.dateError':'Վճարման ամսաթիվը չի կարող լինել ապագայում։ Բանկի մշակման օրը պետք է լինի վճարումից մինչև այսօր։','payment.splitError':'Բանկի երեք մասերի գումարը պետք է հավասար լինի վճարված գումարին։','payment.reload':'Թարմացնել էջը'}, {'payment.done.next':'Check bank balances','payment.done.posted':'Payment saved. Next, enter your updated loan balance and the money you have left for loans.','payment.done.pending':'Payment saved. When it appears in your bank account, open payment history and update this record.','payment.history':'Open payment history','payment.dateError':'The payment cannot be in the future. The bank processing date must be between the payment date and today.','payment.splitError':'The three bank amounts must add up to the amount paid.','payment.reload':'Reload this page'});
const $ = id => document.getElementById(id);
let context, correction, key, busy=false, request=null, generation=0;
const drafts=new Map();
const draftFields=['pay-amount','pay-date','pay-value','pay-posting','pay-intent','pay-allocation','pay-principal','pay-interest','pay-fees'];
const draftID=(id,fact)=>JSON.stringify([id,fact?.id||'']);
function rememberDraft(){
 if(!context||request||busy||$('payment-form').hidden)return;
 drafts.set(draftID(context.loan_id,correction),{values:Object.fromEntries(draftFields.map(id=>[id,$(id).value])),open:$('pay-allocation-wrap').open});
}
const unresolved=new Map(); // In-memory only; survives navigation and offline retry.
function lockFields(locked){ for(const el of document.querySelectorAll('#payment-form input,#payment-form select'))el.disabled=locked; }

function amountText(minor,exponent){
 const digits=BigInt(minor).toString().padStart(exponent+1,'0');
 return exponent?digits.slice(0,-exponent)+'.'+digits.slice(-exponent):digits;
}
function amountMinor(raw, exponent, allowZero=false) {
 if (!/^\d+(?:[.,]\d+)?$/.test(raw.trim())) throw new Error('amount');
 const [whole,part='']=raw.trim().replace(',','.').split('.');
 if(part.length>exponent) throw new Error('precision');
 const value=BigInt(whole)*10n**BigInt(exponent)+BigInt(part.padEnd(exponent,'0')||'0');
 if((allowZero?value<0n:value<=0n)||value>BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('amount');
 return Number(value);
}
function refreshAllocation(){ const known=$('pay-posting').value==='posted' && $('pay-allocation').value==='known';$('pay-allocation-fields').hidden=!known;for(const id of ['pay-principal','pay-interest','pay-fees'])$(id).required=known; }
function refreshPosting(){ $('pay-allocation-wrap').hidden=$('pay-posting').value!=='posted';refreshAllocation(); $('pay-value-wrap').hidden=$('pay-posting').value!=='posted'; $('pay-value').required=!$('pay-value-wrap').hidden; }
async function submit(e){
 e.preventDefault(); if(busy||!context) return;
 $('pay-error').textContent='';
 try {
  if(!request){
   const paid=$('pay-date').value, processed=$('pay-posting').value==='posted'?$('pay-value').value:'';
   if(!/^\d{4}-\d{2}-\d{2}$/.test(paid)||paid>context.today||($('pay-posting').value==='posted'&&!processed)||(processed&&(!/^\d{4}-\d{2}-\d{2}$/.test(processed)||processed<paid||processed>context.today)))throw new Error('date');
   request={idempotency_key:key,expected_version:context.version,amount_minor:amountMinor($('pay-amount').value,context.currency_exponent),transaction_date:$('pay-date').value,value_date:$('pay-posting').value==='posted'?$('pay-value').value:'',extra:$('pay-intent').value==='extra',replaces:correction?.id||''};
  }
   if($('pay-posting').value==='posted' && $('pay-allocation').value==='known' && !request.allocation){
   const allocation={principal_minor:amountMinor($('pay-principal').value,context.currency_exponent,true),interest_minor:amountMinor($('pay-interest').value,context.currency_exponent,true),fees_minor:amountMinor($('pay-fees').value,context.currency_exponent,true)};
   if(BigInt(allocation.principal_minor)+BigInt(allocation.interest_minor)+BigInt(allocation.fees_minor)!==BigInt(request.amount_minor))throw new Error('sum');
   request.allocation=allocation;
  }
 } catch (error) { request=null; $('pay-error').textContent=T(error.message==='date'?'payment.dateError':error.message==='sum'?'payment.splitError':'err.number'); return; }
 const loanID=context.loan_id, submitted=request, submittedGeneration=generation;
 const sameView=()=>generation===submittedGeneration && (typeof currentScreen!=='function'||currentScreen()==='payment');
 const stillHere=()=>sameView() && context?.loan_id===loanID && request===submitted;
 const clearSubmitted=()=>{if(unresolved.get(loanID)===submitted)unresolved.delete(loanID);};
 unresolved.set(loanID,submitted);
 busy=true; lockFields(true); $('pay-save').disabled=true;
 try {
  let res=await api('api/loans/'+encodeURIComponent(loanID)+'/payments',{method:'POST',body:JSON.stringify(submitted)});
  let body=await res.json();
  if(!res.ok && body.error==='possible_duplicate_payment' && stillHere() && await confirmDialog(T('payment.duplicate')) && stillHere()){
   submitted.allow_duplicate=true;
   res=await api('api/loans/'+encodeURIComponent(loanID)+'/payments',{method:'POST',body:JSON.stringify(submitted)}); body=await res.json();
  }
  if(!res.ok){
   if(!stillHere()){if(res.status<500)clearSubmitted();return;}
   $('pay-error').textContent=res.status>=500?T('payment.retry'):res.status===409?T('payment.conflict'):T('err.save');
   if(res.status===409)$('pay-reload').hidden=false;
   if(res.status<500){clearSubmitted();request=null;key=crypto.randomUUID();lockFields(false);}
   return;
  }
  clearSubmitted();drafts.delete(draftID(loanID,{id:submitted.replaces}));invalidate('api/'); if(stillHere()){request=null;toast(T('payment.saved')); $('payment-form').hidden=true;$('pay-done').hidden=false;$('pay-done-message').textContent=T(submitted.value_date?'payment.done.posted':'payment.done.pending');$('pay-check').hidden=!submitted.value_date;$('pay-check').onclick=()=>go('reconcile',{id:loanID});}
 }catch{ if(stillHere())$('pay-error').textContent=T('payment.retry'); /* Preserve the exact request/key after an uncertain response. */ }
 finally{if(sameView()){busy=false;$('pay-save').disabled=false;}}
}
register({id:'payment',parent:'activity',titleKey:'payment.record',html:`
 <form id="payment-form" class="card stack">
 <b id="pay-loan"></b><p class="hint" data-i18n="payment.notice"></p><p class="hint" data-i18n="payment.pause"></p>
 <div class="field"><label for="pay-amount" data-i18n="payment.amount"></label><div class="in"><input id="pay-amount" inputmode="decimal" required><span id="pay-currency" class="unit"></span></div></div>
 <div class="pair"><div class="field"><label for="pay-date" data-i18n="payment.date"></label><input id="pay-date" type="date" required></div><div class="field"><label for="pay-intent" data-i18n="payment.intent"></label><select id="pay-intent"><option value="required" data-i18n="payment.required"></option><option value="extra" data-i18n="payment.extra"></option></select></div></div>
 <div class="field"><label for="pay-posting" data-i18n="payment.posting"></label><select id="pay-posting"><option value="pending" data-i18n="payment.pending"></option><option value="posted" data-i18n="payment.posted"></option></select></div>
 <div id="pay-value-wrap" class="field" hidden><label for="pay-value" data-i18n="payment.value"></label><input id="pay-value" type="date"></div>
 <details id="pay-allocation-wrap" class="field" hidden><summary data-i18n="payment.allocation"></summary><label for="pay-allocation" data-i18n="payment.allocation"></label><select id="pay-allocation"><option value="unknown" data-i18n="payment.allocation.unknown"></option><option value="known" data-i18n="payment.allocation.known"></option></select>
 <div id="pay-allocation-fields" class="stack" hidden><p class="hint" data-i18n="payment.allocation.help"></p><div class="field"><label for="pay-principal" data-i18n="payment.principal"></label><input id="pay-principal" inputmode="decimal"></div><div class="field"><label for="pay-interest" data-i18n="payment.interest"></label><input id="pay-interest" inputmode="decimal"></div><div class="field"><label for="pay-fees" data-i18n="payment.fees"></label><input id="pay-fees" inputmode="decimal"></div></div>
 </details><button type="button" class="alink" id="pay-reload" data-i18n="payment.reload" hidden></button><p id="pay-error" class="error" role="alert"></p><button id="pay-save" class="cta" type="submit" data-i18n="save"></button>
 </form><section id="pay-done" class="card stack" hidden><b data-i18n="payment.saved"></b><p id="pay-done-message"></p><button type="button" class="cta" id="pay-check" data-i18n="payment.done.next"></button><button type="button" class="alink" data-go="activity" data-i18n="payment.history"></button></section>`,onMount(){ $('pay-reload').addEventListener('click',()=>{if($('pay-reload').loanID)go('payment',{id:$('pay-reload').loanID,fact:$('pay-reload').fact});});$('pay-posting').addEventListener('change',refreshPosting);$('pay-allocation').addEventListener('change',refreshAllocation);$('payment-form').addEventListener('submit',submit); },async onShow(_,params){
 rememberDraft();
 const view=++generation;busy=false;context=null;request=unresolved.get(params?.id)||null;key=request?.idempotency_key||crypto.randomUUID();lockFields(false);correction=params?.fact||null;
 $('pay-reload').loanID=params?.id;$('pay-reload').fact=correction;$('pay-reload').hidden=true;$('payment-form').hidden=false;$('pay-done').hidden=true;$('pay-allocation-wrap').open=false;$('payment-form').reset();$('pay-loan').textContent='';$('pay-currency').textContent='';$('pay-error').textContent='';$('pay-save').disabled=true;
 try{
  if(!params?.id) throw new Error('loan');
  const res=await api('api/loans/'+encodeURIComponent(params.id)+'/payments',{cache:'no-store'});
  if(!res.ok)throw new Error('load');const loaded=await res.json();
  if(view!==generation)return;
  if(loaded.loan_id!==params.id||!Number.isSafeInteger(loaded.version)||loaded.version<0||!/^\d{4}-\d{2}-\d{2}$/.test(loaded.today)||!Number.isInteger(loaded.currency_exponent)||loaded.currency_exponent<0||loaded.currency_exponent>6)throw new Error('metadata');
  context=loaded;
  $('pay-loan').textContent=context.loan;$('pay-currency').textContent=context.currency;
  $('pay-date').max=context.today;$('pay-value').max=context.today;
  $('pay-date').value=correction?.transaction_date||context.today;$('pay-value').value=correction?.value_date||context.today;
  if(correction){$('pay-amount').value=amountText(correction.amount_minor,context.currency_exponent);$('pay-intent').value=correction.kind==='prepayment_reported'?'extra':'required';}
  $('pay-posting').value=correction?.value_date?'posted':'pending';
  if(request){
   $('pay-amount').value=amountText(request.amount_minor,context.currency_exponent);
   $('pay-date').value=request.transaction_date;$('pay-value').value=request.value_date||context.today;
   $('pay-posting').value=request.value_date?'posted':'pending';$('pay-intent').value=request.extra?'extra':'required';
   $('pay-error').textContent=T('payment.retry');
  }
  const allocation=(request||correction)?.allocation;
  $('pay-allocation').value=allocation?'known':'unknown';
  for(const [id,name] of [['pay-principal','principal_minor'],['pay-interest','interest_minor'],['pay-fees','fees_minor']])$(id).value=allocation?amountText(allocation[name],context.currency_exponent):'';
  const draft=!request&&drafts.get(draftID(params.id,correction));
  if(draft)for(const id of draftFields)$(id).value=draft.values[id];
  refreshPosting();$('pay-allocation-wrap').open=draft?draft.open:!!allocation;lockFields(!!request);$('pay-save').disabled=false;
 }catch{if(view===generation){$('pay-error').textContent=T('err.load');$('pay-reload').hidden=false;}}
}});
