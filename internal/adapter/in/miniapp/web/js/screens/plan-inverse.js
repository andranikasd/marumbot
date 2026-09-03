"use strict";
import {register} from '../nav.js';
import {getJSON} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {fmtMoney} from '../core.js';
addStrings({'inverse.title':'Բյուջե ըստ վերջնաժամկետի','inverse.target':'Մարման ցանկալի օրը','inverse.calculate':'Հաշվել','inverse.unsupported':'Այս վարկերի համար նվազագույն բյուջեն հաստատել հնարավոր չէ։ Փորձեք համեմատել բյուջեի տարբերակները։','inverse.cash':'Սա ծախսերի սահմանն է։ Գումարի հասանելիությունը հաստատեք առանձին։'}, {'inverse.title':'Budget by target date','inverse.target':'Target payoff date','inverse.calculate':'Calculate','inverse.unsupported':'A minimum budget cannot be proven for these loan terms. Compare budget scenarios instead.','inverse.cash':'This is a spending limit. Confirm cash availability separately.'});
let proposal='';
register({id:'plan-inverse',parent:'plan',titleKey:'inverse.title',html:'<form id="inverse-form" class="card stack"><div class="field"><label for="inverse-target" data-i18n="inverse.target"></label><input id="inverse-target" type="date" required></div><button class="cta" data-i18n="inverse.calculate"></button><p class="hint" data-i18n="inverse.cash"></p></form><p id="inverse-result" class="card" role="status" hidden></p>',onMount(){
 document.getElementById('inverse-form').onsubmit=async e=>{e.preventDefault();const b=e.target.querySelector('button'),out=document.getElementById('inverse-result');b.disabled=true;out.hidden=false;
 try{const d=await getJSON('api/plan/budget-by-date?proposal='+encodeURIComponent(proposal)+'&target='+encodeURIComponent(document.getElementById('inverse-target').value));out.textContent=d.supported?fmtMoney(d.minimum_minor/10**d.currency_exponent,d.currency):T('inverse.unsupported');}catch{out.textContent=T('err.load');}finally{b.disabled=false;}};
},async onShow(){proposal='';const out=document.getElementById('inverse-result');out.hidden=true;const b=document.querySelector('#inverse-form button');b.disabled=true;try{const d=await getJSON('api/plan');proposal=d.proposal;document.getElementById('inverse-target').min=d.as_of;b.disabled=false;}catch{out.hidden=false;out.textContent=T('err.load');}}});
