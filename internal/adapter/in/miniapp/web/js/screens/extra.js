"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T,sub} from '../i18n.js';
import {esc,fmtMoney,fmtMonth,haptic} from '../core.js';
import {extraMinor,extraText,validProjection} from './projection-format.js';
addStrings({
 'extra.title':'Հավելյալ վճարումներ','extra.question':'Որքա՞ն կարող եք ավելացնել','extra.hint':'Պարտադիր վճարումներից բացի։ Զրոն էլ է ընդունելի։',
 'extra.required':'Պարտադիր՝ {month}','extra.amount':'Հավելյալ՝ ամեն ամիս','extra.month':'Հավելյալ՝ {month}','extra.total':'Ընդամենը',
 'extra.show':'Ցույց տալ իմ պլանը','extra.save':'Պահպանել այս ամիսը','extra.reset':'Օգտագործել սովորական գումարը',
 'extra.override':'Այս փոփոխությունը միայն ընտրված ամսվա համար է։','extra.default':'Հաջորդ ամիսներին ձեր սովորական գումարը կմնա։',
 'extra.switch':'Այս պարզ պլանը հիմնված է պարտադիր և հավելյալ ամսական վճարումների վրա։ Նախկին բյուջեն և պատմությունը կպահպանվեն։',
 'extra.review':'Ստուգեք գումարները, ապա շարունակեք։ Նախկին բյուջեն հավելյալ գումար չենք համարում։',
 'extra.invalid':'Մուտքագրեք զրո կամ դրական գումար՝ արժույթի թույլատրելի ճշտությամբ։',
 'extra.uncertain':'Պահպանման արդյունքը հայտնի չէ։ Կրկնեք նույն պահպանումը։',
 'extra.conflict':'Կարգավորումները փոխվել են այլ տեղում։ Վերբեռնեք և ստուգեք գումարները։',
 'extra.reload':'Վերբեռնել՝ հեռացնելով այս փոփոխությունները','extra.empty':'Նախ ավելացրեք առնվազն մեկ վարկ։',
 'extra.settled':'Ակտիվ պարտք չունեք։ Նոր վարկ ավելացնելիս կարող եք ընտրել հավելյալ գումարը։',
 'extra.add':'Ավելացնել վարկ','extra.help':'Չե՞ք կարող վճարել պարտադիրը։',
 'extra.help.text':'Հավելյալը նշեք 0։ Սա չի նվազեցնում բանկի պահանջած վճարումը։ Բացեք վարկը՝ չվճարված գումարը ստուգելու համար, և բանկի հետ քննարկեք տարբերակները։',
 'extra.partial':'Առաջին պլանավորված ամսվա համար նշեք այն հավելյալը, որը դեռ պատրաստվում եք վճարել։',
 'extra.unknown':'Պարտադիր գումարը դեռ ամբողջական չէ։ Նախ ստուգեք վարկերի բանկային տվյալները։',
 'extra.loans':'Ստուգել վարկերը','extra.known':'Հայտնի պարտադիր վճարումները','extra.cover':'Որքա՞ն կարող եք վճարել պարտադիրի համար ({currency})','extra.gap':'Պակասում է՝ {amount}','extra.covered':'Նշված պարտադիր գումարը բավարարում եք։','extra.gap.note':'Սա միայն համեմատություն է։ Այն չի փոխում ձեր պլանը կամ բանկի պահանջը։'
},{
 'extra.title':'Extra payments','extra.question':'How much can you add?','extra.hint':'On top of required payments. Zero is fine.',
 'extra.required':'Required · {month}','extra.amount':'Extra each month','extra.month':'Extra · {month}','extra.total':'Total',
 'extra.show':'Show my plan','extra.save':'Save this month','extra.reset':'Use usual amount',
 'extra.override':'This change is for the selected month only.','extra.default':'Your usual amount stays the same for other months.',
 'extra.switch':'This simpler plan uses required payments plus a monthly extra. Your previous budget and history stay saved.',
 'extra.review':'Review the amounts, then continue. Your old budget is not treated as extra.',
 'extra.invalid':'Enter zero or a positive amount with the currency’s supported precision.',
 'extra.uncertain':'The save outcome is unknown. Retry the same save.',
 'extra.conflict':'Settings changed elsewhere. Reload and review the amounts.',
 'extra.reload':'Reload and discard these changes','extra.empty':'Add at least one loan first.',
 'extra.settled':'You have no active debt. You can choose an extra amount when you add a loan.',
 'extra.add':'Add a loan','extra.help':'Cannot cover the required amount?',
 'extra.help.text':'Set extra to 0. This does not reduce what your bank requires. Open the loan to check what is still unpaid, and discuss options with your bank.',
 'extra.partial':'For the first planned month, extra means the amount you still intend to pay.',
 'extra.unknown':'The required amount is not complete yet. Check your loans’ bank details first.',
 'extra.loans':'Check loans','extra.known':'Known required payments','extra.cover':'What can you pay toward required payments ({currency})?','extra.gap':'Shortfall: {amount}','extra.covered':'You cover the required amount shown.','extra.gap.note':'This is a comparison only. It does not change your plan or what the bank requires.'
});
const $=id=>document.getElementById(id);
let settings=null,projection=null,drafts={},month='',currency='',dirty=false,busy=false,pending=null,conflict=false,loaded=false,generation=0;
const draftContexts=new Map(),coverage={};
const contextKey=()=>`${month}:${currency}`;
function groups(){return (projection?.currencies||[]).filter(c=>!currency||c.currency===currency);}
function row(c){return c.months.find(m=>m.month===(month||c.start_month))||(month?null:c.months[0]);}
function controls(){
 $('extra-fields').disabled=!loaded||busy||!!pending||conflict;
 $('extra-retry').hidden=!pending;$('extra-retry').disabled=busy;
 $('extra-reload').hidden=!!pending||busy||(!conflict&&loaded);
}
function totals(){
 for(const c of groups()){
  const n=extraMinor(drafts[c.currency],c.exponent),required=row(c)?.required_minor;
  const total=$(`extra-total-${c.currency}`);
  if(total)total.textContent=n!=null&&Number.isSafeInteger(required)&&Number.isSafeInteger(required+n)?fmtMoney((required+n)/10**c.exponent,c.currency):'—';
 }
}
function shortfalls(){
 for(const c of groups()){
  const n=extraMinor(coverage[c.currency]??'',c.exponent),required=row(c)?.required_minor;
  const output=$(`extra-gap-${c.currency}`);if(!output)continue;
  output.textContent=n==null||!Number.isSafeInteger(required)?'':n<required?sub('extra.gap',{amount:fmtMoney((required-n)/10**c.exponent,c.currency)}):T('extra.covered');
 }
}
function render(){
 if(!projection)return;
 const active=groups();
 $('extra-intro').textContent=T(month?'extra.override':'extra.hint');
 $('extra-consent').hidden=settings.enabled;
 $('extra-content').innerHTML=active.map(c=>{
  const m=row(c),name=`extra-input-${c.currency}`;
  return `<section class="card stack"><b>${esc(c.currency)}</b><div class="spread"><span>${esc(c.complete?sub('extra.required',{month:fmtMonth((month||c.start_month)+'-01')}):T('extra.known'))}</span><strong class="num">${m?esc(fmtMoney(m.required_minor/10**c.exponent,c.currency)):'—'}</strong></div>
  <label for="${name}">${esc(month?sub('extra.month',{month:fmtMonth(month+'-01')}):T('extra.amount'))}</label><input id="${name}" data-currency="${esc(c.currency)}" inputmode="decimal" autocomplete="off" type="text" value="${esc(drafts[c.currency]??'0')}">
  <div class="spread"><span>${esc(T('extra.total'))}</span><strong id="extra-total-${c.currency}" class="num"></strong></div></section>`;
 }).join('');
 $('extra-submit').textContent=T(month?'extra.save':'extra.show');$('extra-reset').hidden=!month||!active.some(c=>Object.hasOwn(settings.currencies[c.currency]?.overrides||{},month));
 $('extra-note').textContent=T(month?'extra.default':'extra.partial');
 if(!active.length){$('extra-content').innerHTML=`<p>${esc(T('extra.settled'))}</p><button type="button" class="cta" data-go="loan-setup">${esc(T('extra.add'))}</button>`;loaded=false;}
 $('extra-coverage').innerHTML=active.map(c=>`<label for="extra-cover-${c.currency}">${esc(sub('extra.cover',{currency:c.currency}))}</label><input id="extra-cover-${c.currency}" data-cover="${c.currency}" type="text" inputmode="decimal" value="${esc(coverage[c.currency]||'')}"><p id="extra-gap-${c.currency}" role="status"></p>`).join('');
 totals();shortfalls();controls();
}
async function load(discard=false){
 if(busy||pending||(loaded&&dirty&&!discard)){render();return;}
 const revision=++generation;loaded=false;conflict=false;$('extra-status').textContent=T('loading');controls();
 try{
  const [a,b]=await Promise.all([api('api/projection/settings',{cache:'no-store'}),api('api/projection',{cache:'no-store'})]);
  if(!a.ok||!b.ok)throw new Error('load');
  const [s,p]=await Promise.all([a.json(),b.json()]);if(revision!==generation)return;
  if(!Number.isSafeInteger(s.version)||s.version<0||!s.currencies||!validProjection(p))throw new Error('document');
  if(s.version!==p.settings_version)throw new Error('changed');
  settings=s;projection=p;drafts={};dirty=false;loaded=true;
  for(const c of groups()){
   const saved=s.currencies[c.currency];drafts[c.currency]=extraText(month&&Object.hasOwn(saved?.overrides||{},month)?saved.overrides[month]:saved?.extra_minor||0,c.exponent);
  }
  const retained=!discard&&draftContexts.get(contextKey());
  if(retained){drafts={...drafts,...retained};dirty=true;}
  if(discard)draftContexts.delete(contextKey());
  $('extra-status').textContent='';render();
 }catch{if(revision===generation){$('extra-status').textContent=T('err.load');controls();}}
}
async function save(reset=false){
 if(busy||(!pending&&(!loaded||conflict)))return;
 if(!pending){
  const currencies=structuredClone(settings.currencies);
  for(const c of groups()){
   const amount=extraMinor(drafts[c.currency],c.exponent);
   if(!reset&&amount===null){$('extra-status').textContent=T('extra.invalid');$(`extra-input-${c.currency}`).focus();return;}
   const source=currencies[c.currency]||{extra_minor:0,overrides:{}};source.overrides??={};
   if(month){if(reset)delete source.overrides[month];else source.overrides[month]=amount;}else source.extra_minor=amount;
   currencies[c.currency]=source;
  }
  pending=JSON.stringify({enabled:true,expected_version:settings.version,idempotency_key:crypto.randomUUID(),currencies});
 }
 busy=true;controls();$('extra-status').textContent=T('saving');
 try{
  const res=await api('api/projection/settings',{method:'POST',body:pending});
  if(!res.ok){
   if(res.status>=400&&res.status<500){pending=null;conflict=res.status===409;$('extra-status').textContent=T(conflict?'extra.conflict':'err.save');return;}
   throw new Error('uncertain');
  }
  pending=null;dirty=false;loaded=false;draftContexts.delete(contextKey());invalidate('api/');haptic.ok();$('extra-status').textContent='';
  if(currentScreen()==='extra')go('simple-plan',{currency,month,offerReminders:!settings.enabled});
 }catch{$('extra-status').textContent=T('extra.uncertain');haptic.bad();}
 finally{busy=false;controls();}
}
register({id:'extra',parent:'simple-plan',titleKey:'extra.title',html:`<div class="stack">
 <h2 data-i18n="extra.question"></h2><p id="extra-intro" class="hint"></p>
 <div id="extra-consent" class="card stack" hidden><p data-i18n="extra.switch"></p><p class="hint" data-i18n="extra.review"></p></div>
 <p id="extra-status" class="error" role="alert"></p>
 <form id="extra-form"><fieldset id="extra-fields" class="stack" style="border:0;padding:0;min-width:0" disabled>
 <div id="extra-content" class="stack"></div><p id="extra-note" class="hint"></p>
 <button id="extra-submit" class="cta" type="submit"></button><button id="extra-reset" class="alink" type="button" data-i18n="extra.reset" hidden></button></fieldset></form>
 <button id="extra-retry" class="cta" data-i18n="retry" hidden></button><button id="extra-reload" class="alink" data-i18n="extra.reload" hidden></button>
 <details class="card"><summary data-i18n="extra.help"></summary><p class="hint" data-i18n="extra.help.text"></p><div id="extra-coverage" class="stack"></div><p class="hint" data-i18n="extra.gap.note"></p><button class="alink" data-go="loans" data-i18n="extra.loans"></button></details></div>`,
 onMount(){
  $('extra-form').addEventListener('submit',e=>{e.preventDefault();return save();});
  $('extra-content').addEventListener('input',e=>{if(e.target.dataset.currency){drafts[e.target.dataset.currency]=e.target.value;dirty=true;draftContexts.set(contextKey(),{...drafts});totals();}});
  $('extra-coverage').addEventListener('input',e=>{if(e.target.dataset.cover){coverage[e.target.dataset.cover]=e.target.value;shortfalls();}});
  $('extra-reset').onclick=()=>save(true);$('extra-retry').onclick=()=>save();$('extra-reload').onclick=()=>load(true);
 },onShow(_root,params){
  const nextMonth=/^\d{4}-(0[1-9]|1[0-2])$/.test(params?.month||'')?params.month:'';
  const nextCurrency=/^[A-Z]{3}$/.test(params?.currency||'')?params.currency:'';
  if(pending||busy){render();return;}
  if(month!==nextMonth||currency!==nextCurrency){month=nextMonth;currency=nextCurrency;loaded=false;dirty=false;}
  return load();
 },onLanguage(){render();}
});
