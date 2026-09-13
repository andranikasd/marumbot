"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api} from '../api.js';
import {addStrings,T,sub} from '../i18n.js';
import {esc,fmtMoney,fmtMonth,fmtDate} from '../core.js';
import {icon} from '../icons.js';
import {validProjection} from './projection-format.js';
addStrings({
 'sp.reason.bank_payment_mismatch':'Ստուգեք բանկի պարտադիր վճարումն ու մնացորդը։','sp.reason.non_amortizing_payment':'Վճարումը չի ծածկում հաշվարկված տոկոսը։ Ստուգեք բանկի տվյալները։',
 'sp.review.help':'Պլանը կթարմացվի բանկի մնացորդը հաստատելուց հետո։','sp.review.action':'Ստուգել բանկի մնացորդները','sp.title':'Իմ պլանը','sp.required':'Պարտադիր','sp.extra':'Հավելյալ','sp.total':'Այս ամսվա ընդհանուր գումարը',
 'sp.finish':'Մոտավոր ավարտը՝ {month}','sp.change':'Փոխել այս ամսվա հավելյալը','sp.usual':'Սովորական հավելյալ գումարը',
 'sp.assumption':'Պարտադիր վճարումները՝ իրենց օրերին, հավելյալը՝ ամսվա վերջում։',
 'sp.details':'Ինչպես է հաշվարկվում','sp.detail.text':'Այս պլանը ենթադրում է, որ կարող եք վճարել պարտադիր և ընտրված հավելյալ գումարները ամսվա ընթացքում։ Այն չի ստուգում՝ այսօր որքան գումար ունեք։ Վարկի մարումից հետո հավելյալ գումարն ինքնաբերաբար չի ավելանում։',
 'sp.incomplete':'Ամբողջական կանխատեսման համար ստուգեք վարկի տվյալները։','sp.check':'Ստուգել վարկերը',
 'sp.known':'Ցուցադրված են միայն հայտնի պարտադիր վճարումները։ Ավարտի ժամկետ դեռ չկա։',
 'sp.empty':'Սկսեք ձեր առաջին վարկից','sp.empty.hint':'Մի քանի պարզ քայլից հետո ձեր պլանը կհայտնվի այստեղ։','sp.start':'Սկսել',
 'sp.setup':'Ընտրեք ձեր հավելյալ գումարը','sp.setup.hint':'Պարտադիր վճարումները պատրաստ են։ Նշեք՝ որքան կարող եք ավելացնել։','sp.choose':'Ընտրել հավելյալ գումարը',
 'sp.repaid':'Ակտիվ պարտք չունեք','sp.repaid.hint':'Ձեր վարկերի պատմությունը պահպանված է։','sp.loans':'Իմ վարկերը',
 'sp.month':'Ամիս','sp.currency':'Արժույթ','sp.none':'Այս ամսվա համար վճարում չկա։',
 'sp.reminders':'Հիշեցնել վճարումներից առաջ','sp.notnow':'Ոչ հիմա','sp.reminder.hint':'Ընտրովի է․ կարող եք միացնել նաև կարգավորումներից։',
 'sp.paid':'Նշել վճարված','sp.date':'Պարտադիր՝ {date}','sp.extra.end':'Հավելյալ՝ ամսվա վերջում',
 'sp.history':'Նախկին բյուջեն և պլանները','sp.overdue':'Վճարման օրն անցել է։ Եթե վճարել եք, թարմացրեք բանկի տվյալները։',
 'sp.reason.payment_reconciliation_required':'Հաստատեք բանկի թարմ մնացորդը։',
 'sp.reason.unknown_terms':'Ավելացրեք տոկոսադրույքը և բանկի վաղաժամկետ մարման կանոնը։',
 'sp.reason.overdue':'Ստուգեք չվճարված գումարները բանկի հավելվածում։',
 'sp.reason.accrued_interest_needed':'Ավարտը հաշվարկելու համար նշեք բանկում այս պահին հաշվարկված տոկոսագումարը։','sp.reason.interest_needed':'Ավելացրեք բանկի տոկոսադրույքը։','sp.reason.bank_payment_needed':'Ավելացրեք բանկի պարտադիր վճարումը։','sp.reason.early_payment_rules_needed':'Ստուգեք բանկի վաղաժամկետ մարման կանոնը։',
 'sp.requested':'Ընտրված հավելյալը','sp.unallocated':'Դեռ չբաշխված','sp.unallocated.hint':'Չբաշխված գումարը վճարման առաջարկ չէ։ Եթե վարկի տվյալները պակասում են, լրացրեք դրանք․ մարումից հետո մնացած գումարը ձեզ է մնում։','sp.known.required':'Հայտնի պարտադիր վճարումները'
},{
 'sp.reason.bank_payment_mismatch':'Check the bank’s required payment and balance.','sp.reason.non_amortizing_payment':'This payment does not cover the calculated interest. Check the bank details.',
 'sp.review.help':'Your plan will update after you confirm the bank balance.','sp.review.action':'Check bank balances','sp.title':'My plan','sp.required':'Required','sp.extra':'Extra','sp.total':'Total this month',
 'sp.finish':'Estimated finish · {month}','sp.change':'Change this month’s extra','sp.usual':'Usual monthly extra',
 'sp.assumption':'Required payments on their due dates. Extra at month-end.',
 'sp.details':'How this is calculated','sp.detail.text':'This plan assumes you can cover required payments and your chosen extra during each month. It does not check the money available today. When a loan ends, your extra does not automatically increase.',
 'sp.incomplete':'Check loan details for a full forecast.','sp.check':'Check loans',
 'sp.known':'Only known required payments are shown. There is no payoff date yet.',
 'sp.empty':'Start with your first loan','sp.empty.hint':'A few simple steps, then your plan appears here.','sp.start':'Get started',
 'sp.setup':'Choose your extra','sp.setup.hint':'Your required payments are ready. Choose what you can add.','sp.choose':'Choose extra amount',
 'sp.repaid':'You have no active debt','sp.repaid.hint':'Your loan history is still saved.','sp.loans':'My loans',
 'sp.month':'Month','sp.currency':'Currency','sp.none':'No payment is planned for this month.',
 'sp.reminders':'Remind me before payments','sp.notnow':'Not now','sp.reminder.hint':'Optional. You can enable this later in Settings.',
 'sp.paid':'Mark paid','sp.date':'Required · {date}','sp.extra.end':'Extra · month-end',
 'sp.history':'Previous budget and plans','sp.overdue':'This due date has passed. If you paid, update the bank details.',
 'sp.reason.payment_reconciliation_required':'Confirm the updated bank balance.',
 'sp.reason.unknown_terms':'Add interest and your bank’s early-payment rule.',
 'sp.reason.overdue':'Check outstanding payments in your bank app.',
 'sp.reason.accrued_interest_needed':'Add the interest currently owed at your bank to calculate a finish date.','sp.reason.interest_needed':'Add the bank’s interest rate.','sp.reason.bank_payment_needed':'Add the bank’s required payment.','sp.reason.early_payment_rules_needed':'Check the bank’s early-payment rule.',
 'sp.requested':'Your chosen extra','sp.unallocated':'Not allocated','sp.unallocated.hint':'Unallocated money is not a payment suggestion. Add missing loan details if needed; money left after repayment stays yours.','sp.known.required':'Known required payments'
});
const $=id=>document.getElementById(id);
let documentValue=null,loanCount=null,selectedCurrency='',selectedMonth='',offerReminders=false,generation=0;
const button=(key,screen,primary=false)=>`<button class="${primary?'cta':'alink'}" data-go="${screen}">${esc(T(key))}</button>`;
function render(){
 const p=documentValue,root=$('sp-content');if(!p)return;
 const currencies=p.currencies;
 if(!currencies.length){
  root.innerHTML=`<section class="card stack"><h2>${esc(T(loanCount===0?'sp.empty':'sp.repaid'))}</h2><p class="hint">${esc(T(loanCount===0?'sp.empty.hint':'sp.repaid.hint'))}</p>${button(loanCount===0?'sp.start':'sp.loans',loanCount===0?'welcome':'loans',true)}</section>`;return;
 }
 const c=currencies.find(c=>c.currency===selectedCurrency)||currencies[0];selectedCurrency=c.currency;
 const m=c.months.find(m=>m.month===selectedMonth)||c.months[0];selectedMonth=m?.month||'';
 const format=n=>fmtMoney(n/10**c.exponent,c.currency);
 const reason=c.reason==='overdue_payment'?'overdue':c.reason;
 const reasonKey=['bank_payment_mismatch','non_amortizing_payment','payment_reconciliation_required','unknown_terms','overdue','accrued_interest_needed','interest_needed','bank_payment_needed','early_payment_rules_needed'].includes(reason)?`sp.reason.${reason}`:'sp.incomplete';
 root.innerHTML=`${!p.enabled?`<section class="card stack"><b>${esc(T('sp.setup'))}</b><p class="hint">${esc(T('sp.setup.hint'))}</p>${button('sp.choose','extra',true)}</section>`:''}
 ${currencies.length>1?`<label for="sp-currency">${esc(T('sp.currency'))}</label><select id="sp-currency">${currencies.map(g=>`<option${g.currency===c.currency?' selected':''}>${esc(g.currency)}</option>`).join('')}</select>`:''}
 ${c.months.length?`<label for="sp-month">${esc(T('sp.month'))}</label><select id="sp-month">${c.months.map(row=>`<option value="${row.month}"${row.month===selectedMonth?' selected':''}>${esc(fmtMonth(row.month+'-01'))}</option>`).join('')}</select>`:''}
 ${!c.complete?`<section class="card stack"><b>${esc(T(reasonKey))}</b><p class="hint">${esc(T('sp.known'))}</p>${button('sp.check','loans')}</section>`:''}
 ${m?`<section class="card stack"><span>${esc(T('sp.total'))}</span><strong class="v num">${esc(format(m.total_minor))}</strong><div class="spread"><span>${esc(T(c.complete?'sp.required':'sp.known.required'))}</span><b class="num">${esc(format(m.required_minor))}</b></div><div class="spread"><span>${esc(T('sp.extra'))}</span><b class="num">${esc(format(m.extra_minor))}</b></div>${m.unallocated_extra_minor>0?`<div class="spread"><span>${esc(T('sp.requested'))}</span><b class="num">${esc(format(m.requested_extra_minor))}</b></div><div class="spread"><span>${esc(T('sp.unallocated'))}</span><b class="num">${esc(format(m.unallocated_extra_minor))}</b></div><p class="hint">${esc(T('sp.unallocated.hint'))}</p>`:''}${p.enabled?`<button class="alink" id="sp-change">${esc(T('sp.change'))}</button>`:''}</section>
 ${m.loans.map(l=>`<section class="card stack"><b>${esc(l.name)}</b>${l.required_minor?`<div class="spread"><span>${esc(l.due?sub('sp.date',{date:fmtDate(l.due)}):T('sp.required'))}</span><strong class="num">${esc(format(l.required_minor))}</strong></div>`:''}${l.extra_minor?`<div class="spread"><span>${esc(T('sp.extra.end'))}</span><strong class="num">${esc(format(l.extra_minor))}</strong></div>`:''}${l.due&&l.due<p.today&&l.required_minor?`<p class="hint">${esc(T('sp.overdue'))}</p>`:''}${m.month<=p.today.slice(0,7)?`<button class="alink" data-go="paid-months" data-arg="${esc(l.id)}">${esc(T('sp.paid'))}</button>`:''}</section>`).join('')}`:`<p class="hint">${esc(T('sp.none'))}</p>`}
 ${c.complete&&c.finish?`<p><b>${esc(sub('sp.finish',{month:fmtMonth(c.finish.length===7?c.finish+'-01':c.finish)}))}</b></p>`:''}
 ${offerReminders&&p.enabled?`<section id="sp-reminder" class="card stack"><p class="hint">${esc(T('sp.reminder.hint'))}</p>${button('sp.reminders','reminder-setup',true)}<button class="alink" id="sp-notnow">${esc(T('sp.notnow'))}</button></section>`:''}
 <p class="hint">${esc(T('sp.assumption'))}</p><details class="card"><summary>${esc(T('sp.details'))}</summary><p class="hint">${esc(T('sp.detail.text'))}</p>${p.enabled?button('sp.usual','extra'):''}${button('sp.history','plan')}</details>`;
 $('sp-currency')?.addEventListener('change',e=>{selectedCurrency=e.target.value;selectedMonth='';render();});
 $('sp-month')?.addEventListener('change',e=>{selectedMonth=e.target.value;render();});
 if($('sp-change'))$('sp-change').onclick=()=>go('extra',{currency:selectedCurrency,month:selectedMonth});
 if($('sp-notnow'))$('sp-notnow').onclick=()=>{offerReminders=false;render();};
}
async function load(){
 const revision=++generation;$('sp-content').innerHTML=`<p class="state" role="status">${esc(T('loading'))}</p>`;
 try{
  const [res,loansRes]=await Promise.all([api('api/projection',{cache:'no-store'}),api('api/loans',{cache:'no-store'})]);
  if(!res.ok){
   if(res.status===422){const failure=await res.json().catch(()=>({}));if(failure.error==='payment_reconciliation_required')throw new Error('review');}
   throw new Error('load');
  }
  if(!loansRes.ok)throw new Error('load');
  const [p,l]=await Promise.all([res.json(),loansRes.json()]);if(revision!==generation)return;
  if(!validProjection(p)||!Array.isArray(l.loans))throw new Error('document');
  documentValue=p;loanCount=l.loans.length;
  if(!p.enabled&&loanCount===0&&currentScreen()==='simple-plan'){go('welcome');return;}
  render();
 }catch(error){
  if(revision===generation&&error.message==='review'){
   $('sp-content').innerHTML=`<section class="card stack"><h2>${esc(T('sp.reason.payment_reconciliation_required'))}</h2><p class="hint">${esc(T('sp.review.help'))}</p>${button('sp.review.action','activity',true)}</section>`;return;
  }
  if(revision===generation)$('sp-content').innerHTML=`<div class="state"><p role="alert">${esc(T('err.load'))}</p><button class="cta" data-go="simple-plan">${esc(T('retry'))}</button></div>`;}
}
register({id:'simple-plan',icon:icon('document'),titleKey:'sp.title',labelKey:'tab.plan',html:'<div class="stack" id="sp-content"></div>',
 onShow(_root,params){if(params?.currency)selectedCurrency=params.currency;if(params?.month)selectedMonth=params.month;if(params?.offerReminders)offerReminders=true;return load();},onLanguage(){render();}
});
