"use strict";
import {register} from '../nav.js';
import {getJSON} from '../api.js';
import {addStrings,T,sub} from '../i18n.js';
import {fmtMoney,fmtFull,esc} from '../core.js';
import {icon} from '../icons.js';
addStrings({'tab.home':'Գլխավոր','home.next':'Հաջորդ պարտադիր վճարումը','home.empty':'Ավելացրեք ձեր առաջին վարկը','home.budget':'Բյուջե','home.plan':'Պլան','home.loans':'Իմ վարկերը','home.none':'Առաջիկա վճարում չկա'},{'tab.home':'Home','home.next':'Next required payment','home.empty':'Add your first loan','home.budget':'Budget','home.plan':'Plan','home.loans':'My loans','home.none':'No upcoming payment'});
addStrings({'balance.asof':'Մնացորդը՝ {d}','balance.undated':'Մնացորդի ամսաթիվը նշված չէ'},{'balance.asof':'Balance as of {d}','balance.undated':'Balance date not supplied'});
addStrings({'home.settled':'Ձեր վարկերն ամբողջությամբ մարված են','home.review':'Ստուգել վճարումները'},{'home.settled':'Your loans are fully repaid','home.review':'Review payments'});
let loadVersion=0;
register({id:'home',icon:icon('home'),labelKey:'tab.home',titleKey:'tab.home',html:'<div class="stack" id="home-content"></div>',async onShow(){
 const version=++loadVersion,root=document.getElementById('home-content');
 root.innerHTML=`<div class="state" role="status">${esc(T('loading'))}</div>`;
 root.setAttribute('aria-busy','true');
 try{await getJSON('api/loans',d=>{
  if(version!==loadVersion)return;
  const loans=d.loans||[],review=loans.some(l=>l.needs_reconciliation);
  const live=loans.filter(l=>l.balance_major>0), next=[...live].filter(l=>l.next_due).sort((a,b)=>a.next_due.localeCompare(b.next_due))[0];
  const emptyKey=review?'payment.review':live.length?'home.none':loans.length?'home.settled':'home.empty';
  const action=review?'activity':loans.length?'loans':'add';
  const actionKey=review?'home.review':loans.length?'tab.loans':'manage.add';
  root.innerHTML=`${(d.loans||[]).some(l=>l.needs_reconciliation)?`<button class="card" data-go="activity">${esc(T("payment.review"))}</button>`:""}<div class="card stack"><span>${esc(T('home.next'))}</span>${next?`<strong>${esc(next.name)}</strong><div class="v num">${esc(fmtMoney(next.next_payment_major,next.currency))}</div><span>${esc(fmtFull(next.next_due))}</span><button class="cta" data-go="loan" data-arg="${esc(next.id)}">${esc(T('loan.update'))}</button>`:`<p>${esc(T(emptyKey))}</p><button class="cta" data-go="${action}">${esc(T(actionKey))}</button>`}</div><div class="pair"><button class="card shortcut" data-go="budget">${icon('wallet')}<span>${esc(T('home.budget'))}</span></button><button class="card shortcut" data-go="plan">${icon('document')}<span>${esc(T('home.plan'))}</span></button></div><div class="card stack"><b>${esc(T('home.loans'))}</b>${live.slice(0,3).map(l=>`<button class="home-loan" data-go="loan" data-arg="${esc(l.id)}">${icon(l.icon)}<span>${esc(l.name)}</span><b>${esc(fmtMoney(l.balance_major,l.currency))} <small class="mute" style="display:block;font-weight:400;white-space:normal">${esc(l.balance_as_of?sub('balance.asof',{d:fmtFull(l.balance_as_of)}):T('balance.undated'))}</small></b></button>`).join('')}<button class="alink" data-go="loans">${esc(T('tab.loans'))}</button></div>`;
 });}catch{if(version===loadVersion)root.innerHTML=`<div class="state"><p role="alert">${esc(T('err.load'))}</p><button class="alink" data-go="home">${esc(T('retry'))}</button></div>`;}finally{if(version===loadVersion)root.removeAttribute('aria-busy');}
}});
