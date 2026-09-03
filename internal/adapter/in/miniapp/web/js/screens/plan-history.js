"use strict";
import {planEvidence} from "./plan-evidence.js";
import {register} from '../nav.js';
import {getJSON} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {esc,fmtMoney,fmtDate} from '../core.js';
addStrings({'history.title':'Պլանների պատմություն','history.empty':'Հաստատված պլաններ դեռ չկան','history.active':'Ակտիվ','history.old':'Տվյալները փոխվել են','history.replay':'Դիտել սկզբնական պլանը','history.original':'Սկզբնական հաշվարկ','history.engine':'Այս տարբերակի հաշվարկն այժմ հասանելի չէ','history.back':'Վերադառնալ պատմությանը'}, {'history.title':'Plan history','history.empty':'No approved plans yet','history.active':'Active','history.old':'Inputs have changed','history.replay':'View original plan','history.original':'Original calculation','history.engine':'This calculation version is unavailable','history.back':'Back to history'});
let busy=false;
async function load(){
 const data=await getJSON('api/plans');
 document.getElementById('history-list').innerHTML=(data.plans||[]).map(p=>`<article class="card stack"><strong>${esc(p.currency)}</strong><span>${esc(p.created_at.slice(0,10))}</span>${p.active?`<span class="pill">${esc(T('history.active'))}</span>`:''}${p.outdated?`<span class="hint">${esc(T('history.old'))}</span>`:''}<button class="alink" data-replay="${esc(p.id)}">${esc(T('history.replay'))}</button></article>`).join('')||esc(T('history.empty'));
}
register({id:'plan-history',parent:'plan',titleKey:'history.title',html:'<p id="history-error" class="error" role="alert"></p><div id="history-list" class="stack"></div><section id="history-detail" class="stack" hidden></section>',onMount(el){
 el.addEventListener('click',async e=>{
  const b=e.target.closest('[data-replay]');if(!b||busy)return;busy=true;b.disabled=true;
  try{
   const d=await getJSON('api/plans/'+encodeURIComponent(b.dataset.replay));
   const m=v=>esc(fmtMoney(v/10**d.currency_exponent,d.currency));
   const panel=document.getElementById('history-detail');
   panel.innerHTML=`<button class="alink" id="history-back">${esc(T('history.back'))}</button><div class="card stack"><strong>${esc(T('history.original'))}</strong><span>${esc(fmtDate(d.as_of))} · ${esc(d.engine_version)}</span><div class="kv"><div><span>${esc(T('plan.debtfree'))}</span><b>${esc(fmtDate(d.summary.payoff_date))}</b></div><div><span>${esc(T('plan.interest'))}</span><b class="num">${m(d.summary.interest_minor)}</b></div></div></div>${planEvidence(d)}`;
   document.getElementById('history-list').hidden=true;panel.hidden=false;
   document.getElementById('history-back').onclick=()=>{panel.hidden=true;document.getElementById('history-list').hidden=false;};
  }catch{document.getElementById('history-error').textContent=T('history.engine');}finally{busy=false;b.disabled=false;}
 });
},async onShow(){document.getElementById('history-error').textContent='';document.getElementById('history-detail').hidden=true;document.getElementById('history-list').hidden=false;try{await load();}catch{document.getElementById('history-error').textContent=T('err.load');}}});
