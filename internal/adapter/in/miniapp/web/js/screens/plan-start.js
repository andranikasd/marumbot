"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T,sub} from '../i18n.js';
import {toast,haptic} from '../core.js';

addStrings({
 'ps.title':'Պլանի մեկնարկը','ps.hint':'Եթե այս ամսվա պարտադիր վճարումներն արդեն կատարել եք, գրանցեք դրանք վարկերի բաժնում, ապա պլանը սկսեք հաջորդ ամսից։ Սա չի փոխում վարկերի մնացորդները և վճարում չի գրանցում։',
 'ps.when':'Երբ սկսել հաշվարկները','ps.today':'Այսօրվանից','ps.next':'Հաջորդ ամսից ({month})','ps.save':'Պահպանել մեկնարկը',
 'ps.budget':'Նախ ավելացրեք ձեր բյուջեն։','ps.reload':'Վերբեռնել՝ փոփոխությունները հեռացնելով',
 'ps.conflict':'Բյուջեն փոխվել է։ Վերբեռնեք վերջին տարբերակը և նորից ընտրեք մեկնարկը։',
 'ps.unpaid':'Մինչև ընտրված մեկնարկը կան պարտադիր վճարումներ։ Վարկերի բաժնում գրանցեք կատարված վճարումները կամ ընտրեք այսօրվանից։',
 'ps.rejected':'Չհաջողվեց պահպանել այս ընտրությունը։ Ստուգեք բյուջեն և մեկնարկը։',
 'ps.review':'Նախ համադրեք գրանցված վճարումները բանկի տվյալների և բյուջեի հետ։',
 'ps.invalidLoan':'Վարկի տվյալներով ժամանակացույց կազմել հնարավոր չէ։ Ստուգեք բանկի մնացորդը, վճարումը և պայմանները վարկերի բաժնում։',
 'ps.uncertain':'Պահպանման արդյունքը հայտնի չէ։ Կրկնեք նույն պահպանումը՝ նախքան փոփոխելը։'
},{
 'ps.title':'Plan start','ps.hint':'If you have already made this month’s required payments, record them on your loans, then start planning next month. This does not change loan balances or record a payment.',
 'ps.when':'When should calculations start?','ps.today':'From today','ps.next':'Next month ({month})','ps.save':'Save start date',
 'ps.budget':'Add your budget first.','ps.reload':'Reload and discard changes',
 'ps.conflict':'Your budget changed elsewhere. Reload its latest version and choose the start again.',
 'ps.unpaid':'Required payments remain before this start date. Record completed payments on your loans, or choose today.',
 'ps.rejected':'This choice could not be saved. Check your budget and start date.',
 'ps.review':'Reconcile recorded payments with your bank figures and budget first.',
 'ps.invalidLoan':'The loan details do not produce a valid schedule. Check the bank balance, payment and terms on your loans.',
 'ps.uncertain':'The save outcome is unknown. Retry the same save before changing your choice.'
});
const $=id=>document.getElementById(id);
let version=null,loaded=false,loading=false,busy=false,dirty=false,conflict=false,pending=null,request=0;
function controls(){
 $('ps-fields').disabled=!loaded||loading||busy||!!pending||conflict;
 $('ps-retry').hidden=!pending;$('ps-retry').disabled=busy;
 $('ps-reload').hidden=!!pending||busy||loading||(!conflict&&loaded);
}
async function load(discard=false){
 if(pending||busy||loading||(dirty&&!discard)){controls();return;}
 loading=true;loaded=false;$('ps-review').hidden=true;controls();$('ps-status').textContent=T('loading');
 const revision=++request;
 try{
  const res=await api('api/budget',{cache:'no-store'});if(!res.ok)throw new Error('load');
  const b=await res.json();
  if(revision!==request)return;
  if(!Number.isSafeInteger(b.version)||b.version<0||!/^\d{4}-(0[1-9]|1[0-2])-\d{2}$/.test(b.today))throw new Error('document');
  if(b.monthly_major==null||!b.funding){$('ps-status').textContent=T('ps.budget');$('ps-budget').hidden=false;return;}
  const [year,month]=b.today.split('-').map(Number);
  const next=`${year+(month===12?1:0)}-${String(month===12?1:month+1).padStart(2,'0')}`;
  $('ps-next').value=next;$('ps-next').textContent=sub('ps.next',{month:next});
  const saved=b.funding.planning_start_month||'';
  if(saved&&saved>b.today.slice(0,7)&&saved!==next)throw new Error('start');
  $('ps-month').value=saved===next?next:'';
  version=b.version;loaded=true;dirty=false;conflict=false;
  $('ps-budget').hidden=true;$('ps-status').textContent='';
 }catch{if(revision===request)$('ps-status').textContent=T('err.load');}
 finally{if(revision===request){loading=false;controls();}}
}
async function save(event){
 event?.preventDefault();if(busy||loading||!loaded||conflict)return;
 if(!pending)pending=JSON.stringify({expected_version:version,idempotency_key:crypto.randomUUID(),planning_start_month:$('ps-month').value});
 busy=true;$('ps-review').hidden=true;controls();$('ps-status').textContent='';
 try{
  const res=await api('api/budget/planning-start',{method:'POST',body:pending});
  if(!res.ok){
   if(res.status>=400&&res.status<500){
    pending=null;dirty=true;conflict=res.status===409;
    const error=await res.json().catch(()=>({}));
    const review=error.error==='payment_reconciliation_required';
    $('ps-review').hidden=!review;
    $('ps-status').textContent=T(conflict?'ps.conflict':review?'ps.review':error.error==='loan_schedule_invalid'?'ps.invalidLoan':error.reason==='required payments remain before planning start'?'ps.unpaid':'ps.rejected');
    haptic.bad();return;
   }
   throw new Error('uncertain');
  }
  pending=null;dirty=false;loaded=false;invalidate('api/');toast(T('saved'));haptic.ok();
  if(currentScreen()==='plan-start')go('plan');
 }catch{$('ps-status').textContent=T('ps.uncertain');haptic.bad();}
 finally{busy=false;controls();}
}
register({id:'plan-start',parent:'plan',titleKey:'ps.title',html:`
 <div class="stack"><p class="hint" data-i18n="ps.hint"></p>
 <form id="ps-form" class="stack"><p id="ps-status" class="error" role="alert"></p>
 <fieldset id="ps-fields" class="card stack" style="border:0;min-width:0" disabled>
 <label for="ps-month" data-i18n="ps.when"></label><select id="ps-month"><option value="" data-i18n="ps.today"></option><option id="ps-next"></option></select>
 <button type="submit" class="cta" data-i18n="ps.save"></button></fieldset></form>
 <button id="ps-retry" type="button" class="cta" data-i18n="retry" hidden></button>
 <button id="ps-reload" type="button" class="alink" data-i18n="ps.reload" hidden></button>
 <button id="ps-budget" type="button" class="cta" data-go="budget-edit" data-i18n="more.budget" hidden></button>
 <button id="ps-review" type="button" class="alink" data-go="activity" data-i18n="payment.review" hidden></button>
 <button type="button" class="alink" data-go="loans" data-i18n="tab.loans"></button></div>`,
 onMount(){
  $('ps-form').addEventListener('submit',save);$('ps-month').addEventListener('change',()=>{dirty=true;});
  $('ps-retry').onclick=save;$('ps-reload').onclick=()=>load(true);
 },onShow(){return load();},onLanguage(){const month=$('ps-next').value;if(month)$('ps-next').textContent=sub('ps.next',{month});}
});
