"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {loanMutation} from '../loan-mutations.js';
import {majorAmount} from './budget-funding.js';
import {extraMinor,extraText} from './projection-format.js';
import {addStrings,T,sub} from '../i18n.js';
import {toast,haptic,fmtFull,fmtMonth} from '../core.js';
addStrings({
 'pm.accrued':'Այսօրվա կուտակված տոկոսը (ըստ ցանկության)','pm.accruedHint':'Բանկի առանձին նշած տոկոսը։ Դատարկը՝ չգիտեմ, 0-ը՝ տոկոս չկա։',
 'pm.title':'Վճարված ամիսներ','pm.hint':'Նշեք մինչև որ ամիսն են պարտադիր վճարումները կատարված և հաստատեք բանկի այսօրվա թվերը։ Սա թարմացնում է վարկի մնացորդն ու հաջորդ վճարումը, բայց չի փոխում ձեր հասանելի գումարը։',
 'pm.month':'Պարտադիր վճարումները կատարված են մինչև','pm.balance':'Մայր գումարի մնացորդն այսօր','pm.payment':'Հաջորդ պարտադիր վճարումը','pm.confirm':'Բանկում ստուգել եմ՝ այս ամիսների վճարումները կատարված են, իսկ նշված թվերն արդիական են։',
 'pm.next':'Հաջորդ վճարումը՝ {date}','pm.save':'Հաստատել վճարված ամիսները','pm.invalid':'Ստուգեք ամիսը, գումարները և հաստատման նշումը։ Ամբողջությամբ մարված վարկի համար երկու գումարներն էլ զրո նշեք։',
 'pm.reload':'Վերբեռնել՝ փոփոխությունները հեռացնելով','pm.review':'Նախ համադրեք նախկինում գրանցված վճարումները։','pm.zero':'Այս ամսվա համար հաջորդ վճարում չկա։ Ամբողջությամբ մարված վարկի համար երկու գումարներն էլ զրո նշեք։ Հակառակ դեպքում ստուգեք վարկի ամսաթվերը կամ բանկի մնացորդները։','pm.currency':'Գումարների արժույթը՝ {currency}',
 'pm.rejected':'Թվերը չեն համապատասխանում վարկի պայմաններին, կամ ընտրված ամսից հետո վճարման օրը այլևս ապագայում չէ։ Վերբեռնեք և ստուգեք բանկի մնացորդը, հաջորդ վճարումն ու պայմանները։',
 'pm.current':'Արդեն նշված է վճարված մինչև {month}',
 'pm.saved':'Վճարված ամիսները պահպանված են։ Պլանը թարմացվել է։'
},{
 'pm.accrued':'Interest accrued today (optional)','pm.accruedHint':'Interest shown separately by your bank. Blank means unknown; 0 means none.',
 'pm.title':'Paid months','pm.hint':'Declare through which month required payments are complete and confirm today’s figures from your bank. This updates the loan balance and next payment, but does not update your available cash.',
 'pm.month':'Required payments completed through','pm.balance':'Principal balance today','pm.payment':'Next required payment','pm.confirm':'I checked with my bank: these months are paid and the figures entered here are current.',
 'pm.next':'Next payment: {date}','pm.save':'Confirm paid months','pm.invalid':'Check the month, amounts and confirmation. For a fully repaid loan, enter zero for both amounts.',
 'pm.reload':'Reload and discard changes','pm.review':'Check previously recorded payments and update the bank balances first.','pm.zero':'No future payment is available for this month. If the loan is fully repaid, enter zero for both amounts. Otherwise check the loan dates or update its bank balances.','pm.currency':'Amounts in {currency}',
 'pm.rejected':'The figures are inconsistent with the loan terms, or the payment after this month is no longer in the future. Reload and check the bank balance, next payment and loan terms.',
 'pm.current':'Already marked paid through {month}',
 'pm.saved':'Paid months saved. Your plan is updated.'
});
const $=id=>document.getElementById(id),states=new Map();let active=null;
function path(s){return 'api/loans/'+encodeURIComponent(s.id)+'/paid-months';}
function render(){
 const s=active;if(!s)return;
 $('pm-fields').disabled=!s.doc||s.loading||s.busy||!!s.pending||s.conflict||s.doc.needs_reconciliation;
 $('pm-retry').hidden=!s.pending;$('pm-retry').disabled=!!s.busy;
 $('pm-reload').hidden=!!s.pending||s.busy||s.loading||(!s.error&&!s.conflict);
 $('pm-review').hidden=!s.doc?.needs_reconciliation;
 $('pm-status').textContent=s.loading?T('loading'):s.error?T(s.error):'';
 $('pm-name').textContent=s.doc?.name||'';
 $('pm-current').hidden=!s.doc?.paid_through;
 $('pm-current').textContent=s.doc?.paid_through?sub('pm.current',{month:fmtMonth(s.doc.paid_through+"-01")}):'';
 $('pm-currency').textContent=s.doc?sub('pm.currency',{currency:s.doc.currency}):'';
 if(!s.doc)return;
 const months=[...new Set([...Object.keys(s.doc.next_dates),s.doc.today.slice(0,7)])].sort().reverse();
 $('pm-month').replaceChildren(...months.map(month=>{const option=document.createElement('option');option.value=month;option.textContent=fmtMonth(month+"-01");return option;}));
 for(const field of ['month','balance','payment','accrued'])$('pm-'+field).value=s.values[field];
 $('pm-confirm').checked=s.values.confirmed;
 const next=s.doc.next_dates[s.values.month];$('pm-next').textContent=next?sub('pm.next',{date:fmtFull(next)}):T('pm.zero');
}
async function load(s,discard=false){
 if(s.loading||s.busy||s.pending||(s.dirty&&!discard)){render();return;}
 s.loading=true;s.error=null;render();
 try{
  const response=await api(path(s),{cache:'no-store'});if(!response.ok)throw new Error('load');
  const d=await response.json();if(!Number.isSafeInteger(d.version)||!/^\d{4}-\d{2}-\d{2}$/.test(d.today)||!d.next_dates||!Number.isInteger(d.currency_exponent))throw new Error('document');
  s.doc=d;s.values={month:d.today.slice(0,7),balance:String(d.balance_major),payment:d.payment_major==null?'':String(d.payment_major),accrued:'',confirmed:false};
  s.dirty=false;s.conflict=false;s.error=d.needs_reconciliation?'pm.review':null;
 }catch{s.doc=null;s.error='err.load';}
 finally{s.loading=false;if(active===s)render();}
}
function changed(){
 if(!active||active.pending||active.busy)return;
 active.values={month:$('pm-month').value,balance:$('pm-balance').value,payment:$('pm-payment').value,accrued:$('pm-accrued').value,confirmed:$('pm-confirm').checked};active.dirty=true;
 const next=active.doc?.next_dates[active.values.month];$('pm-next').textContent=next?sub('pm.next',{date:fmtFull(next)}):T('pm.zero');
}
async function save(event){
 event?.preventDefault();const s=active;if(!s?.doc||s.busy||s.loading||s.conflict||s.doc.needs_reconciliation)return;
 if(!s.pending){
  changed();
  try{
   const balance=majorAmount(s.values.balance,s.doc.currency_exponent),payment=majorAmount(s.values.payment,s.doc.currency_exponent);
   if(!s.values.confirmed||(!s.doc.next_dates[s.values.month]&&balance>0)||(balance===0?payment!==0:payment<=0))throw new Error('statement');
   const interest=s.values.accrued.trim()?extraMinor(s.values.accrued,s.doc.currency_exponent):null;
   if(s.values.accrued.trim()&&(interest===null||(balance===0&&interest>0)))throw new Error('interest');
   s.pending={...(interest===null?{}:{accrued_interest_major:extraText(interest,s.doc.currency_exponent)}),month:s.values.month,as_of:s.doc.today,balance_major:balance,payment_major:payment,confirmed:true};
  }catch{s.error='pm.invalid';render();haptic.bad();return;}
 }
 s.busy=true;s.error=null;render();
 try{
  const response=await loanMutation(path(s),'POST',s.pending,s.doc.version);
  if(!response.ok){
   if(response.status>=400&&response.status<500){
    s.pending=null;const error=await response.json().catch(()=>({}));s.conflict=response.status===409;
    s.error=error.error==='payment_reconciliation_required'?'pm.review':s.conflict?'loan.write.conflict':error.error==='invalid_paid_month'?'pm.rejected':'pm.invalid';
    if(error.error==='payment_reconciliation_required')s.doc.needs_reconciliation=true;
    haptic.bad();return;
   }
   throw new Error('uncertain');
  }
  s.pending=null;s.dirty=false;states.delete(s.id);invalidate('api/');toast(T('pm.saved'));haptic.ok();
  if(active===s&&currentScreen()==='paid-months')go('simple-plan');
 }catch{s.error='loan.write.retry';haptic.bad();}
 finally{s.busy=false;if(active===s)render();}
}
register({id:'paid-months',parent:'loans',titleKey:'pm.title',html:`
 <div class="stack"><b id="pm-name"></b><p id="pm-current" class="hint" hidden></p><p class="hint" data-i18n="pm.hint"></p><p id="pm-status" class="error" role="alert"></p>
 <form id="pm-form" novalidate><fieldset id="pm-fields" class="card stack" style="border:0;min-width:0" disabled>
 <label for="pm-month" data-i18n="pm.month"></label><select id="pm-month"></select><p id="pm-next" class="hint" aria-live="polite"></p>
 <p id="pm-currency" class="hint"></p><label for="pm-balance" data-i18n="pm.balance"></label><input id="pm-balance" inputmode="decimal" required>
 <label for="pm-payment" data-i18n="pm.payment"></label><input id="pm-payment" inputmode="decimal" required>
 <details class="fold"><summary data-i18n="pm.accrued"></summary><div class="fold-body"><label for="pm-accrued" data-i18n="pm.accrued"></label><input id="pm-accrued" inputmode="decimal"><p class="hint" data-i18n="pm.accruedHint"></p></div></details>
 <label><input id="pm-confirm" type="checkbox"><span data-i18n="pm.confirm"></span></label>
 <button type="submit" class="cta" data-i18n="pm.save"></button></fieldset></form>
 <button id="pm-retry" class="cta" type="button" data-i18n="loan.write.check" hidden></button><button id="pm-reload" class="alink" type="button" data-i18n="pm.reload" hidden></button>
 <button id="pm-review" class="alink" type="button" data-go="activity" data-i18n="payment.review" hidden></button>
 </div>`,
 onMount(){ $('pm-form').addEventListener('submit',save);$('pm-form').addEventListener('input',changed);$('pm-form').addEventListener('change',changed);$('pm-retry').onclick=save;$('pm-reload').onclick=()=>load(active,true); },
 onShow(_el,params){const id=params?.id;if(!id){go('loans');return;}if(!states.has(id))states.set(id,{id,values:{}});active=states.get(id);render();return load(active);},onLanguage:render
});
