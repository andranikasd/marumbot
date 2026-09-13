"use strict";
import {register} from '../nav.js';
import {getJSON} from '../api.js';
import {addStrings,T,sub} from '../i18n.js';
import {fmtMoney,fmtFull,esc} from '../core.js';
import {icon} from '../icons.js';

addStrings({
 'tab.home':'Գլխավոր','home.next':'Հաջորդ պարտադիր վճարումը','home.empty':'Ավելացրեք ձեր առաջին վարկը',
 'home.budget':'Վարկերի համար գումար','home.plan':'Մարման պլան','home.loans':'Իմ վարկերը',
 'home.none':'Ստուգեք վարկի տվյալները','home.none.help':'Հաջորդ վճարումը դեռ հնարավոր չէ ցույց տալ։ Բացեք վարկը և ստուգեք բանկի տվյալները։',
 'home.settled':'Ձեր վարկերն ամբողջությամբ մարված են','home.review':'Հաստատել բանկի մնացորդը',
 'home.review.help':'Վճարումը պահպանված է։ Բացեք վճարումների պատմությունը, ստուգեք՝ բանկը մշակե՞լ է այն, ապա հաստատեք թարմ մնացորդը։',
 'home.start':'Սկսենք երկու պարզ քայլից','home.start.help':'Պատրաստեք բանկի հավելվածը․ այնտեղից պետք կգան վարկի տվյալները։',
 'home.step.loan':'1. Ավելացրեք վարկը','home.step.loan.help':'Նշեք բանկի ցույց տված մնացորդը և վճարման պայմանները։',
 'home.step.money':'2. Նշեք վարկերի համար գումարը','home.step.money.help':'Որքա՞ն կարող եք հատկացնել և ե՞րբ է գումարը հասանելի։',
 'home.saved':'Պահպանված է','home.set.money':'Նշել հասանելի գումարը','home.money.failed':'Չհաջողվեց ստուգել հասանելի գումարի կարգավորումները։ Վարկերի տվյալները բեռնված են։',
 'home.view.loan':'Բացել վարկը','home.view.plan':'Տեսնել մարման պլանը','home.plan.help':'Պլանը կստուգի՝ արդյոք գումարը բավարար է և հասանելի է վճարման օրերին։',
 'home.date.passed':'Նշված վճարման օրն անցել է։ Եթե արդեն վճարել եք, բացեք վարկը և նշեք դա։',
 'balance.asof':'Մնացորդը՝ {d}','balance.undated':'Մնացորդի ամսաթիվը նշված չէ'
},{
 'tab.home':'Home','home.next':'Next required payment','home.empty':'Add your first loan',
 'home.budget':'Money for loans','home.plan':'Repayment plan','home.loans':'My loans',
 'home.none':'Check your loan details','home.none.help':'We cannot show the next payment yet. Open the loan and check its bank details.',
 'home.settled':'Your loans are fully repaid','home.review':'Confirm bank balance',
 'home.review.help':'Your payment is saved. Open payment history to check whether the bank has processed it, then confirm the updated balance.',
 'home.start':'Start with two simple steps','home.start.help':'Keep your bank app handy. You will need the loan details it shows.',
 'home.step.loan':'1. Add your loan','home.step.loan.help':'Enter the balance and payment terms shown by your bank.',
 'home.step.money':'2. Set your money for loans','home.step.money.help':'How much can you set aside, and when is it available?',
 'home.saved':'Saved','home.set.money':'Set available money','home.money.failed':'We could not check your money settings. Your loan details have loaded.',
 'home.view.loan':'View loan','home.view.plan':'See repayment plan','home.plan.help':'The plan will check whether your money covers payments on their due dates.',
 'home.date.passed':'This payment date has passed. If you already paid, open the loan and mark it there.',
 'balance.asof':'Balance as of {d}','balance.undated':'Balance date not supplied'
});

let loadVersion=0;
const button=(target,key,primary=false,arg='')=>`<button class="${primary?'cta':'alink'}" data-go="${target}"${arg?` data-arg="${esc(arg)}"`:''}>${esc(T(key))}</button>`;
function setup(loans,budget,failed){
 const hasLoans=loans.length>0,hasMoney=budget?.monthly_major!=null;
 if(hasLoans&&hasMoney)return '';
 if(failed)return `<div class="card stack"><p role="alert">${esc(T('home.money.failed'))}</p>${button('home','retry')}${hasLoans?'':button('add','home.empty',true)}</div>`;
 if(hasLoans&&!loans.some(l=>l.balance_major>0))return '';
 if(!budget)return '';
 return `<section class="card stack" aria-label="${esc(T('home.start'))}"><b>${esc(T('home.start'))}</b><p class="hint">${esc(T('home.start.help'))}</p>
 <div><b>${esc(T('home.step.loan'))}${hasLoans?` · ${esc(T('home.saved'))}`:''}</b>${hasLoans?'':`<p class="hint">${esc(T('home.step.loan.help'))}</p>${button('add','home.empty',true)}`}</div>
 <div><b>${esc(T('home.step.money'))}${hasMoney?` · ${esc(T('home.saved'))}`:''}</b>${hasMoney?'':`<p class="hint">${esc(T('home.step.money.help'))}</p>${button('budget-edit','home.set.money',hasLoans)}`}</div></section>`;
}
function render(root,d,budget,budgetFailed){
 const loans=d.loans,live=loans.filter(l=>l.balance_major>0);
 const review=loans.some(l=>l.needs_reconciliation);
 const next=live.filter(l=>!l.needs_reconciliation&&l.next_due&&l.next_payment_major!=null).sort((a,b)=>a.next_due.localeCompare(b.next_due))[0];
 const paidOff=loans.length>0&&!live.length&&!review;
 let nextCard='';
 if(next){
  nextCard=`<div class="card stack"><span>${esc(T('home.next'))}</span><strong>${esc(next.name)}</strong><div class="v num">${esc(fmtMoney(next.next_payment_major,next.currency))}</div><span>${esc(fmtFull(next.next_due))}</span>${d.today&&next.next_due<d.today?`<p class="hint">${esc(T('home.date.passed'))}</p>`:''}${button('loan','home.view.loan',true,next.id)}</div>`;
 }else if(loans.length&&!review){
  nextCard=`<div class="card stack"><b>${esc(T(paidOff?'home.settled':'home.none'))}</b>${paidOff?'':`<p class="hint">${esc(T('home.none.help'))}</p>`}${button('loans','home.loans',true)}</div>`;
 }
 const reviewCard=review?`<div class="card stack"><b>${esc(T('home.review'))}</b><p class="hint">${esc(T('home.review.help'))}</p>${button('activity','home.review',true)}</div>`:'';
 const ready=live.length>0&&!review&&budget?.monthly_major!=null;
 root.innerHTML=`${reviewCard}${setup(loans,budget,budgetFailed)}${nextCard}${ready?`<div class="card stack"><p class="hint">${esc(T('home.plan.help'))}</p>${button('plan','home.view.plan',true)}</div>`:''}<div class="pair"><button class="card shortcut" data-go="budget">${icon('wallet')}<span>${esc(T('home.budget'))}</span></button><button class="card shortcut" data-go="plan">${icon('document')}<span>${esc(T('home.plan'))}</span></button></div>${live.length?`<div class="card stack"><b>${esc(T('home.loans'))}</b>${live.slice(0,3).map(l=>`<button class="home-loan" data-go="loan" data-arg="${esc(l.id)}">${icon(l.icon)}<span>${esc(l.name)}</span><b>${esc(fmtMoney(l.balance_major,l.currency))} <small class="mute" style="display:block;font-weight:400;white-space:normal">${esc(l.balance_as_of?sub('balance.asof',{d:fmtFull(l.balance_as_of)}):T('balance.undated'))}</small></b></button>`).join('')}${button('loans','home.loans')}</div>`:''}`;
}
register({id:'home',icon:icon('home'),labelKey:'tab.home',titleKey:'tab.home',html:'<div class="stack" id="home-content"></div>',async onShow(){
 const version=++loadVersion,root=document.getElementById('home-content');
 let loans=null,budget=null,budgetFailed=false;
 const update=()=>{if(version===loadVersion&&loans)render(root,loans,budget,budgetFailed);};
 root.innerHTML=`<div class="state" role="status">${esc(T('loading'))}</div>`;
 root.setAttribute('aria-busy','true');
 await Promise.allSettled([
  getJSON('api/loans',d=>{
   if(!d||!Array.isArray(d.loans))throw new Error('Invalid loan response');
   loans=d;update();
  }).catch(()=>{if(version===loadVersion)root.innerHTML=`<div class="state"><p role="alert">${esc(T('err.load'))}</p>${button('home','retry')}</div>`;}),
  getJSON('api/budget',d=>{
   if(!d||typeof d!=='object'||Array.isArray(d))throw new Error('Invalid budget response');
   budget=d;budgetFailed=false;update();
  }).catch(()=>{budgetFailed=true;update();})
 ]);
 if(version===loadVersion)root.removeAttribute('aria-busy');
}});
