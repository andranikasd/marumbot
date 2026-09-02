"use strict";
import {register} from '../nav.js';
import {getJSON} from '../api.js';
import {addStrings,T} from '../i18n.js';
import {fmtMoney,fmtFull,esc} from '../core.js';
import {icon} from '../icons.js';
addStrings({'tab.activity':'Պատմություն','activity.balance':'Մնացորդի գրառում','activity.empty':'Գրառումներ դեռ չկան'},{'tab.activity':'Activity','activity.balance':'Balance statement','activity.empty':'No records yet'});
register({id:'activity',icon:icon('document'),labelKey:'tab.activity',html:'<div id="activity-list" class="stack"></div>',async onShow(){const root=document.getElementById('activity-list');try{await getJSON('api/activity',d=>{root.innerHTML=(d.facts||[]).map(f=>`<button class="card stack" data-go="loan" data-arg="${esc(f.loan_id)}"><b>${esc(f.loan)}</b><span>${esc(T('activity.balance'))} · ${esc(fmtFull(f.as_of))}</span><strong>${esc(fmtMoney(f.principal_minor/10**f.currency_exponent,f.currency))}</strong></button>`).join('')||esc(T('activity.empty'));});}catch{root.textContent=T('err.load');}}});
