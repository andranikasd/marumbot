import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/activity.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map();const field=id=>{if(!fields.has(id))fields.set(id,{value:'',innerHTML:'',textContent:'',addEventListener(){}});return fields.get(id);};
let screen,response;
const calls=[];
const row={loan_id:'loan',loan:'Golden',planned_minor:'12507940',planned_fee_minor:'0',posted_minor:'12507940',amount_delta_minor:'0',known_principal_minor:'5603410',known_interest_minor:'6904530',known_fee_minor:'0',fee_delta_minor:'0',missing_allocation_count:0,planned_dates:[{on:'2026-09-24',amount_minor:'12507940'}],posted_dates:[{on:'2026-09-25',amount_minor:'12507940'}],causes:['date']};
const comparison={currency:'AMD',currency_exponent:2,from:'2026-09-04',through:'2026-09-30',pending_count:1,excluded_before_activation_count:2,outside_baseline_count:0,rows:[row]};
const env={BigInt,Map,JSON,encodeURIComponent,document:{getElementById:field},register:s=>screen=s,icon:()=>'',addStrings(){},T:s=>s,esc:s=>String(s),fmtFull:s=>s,fmtMoney:s=>s,getJSON:async path=>{calls.push(path);if(path.startsWith('api/plan-actuals'))return response();if(path.startsWith('api/payment-actuals'))return {month:'2026-09',totals:[]};return {facts:[]};}};
vm.runInNewContext(source,env);field('activity-filter').value='all';field('activity-month').value='2026-09';response=async()=>({comparisons:[comparison]});await screen.onShow();
let html=field('activity-plan-actuals').innerHTML;
assert.match(html,/progress\.planned: 125079\.40 AMD/);assert.match(html,/progress\.posted: 125079\.40 AMD/);assert.match(html,/69045\.30 AMD/);assert.match(html,/progress\.date/);assert.match(html,/progress\.excluded: 2/);assert.match(html,/2026-09-04/);assert.doesNotMatch(html,/verified|matched|reconciled/);assert(calls.includes('api/plan-actuals?month=2026-09'));
const missing={...row,posted_minor:null,amount_delta_minor:null,known_principal_minor:null,known_interest_minor:null,known_fee_minor:null,fee_delta_minor:null,posted_dates:[],causes:['missing']};response=async()=>({comparisons:[{...comparison,rows:[missing]}]});await screen.onShow();html=field('activity-plan-actuals').innerHTML;
assert.match(html,/progress\.posted: activity\.unknown/);assert.match(html,/payment\.interest: activity\.unknown/);assert.match(html,/progress\.missing/);assert.doesNotMatch(html,/progress\.posted: 0\.00/);
assert.equal(vm.runInNewContext("exactMoney('-1',2,'AMD')",env),'-0.01 AMD');
// An older month response must not overwrite the selected month.
let finish;response=()=>new Promise(resolve=>finish=resolve);const first=vm.runInNewContext('loadPlanActuals()',env);field('activity-month').value='2026-10';response=async()=>({comparisons:[]});await vm.runInNewContext('loadPlanActuals()',env);finish({comparisons:[comparison]});await first;assert.equal(field('activity-plan-actuals').innerHTML,'progress.none');
console.log('Active-plan chart, explicit unknowns, dated evidence, and stale-month response guard passed');
