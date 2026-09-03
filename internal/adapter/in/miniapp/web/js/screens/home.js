"use strict";
import {register} from '../nav.js';
import {getJSON} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {fmtMoney,fmtFull,esc} from '../core.js';
import {icon} from '../icons.js';
addStrings({'tab.home':'Գլխավոր','home.next':'Հաջորդ պարտադիր վճարումը','home.empty':'Ավելացրեք ձեր առաջին վարկը','home.budget':'Բյուջե','home.plan':'Պլան','home.loans':'Իմ վարկերը','home.none':'Առաջիկա վճարում չկա'},{'tab.home':'Home','home.next':'Next required payment','home.empty':'Add your first loan','home.budget':'Budget','home.plan':'Plan','home.loans':'My loans','home.none':'No upcoming payment'});
register({id:'home',icon:icon('home'),labelKey:'tab.home',titleKey:'tab.home',html:'<div class="stack" id="home-content"></div>',async onShow(){
 const root=document.getElementById('home-content');
 try{await getJSON('api/loans',d=>{
  const live=(d.loans||[]).filter(l=>l.balance_major>0), next=[...live].filter(l=>l.next_due).sort((a,b)=>a.next_due.localeCompare(b.next_due))[0];
  root.innerHTML=`${(d.loans||[]).some(l=>l.needs_reconciliation)?`<button class="card" data-go="activity">${esc(T("payment.review"))}</button>`:""}<div class="card stack"><span>${esc(T('home.next'))}</span>${next?`<strong>${esc(next.name)}</strong><div class="v num">${esc(fmtMoney(next.next_payment_major,next.currency))}</div><span>${esc(fmtFull(next.next_due))}</span><button class="cta" data-go="loan" data-arg="${esc(next.id)}">${esc(T('loan.update'))}</button>`:`<p>${esc(T((d.loans||[]).some(l=>l.needs_reconciliation)?'payment.review':live.length?'home.none':'home.empty'))}</p><button class="cta" data-go="add">${esc(T('manage.add'))}</button>`}</div><div class="pair"><button class="card" data-go="budget">${icon('wallet')}<span>${esc(T('home.budget'))}</span></button><button class="card" data-go="plan">${icon('document')}<span>${esc(T('home.plan'))}</span></button></div><div class="card stack"><b>${esc(T('home.loans'))}</b>${live.slice(0,3).map(l=>`<button class="row alink" data-go="loan" data-arg="${esc(l.id)}">${icon(l.icon)}<span>${esc(l.name)}</span><b>${esc(fmtMoney(l.balance_major,l.currency))}</b></button>`).join('')}<button class="alink" data-go="loans">${esc(T('tab.loans'))}</button></div>`;
 });}catch{root.textContent=T('err.load');}
}});
