"use strict";
import {fmtMoney,fmtMonth,esc} from '../core.js';
import {addStrings,T} from '../i18n.js';
addStrings({'chart.type':'Գրաֆիկ','chart.balance':'Մայր գումարի կանխատեսում','chart.payments':'Ամսական վճարումներ','chart.interest':'Տոկոսի կանխատեսում','chart.required':'Պարտադիր','chart.extra':'Լրացուցիչ','chart.fees':'Վճարներ','chart.minimum':'Միայն պարտադիր վճարումներով','chart.plan':'Այս պլանը','chart.month':'Ամիս','chart.unavailable':'Համեմատությունը հասանելի չէ'},{'chart.type':'Chart','chart.balance':'Projected principal','chart.payments':'Monthly payments','chart.interest':'Projected interest','chart.required':'Required','chart.extra':'Extra','chart.fees':'Fees','chart.minimum':'Required-only plan','chart.plan':'This plan','chart.month':'Month','chart.unavailable':'Comparison unavailable'});
export const chartHTML=`<div class="card"><label for="chart-type" data-i18n="chart.type">Chart</label><select id="chart-type"><option value="balance" data-i18n="chart.balance">Projected principal</option><option value="payments" data-i18n="chart.payments">Monthly payments</option><option value="interest" data-i18n="chart.interest">Projected interest</option></select><div id="chart-legend" class="chart-legend"></div><div id="plan-chart"></div><label for="chart-month" id="chart-date"></label><input type="range" id="chart-month" style="accent-color:var(--accent)" min="0" value="0" step="1"><div id="chart-values"></div></div>`;
export function drawChart(d){
 const select=document.getElementById('chart-type'), slider=document.getElementById('chart-month');
 const months=d.months||[];
 slider.max=String(Math.max(0,months.length-1)); slider.value=String(Math.min(Number(slider.value),Number(slider.max)));
 const update=()=>{
  const mode=select.value, i=Number(slider.value), mo=months[i];
  if(!mo){document.getElementById('plan-chart').textContent='—';return;}
  const baseline=new Map((d.minimum_months||[]).map(m=>[m.on.slice(0,7),m]));
  const total=m=>m.required_minor+m.extra_minor+m.fees_minor;
  let series=[{label:T('chart.balance'),values:months.map(m=>m.owed_minor)}];
  if(mode==='payments') series=[{label:T('chart.payments'),values:months.map(total)},{label:T('chart.required'),values:months.map(m=>m.required_minor)}];
  if(mode==='interest'){
   series=[{label:T('chart.plan'),values:months.map(m=>m.interest_minor)}];
   if(d.baseline_available) series.push({label:T('chart.minimum'),values:months.map(m=>baseline.get(m.on.slice(0,7))?.interest_minor??null)});
  }
  const max=Math.max(1,...series.flatMap(s=>s.values.filter(v=>v!=null))), x=n=>16+n*288/Math.max(1,months.length-1), y=v=>142-v/max*126;
  const lines=series.map((s,j)=>{let path='',open=false; s.values.forEach((v,k)=>{if(v==null){open=false;return;}path+=`${open?'L':'M'}${x(k).toFixed(1)} ${y(v).toFixed(1)} `;open=true;});return `<path class="series-${j}" d="${path}" fill="none" stroke="${j?'var(--hint)':'var(--accent)'}" stroke-width="2.5" ${j?'stroke-dasharray="5 4"':''}/>`;}).join('');
  document.getElementById('chart-legend').innerHTML=series.map((s,j)=>`<span><i class="key-${j}" aria-hidden="true"></i>${esc(s.label)}</span>`).join('');
  document.getElementById('plan-chart').innerHTML=`<svg class="plan-chart" viewBox="0 0 320 160" role="img" aria-label="${esc(T('chart.'+mode))}"><path class="grid-line" d="M16 142H304M16 79H304M16 16H304" fill="none" stroke="var(--line)"/>${lines}<path class="grid-line" d="M${x(i)} 8V148" stroke="var(--hint)" stroke-dasharray="2 4"/></svg>`;
  document.getElementById('chart-date').textContent=T('chart.month')+': '+fmtMonth(mo.on);
  const money=v=>fmtMoney(v/10**(d.currency_exponent??2),d.currency);
  let rows=series.map(s=>[s.label,s.values[i]==null?'—':money(s.values[i])]);
  if(mode==='payments') rows.push([T('chart.extra'),money(mo.extra_minor)],[T('chart.fees'),money(mo.fees_minor)]);
  if(mode==='interest'&&!d.baseline_available) rows.push([T('chart.minimum'),T('chart.unavailable')]);
  document.getElementById('chart-values').innerHTML=rows.map(([label,v])=>`<div><span>${esc(label)}</span><b>${esc(v)}</b></div>`).join('');
 };
 select.onchange=update;slider.oninput=update;update();
}
