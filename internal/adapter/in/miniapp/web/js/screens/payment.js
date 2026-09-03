"use strict";
import {register,go} from '../nav.js';
import {api,getJSON,invalidate} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {toast,confirmDialog} from '../core.js';

addStrings({
 'payment.pause':'Նոր պլանը կսպասի բանկի հետ համադրմանը։','payment.retry':'Պահպանումը հաստատված չէ։ Կրկին սեղմեք՝ նույն գրառումը ստուգելու համար։','payment.record':'Գրանցել վճարումը','payment.amount':'Վճարված գումար','payment.date':'Վճարման ամսաթիվ','payment.posting':'Բանկի գրանցում','payment.pending':'Դեռ հայտնի չէ','payment.posted':'Գրանցվել է բանկում','payment.value':'Բանկի գրանցման ամսաթիվ','payment.intent':'Վճարման տեսակ','payment.required':'Պարտադիր','payment.extra':'Լրացուցիչ','payment.review':'Համադրման կարիք կա','payment.reconcile':'Գրառումը դեռ չի համադրվել բանկի մնացորդի և բյուջեի հետ։ Նոր պլանը ժամանակավորապես կասեցված է։','payment.saved':'Վճարման գրառումը պահպանված է','payment.duplicate':'Նման վճարում արդեն կա։ Գրանցե՞լ ևս մեկ առանձին վճարում։','payment.conflict':'Գրառումները փոխվել են։ Վերաբացեք էջը և ստուգեք դրանք։','payment.notice':'Marum-ը գումար չի փոխանցում։','payment.correct':'Ուղղել','payment.void':'Չեղարկել գրառումը','payment.void.confirm':'Չեղարկե՞լ այս գրառումը։ Այն կմնա պատմության մեջ։','payment.status.pending_bank_posting':'Սպասում է բանկի գրանցմանը','payment.status.needs_reconciliation':'Համադրման կարիք կա','payment.status.voided':'Չեղարկված','payment.reported':'Ձեր գրառումը','payment.kind.payment_reported':'Վճարում','payment.kind.prepayment_reported':'Լրացուցիչ վճարում','payment.kind.entry_voided':'Գրառման չեղարկում'
},{
 'payment.pause':'New plans wait for bank reconciliation.','payment.retry':'Save is unconfirmed. Retry checks the same record; fields stay locked.','payment.record':'Quick Record','payment.amount':'Amount paid','payment.date':'Transaction date','payment.posting':'Bank posting','payment.pending':'Not known yet','payment.posted':'Posted by bank','payment.value':'Bank value date','payment.intent':'Payment type','payment.required':'Required','payment.extra':'Extra','payment.review':'Payment needs review','payment.reconcile':'This record has not been reconciled with your bank balance and budget. Further planning is paused.','payment.saved':'Payment record saved','payment.duplicate':'A similar payment exists. Record another separate payment?','payment.conflict':'Records changed. Reopen this page and review them before saving.','payment.notice':'Marum records payments; it does not transfer money.','payment.correct':'Correct / update posting','payment.void':'Void record','payment.void.confirm':'Void this record? It will remain visible in history.','payment.status.pending_bank_posting':'Pending bank posting','payment.status.needs_reconciliation':'Needs reconciliation','payment.status.voided':'Voided','payment.reported':'User reported','payment.kind.payment_reported':'Payment','payment.kind.prepayment_reported':'Extra payment','payment.kind.entry_voided':'Record voided'
});
const $ = id => document.getElementById(id);
let context, correction, key, busy=false, request=null;
const unresolved=new Map(); // In-memory only; survives navigation and offline retry.
function lockFields(locked){ for(const el of document.querySelectorAll('#payment-form input,#payment-form select'))el.disabled=locked; }

function amountMinor(raw, exponent) {
 if (!/^\d+(?:[.,]\d+)?$/.test(raw.trim())) throw new Error('amount');
 const [whole,part='']=raw.trim().replace(',','.').split('.');
 if(part.length>exponent) throw new Error('precision');
 const value=BigInt(whole)*10n**BigInt(exponent)+BigInt(part.padEnd(exponent,'0')||'0');
 if(value<=0n||value>BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('amount');
 return Number(value);
}
function refreshPosting(){ $('pay-value-wrap').hidden=$('pay-posting').value!=='posted'; $('pay-value').required=!$('pay-value-wrap').hidden; }
async function submit(e){
 e.preventDefault(); if(busy||!context) return;
 $('pay-error').textContent='';
 try {
  if(!request) request={idempotency_key:key,expected_version:context.version,amount_minor:amountMinor($('pay-amount').value,context.currency_exponent),transaction_date:$('pay-date').value,value_date:$('pay-posting').value==='posted'?$('pay-value').value:'',extra:$('pay-intent').value==='extra',replaces:correction?.id||''};
 } catch { $('pay-error').textContent=T('err.number'); return; }
 const loanID=context.loan_id, submitted=request;
 const stillHere=()=>context?.loan_id===loanID && request===submitted;
 unresolved.set(loanID,submitted);
 busy=true; lockFields(true); $('pay-save').disabled=true;
 try {
  let res=await api('api/loans/'+encodeURIComponent(loanID)+'/payments',{method:'POST',body:JSON.stringify(submitted)});
  let body=await res.json();
  if(!res.ok && body.error==='possible_duplicate_payment' && stillHere() && await confirmDialog(T('payment.duplicate'))){
   submitted.allow_duplicate=true;
   res=await api('api/loans/'+encodeURIComponent(loanID)+'/payments',{method:'POST',body:JSON.stringify(submitted)}); body=await res.json();
  }
  if(!res.ok){
   if(!stillHere()){if(res.status<500)unresolved.delete(loanID);return;}
   $('pay-error').textContent=res.status>=500?T('payment.retry'):res.status===409?T('payment.conflict'):T('err.save');
   if(res.status<500){unresolved.delete(loanID);request=null;key=crypto.randomUUID();lockFields(false);}
   return;
  }
  unresolved.delete(loanID);invalidate('api/'); if(stillHere()){request=null;toast(T('payment.saved')); go('activity');}
 }catch{ if(stillHere())$('pay-error').textContent=T('payment.retry'); /* Preserve the exact request/key after an uncertain response. */ }
 finally{busy=false;if(context?.loan_id===loanID)$('pay-save').disabled=false;}
}
register({id:'payment',parent:'activity',titleKey:'payment.record',html:`
 <form id="payment-form" class="card stack">
 <b id="pay-loan"></b><p class="hint" data-i18n="payment.notice"></p><p class="hint" data-i18n="payment.pause"></p>
 <div class="field"><label for="pay-amount" data-i18n="payment.amount"></label><div class="in"><input id="pay-amount" inputmode="decimal" required><span id="pay-currency" class="unit"></span></div></div>
 <div class="pair"><div class="field"><label for="pay-date" data-i18n="payment.date"></label><input id="pay-date" type="date" required></div><div class="field"><label for="pay-intent" data-i18n="payment.intent"></label><select id="pay-intent"><option value="required" data-i18n="payment.required"></option><option value="extra" data-i18n="payment.extra"></option></select></div></div>
 <div class="field"><label for="pay-posting" data-i18n="payment.posting"></label><select id="pay-posting"><option value="pending" data-i18n="payment.pending"></option><option value="posted" data-i18n="payment.posted"></option></select></div>
 <div id="pay-value-wrap" class="field" hidden><label for="pay-value" data-i18n="payment.value"></label><input id="pay-value" type="date"></div>
 <p id="pay-error" class="error" role="alert"></p><button id="pay-save" class="cta" type="submit" data-i18n="save"></button>
 </form>`,onMount(){ $('pay-posting').addEventListener('change',refreshPosting);$('payment-form').addEventListener('submit',submit); },async onShow(_,params){
 context=null;request=unresolved.get(params?.id)||null;key=request?.idempotency_key||crypto.randomUUID();lockFields(false);correction=params?.fact||null;
 $('payment-form').reset();$('pay-error').textContent='';$('pay-save').disabled=true;
 try{
  if(!params?.id) throw new Error('loan');
  context=await getJSON('api/loans/'+encodeURIComponent(params.id)+'/payments');
  $('pay-loan').textContent=context.loan;$('pay-currency').textContent=context.currency;
  $('pay-date').max=context.today;$('pay-value').max=context.today;
  $('pay-date').value=correction?.transaction_date||context.today;$('pay-value').value=correction?.value_date||context.today;
  if(correction){$('pay-amount').value=(correction.amount_minor/10**context.currency_exponent).toFixed(context.currency_exponent);$('pay-intent').value=correction.kind==='prepayment_reported'?'extra':'required';}
  $('pay-posting').value=correction?.value_date?'posted':'pending';
  if(request){
   $('pay-amount').value=(request.amount_minor/10**context.currency_exponent).toFixed(context.currency_exponent);
   $('pay-date').value=request.transaction_date;$('pay-value').value=request.value_date||context.today;
   $('pay-posting').value=request.value_date?'posted':'pending';$('pay-intent').value=request.extra?'extra':'required';
   $('pay-error').textContent=T('payment.retry');
  }
  refreshPosting();lockFields(!!request);$('pay-save').disabled=false;
 }catch{$('pay-error').textContent=T('err.load');}
}});
