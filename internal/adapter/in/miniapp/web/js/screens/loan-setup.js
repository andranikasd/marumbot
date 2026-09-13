"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api,invalidate} from '../api.js';
import {addStrings,T,sub} from '../i18n.js';
import {num,fmtDate,haptic} from '../core.js';
import {loanMutation,showLoanRetry} from '../loan-mutations.js';
import {extraMinor,extraText} from './projection-format.js';

addStrings({
 'ls.accrued':'Այսօրվա կուտակված տոկոսը (ըստ ցանկության)','ls.accruedHint':'Միայն եթե բանկը ցույց է տալիս առանձին։ Դատարկը նշանակում է՝ չգիտեմ, 0-ը՝ տոկոս չկա։',
 'ls.title':'Ավելացնել վարկ','ls.step':'Քայլ {step} / 4','ls.amounts':'Որքա՞ն եք պարտք',
 'ls.original':'Վարկի սկզբնական գումարը','ls.remaining':'Մնացած մայր գումարը','ls.bank':'Գրեք բանկի հավելվածում նշված մնացորդը՝ կատարված վճարումներից հետո։',
 'ls.name':'Անվանում (ըստ ցանկության)','ls.default':'Իմ վարկը','ls.currency':'Արժույթ','ls.dates':'Վարկի ժամկետը','ls.start':'Սկսվել է','ls.end':'Ավարտվում է',
 'ls.payment':'Հաջորդ վճարումը','ls.monthly':'Բանկի պահանջած ամսական վճարը','ls.nextDue':'Առաջին չվճարված վճարման օրը',
 'ls.paid':'Այս ամիսն արդեն վճարե՞լ եք։ Ընտրեք հաջորդ չվճարված վճարման օրը։ Մնացորդից կրկին գումար չի հանվի։',
 'ls.details':'Տոկոսադրույք և բանկի կանոններ (ըստ ցանկության)','ls.rate':'Տարեկան տոկոսադրույքը (%)','ls.unknown':'Չգիտե՞ք։ Թողեք դատարկ։','ls.method':'Վճարումների տեսակը','ls.fixed':'Հավասար վճարներ','ls.declining':'Նվազող վճարներ',
 'ls.terms':'Բանկս հավելյալ վճարն ուղղում է մայր գումարին, չի փոխում ամսական վճարը և վաղաժամկետ վճարման վճար չի գանձում։',
 'ls.review':'Ստուգեք վարկը','ls.confirm':'Այս մնացորդն արդեն ներառում է իմ կատարած բոլոր վճարումները։','ls.save':'Պահպանել վարկը','ls.next':'Հաջորդը','ls.back':'Նախորդը',
 'ls.invalid':'Ստուգեք գումարները և ամսաթվերը։ Մնացորդը չի կարող գերազանցել սկզբնական գումարը։','ls.missing':'Լրացրեք այս քայլի տվյալները։','ls.limited':'Առանց տոկոսադրույքի և բանկի կանոնների՝ ցույց կտանք վճարումները, բայց չենք խոստանա մարման օր կամ խնայողություն։',
 'ls.overdue':'Այս վճարումը ժամկետանց է։ Կպահենք նշված օրը։ Բանկից ճշտեք պարտքն ու տույժերը․ մինչև այդ մարման կանխատեսում չենք ցուցադրի։','ls.future':'Ավելացրեք վարկը, երբ այն սկսվի և բանկը ցույց տա ընթացիկ մնացորդը։','ls.larger':'Մնացորդը մեծ է սկզբնական գումարից։ Բանկից ճշտեք մայր գումարը՝ տոկոսներն ու տույժերը չներառելով։',
 'ls.repaid':'Այս վարկն ամբողջությամբ մարված է։','ls.confirmNeeded':'Հաստատեք բանկի մնացորդը։','ls.saveError':'Չհաջողվեց պահպանել։ Ստուգեք տվյալները և կրկին փորձեք։','ls.uncertain':'Պահպանումը դեռ հաստատված չէ։ Ստուգեք նույն պահպանումը՝ կրկնօրինակ չստեղծելու համար։'
},{
 'ls.accrued':'Interest accrued today (optional)','ls.accruedHint':'Only if your bank shows it separately. Blank means unknown; 0 means none.',
 'ls.title':'Add a loan','ls.step':'Step {step} of 4','ls.amounts':'How much do you owe?',
 'ls.original':'Original loan amount','ls.remaining':'Remaining principal','ls.bank':'Use the balance in your bank app, after any payments you have already made.',
 'ls.name':'Name (optional)','ls.default':'My loan','ls.currency':'Currency','ls.dates':'Loan dates','ls.start':'Started on','ls.end':'Ends on',
 'ls.payment':'Next payment','ls.monthly':'Monthly payment required by your bank','ls.nextDue':'First unpaid payment date',
 'ls.paid':'Already paid this month? Choose your next unpaid payment date. We will not subtract the payment again.',
 'ls.details':'Interest and bank rules (optional)','ls.rate':'Annual interest rate (%)','ls.unknown':'Not sure? Leave blank.','ls.method':'Payment type','ls.fixed':'Equal monthly payments','ls.declining':'Decreasing payments',
 'ls.terms':'My bank applies extra payments directly to principal, keeps the monthly payment unchanged, and charges no early-payment fee.',
 'ls.review':'Check your loan','ls.confirm':'This balance already includes all payments I have made.','ls.save':'Save loan','ls.next':'Next','ls.back':'Back',
 'ls.invalid':'Check the amounts and dates. Remaining principal cannot exceed the original amount.','ls.missing':'Complete the details in this step.','ls.limited':'Without the interest rate and bank rules, we can show payments but cannot promise a payoff date or savings.',
 'ls.overdue':'This payment is overdue. We will keep the date you entered. Check the balance and penalties with your bank; payoff forecasts stay paused.','ls.future':'Add this loan once it has started and your bank shows the current balance.','ls.larger':'The balance exceeds the original amount. Check the remaining principal with your bank, excluding interest and penalties.',
 'ls.repaid':'This loan is fully paid.','ls.confirmNeeded':'Confirm the bank balance before saving.','ls.saveError':'Could not save. Check the details and try again.','ls.uncertain':'Save is not confirmed yet. Check the same save to avoid adding a duplicate.'
});
const field=(id,key,type='text',extra='')=>`<div class="field"><label for="ls-${id}" data-i18n="${key}"></label><input id="ls-${id}" type="${type}" ${extra}></div>`;
const html=`<form id="ls-form" class="stack" novalidate>
 <p id="ls-progress" class="hint" aria-live="polite"></p>
 <fieldset id="ls-fields" class="stack" style="border:0;padding:0;margin:0;min-width:0" disabled>
 <div id="ls-step-0" class="card stack"><h2 data-i18n="ls.amounts"></h2>
 ${field('original','ls.original','text','inputmode="decimal" autocomplete="off"')}
 ${field('remaining','ls.remaining','text','inputmode="decimal" autocomplete="off"')}
 <p class="hint" data-i18n="ls.bank"></p>
 <div class="field"><label for="ls-currency" data-i18n="ls.currency"></label><select id="ls-currency"><option>AMD</option><option>USD</option><option>EUR</option><option>RUB</option></select></div>
 ${field('name','ls.name','text','maxlength="60" autocomplete="off"')}
 </div>
 <div id="ls-step-1" class="card stack" hidden><h2 data-i18n="ls.dates"></h2>${field('start','ls.start','date')}${field('end','ls.end','date')}</div>
 <div id="ls-step-2" class="card stack" hidden><h2 data-i18n="ls.payment"></h2>
 <p id="ls-repaid" data-i18n="ls.repaid" hidden></p><div id="ls-payment-fields" class="stack">
 ${field('payment','ls.monthly','text','inputmode="decimal" autocomplete="off"')}${field('due','ls.nextDue','date')}
 <p class="hint" data-i18n="ls.paid"></p></div>
 <details class="fold"><summary data-i18n="ls.details"></summary><div class="fold-body stack">
 ${field('accrued','ls.accrued','text','inputmode="decimal" autocomplete="off"')}<p class="hint" data-i18n="ls.accruedHint"></p>${field('rate','ls.rate','text','inputmode="decimal" autocomplete="off"')}<p class="hint" data-i18n="ls.unknown"></p>
 <div class="field"><label for="ls-method" data-i18n="ls.method"></label><select id="ls-method"><option value="annuity" data-i18n="ls.fixed"></option><option value="declining" data-i18n="ls.declining"></option></select></div>
 <label><input id="ls-terms" type="checkbox"> <span data-i18n="ls.terms"></span></label>
 </div></details></div>
 <div id="ls-step-3" class="card stack" hidden><h2 data-i18n="ls.review"></h2><div id="ls-summary" class="kv"></div><p id="ls-limited" class="hint" data-i18n="ls.limited"></p><p id="ls-overdue" class="hint" data-i18n="ls.overdue" hidden></p><label><input id="ls-confirm" type="checkbox"> <span data-i18n="ls.confirm"></span></label></div>
 <div class="row"><button class="alink" type="button" id="ls-back" data-i18n="ls.back" hidden></button><button class="cta" id="ls-next" type="submit" data-i18n="ls.next"></button></div>
 </fieldset><p id="ls-error" class="error" role="alert"></p><button type="button" id="ls-retry" hidden></button><button type="button" id="ls-load" data-i18n="retry" hidden></button></form>`;
const $=id=>document.getElementById('ls-'+id);
let step=0,today='',loading=false,busy=false,uncertain=false;
// All four offered currencies use two minor-unit decimal places.
const minor=id=>extraMinor($(id).value,2);
const decimal=id=>{const value=minor(id);return value===null?null:extraText(value,2);};
function valid(part){
 const original=decimal('original'),remaining=decimal('remaining');
 if(part===0)return original!==null&&remaining!==null&&Number(original)>0&&Number(remaining)>=0&&minor('remaining')<=minor('original');
 if(part===1)return /^\d{4}-\d{2}-\d{2}$/.test($('start').value)&&$('start').value<=today&&$('end').value>$('start').value&&Number($('end').value.slice(0,4))-Number($('start').value.slice(0,4))<=40;
 if(part===2){if($('accrued').value.trim()&&(minor('accrued')===null||(minor('remaining')===0&&minor('accrued')>0)))return false;const rate=$('rate').value.trim();return (!rate||(Number.isFinite(num(rate))&&num(rate)>=0&&num(rate)<=200))&&(Number(remaining)===0||(decimal('payment')!==null&&Number(decimal('payment'))>0&&!!$('due').value&&$('due').value>$('start').value&&$('due').value<=$('end').value));}
 return $('confirm').checked;
}
function validationMessage(){
 if(step===3)return T('ls.confirmNeeded');
 if(step===0&&minor('remaining')>minor('original'))return T('ls.larger');
 if(step===1&&$('start').value>today)return T('ls.future');
 if(step===2&&$('due').value&&$('due').value<today)return T('ls.overdue');
 return T('ls.invalid');
}
function moneyLabel(id){const [whole,fraction]=decimal(id).split('.');return whole.replace(/\B(?=(\d{3})+(?!\d))/g,' ')+(fraction==='00'?'':'.'+fraction)+' '+$('currency').value;}
function summary(){
 const root=$('summary');root.replaceChildren();
 const row=(label,value)=>{const line=document.createElement('div'),key=document.createElement('span'),val=document.createElement('b');key.textContent=T(label);val.textContent=value;line.append(key,val);root.append(line);};
 if($('accrued').value.trim())row('ls.accrued',moneyLabel('accrued'));
 row('ls.name',$('name').value.trim()||T('ls.default'));
 row('ls.original',moneyLabel('original'));row('ls.remaining',moneyLabel('remaining'));
 row('ls.start',fmtDate($('start').value));row('ls.end',fmtDate($('end').value));
 if($('rate').value.trim()){row('ls.rate',String(num($('rate').value))+'%');row('ls.method',T($('method').value==='declining'?'ls.declining':'ls.fixed'));}
 if(Number(decimal('remaining'))>0){row('ls.monthly',moneyLabel('payment'));row('ls.nextDue',fmtDate($('due').value));}
 $('overdue').hidden=Number(decimal('remaining'))===0||!$('due').value||$('due').value>=today;
 $('limited').hidden=!!$('rate').value.trim()&&$('terms').checked&&$('method').value==='annuity';
}
function render(){
 for(let i=0;i<4;i++)$('step-'+i).hidden=i!==step;
 $('progress').textContent=sub('ls.step',{step:step+1});$('back').hidden=step===0;
 $('next').textContent=T(step===3?'ls.save':'ls.next');$('fields').disabled=!today||loading||busy||uncertain;
 const repaid=minor('remaining')===0;$('repaid').hidden=!repaid;$('payment-fields').hidden=repaid;
 if(step===3)summary();
 showLoanRetry($('retry'),'api/setup/loans',done);
 const retry=$('retry').onclick;
 $('retry').onclick=async()=>{if(busy)return;busy=true;try{await retry();}finally{busy=false;showLoanRetry($('retry'),'api/setup/loans',done);uncertain=!$('retry').hidden;if(!uncertain&&step===3)$('error').textContent=T('ls.saveError');render();}};
}
function done(){
 uncertain=false;busy=false;step=0;
 for(const id of ['original','remaining','name','start','end','payment','due','rate','accrued'])$(id).value='';
 $('confirm').checked=false;$('terms').checked=false;$('error').textContent='';
 invalidate('api/');haptic.ok();render();if(currentScreen()==='loan-setup')go('loans',{setup:true});
}
async function load(){
 if(today||loading)return;loading=true;render();$('load').hidden=true;$('error').textContent=T('loading');
 try{const res=await api('api/setup/loans',{cache:'no-store'});if(!res.ok)throw new Error();const data=await res.json();if(!/^\d{4}-\d{2}-\d{2}$/.test(data.today))throw new Error();today=data.today;$('start').max=today;$('error').textContent='';}
 catch{$('error').textContent=T('err.load');$('load').hidden=false;}finally{loading=false;render();}
}
async function submit(event){
 event.preventDefault();if(!today||loading||busy||uncertain)return;$('error').textContent='';
 if(!valid(step)){$('error').textContent=validationMessage();return;}
 if(step<3){step++;render();return;}
 for(let i=0;i<3;i++)if(!valid(i)){step=i;$('error').textContent=T('ls.invalid');render();return;}
 const zero=minor('remaining')===0;
 const body={accrued_interest_major:$('accrued').value.trim()?decimal('accrued'):null,title:$('name').value.trim()||T('ls.default'),currency:$('currency').value,principal_major:decimal('original'),remaining_major:decimal('remaining'),start_date:$('start').value,maturity_date:$('end').value,next_due_date:zero?'':$('due').value,payment_major:zero?'0':decimal('payment'),rate_percent:$('rate').value.trim()?String(num($('rate').value)):null,method:$('method').value,confirmed:true,projection_terms_confirmed:$('terms').checked};
 busy=true;render();
 try{const res=await loanMutation('api/setup/loans','POST',body);if(res.ok){done();return;}uncertain=res.status>=500;$('error').textContent=T(uncertain?'ls.uncertain':'ls.saveError');}
 catch{uncertain=true;$('error').textContent=T('ls.uncertain');}
 finally{busy=false;render();}
}
register({id:'loan-setup',parent:'loans',titleKey:'ls.title',html,onMount(){
 $('form').addEventListener('submit',submit);$('back').onclick=()=>{if(busy||uncertain)return;step=Math.max(0,step-1);$('error').textContent='';render();};$('load').onclick=load;
 },onShow(){render();return load();},onLanguage(){render();}});
