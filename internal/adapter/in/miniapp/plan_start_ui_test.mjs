import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/plan-start.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
function setup(){
 const fields=new Map(),calls=[];let screen,id=0;const node=key=>{if(!fields.has(key))fields.set(key,{value:'',hidden:false,disabled:false,textContent:'',addEventListener(name,fn){this[name]=fn;}});return fields.get(key);};
 const env={document:{getElementById:node},register:s=>screen=s,addStrings(){},T:k=>k,sub:(k,p)=>k+':'+p.month,toast(){},haptic:{ok(){},bad(){}},invalidate(){},currentScreen:()=> 'plan-start',go:target=>calls.push({go:target}),crypto:{randomUUID:()=>String(++id)},api:(path,options)=>new Promise((resolve,reject)=>calls.push({path,options,resolve,reject}))};
 vm.createContext(env);vm.runInContext(source,env);screen.onMount();return {node,calls,screen};
}
const budget={version:3,today:'2026-12-13',monthly_major:100,funding:{planning_start_month:''}};
const s=setup();let pending=s.screen.onShow();s.calls.shift().resolve({ok:true,json:async()=>budget});await pending;
assert.equal(s.node('ps-next').value,'2027-01','next month crosses year using the server date');
assert.equal(s.node('ps-month').value,'');assert.equal(s.node('ps-fields').disabled,false);
s.node('ps-month').value='2027-01';s.node('ps-month').change();
pending=s.node('ps-form').submit({preventDefault(){}});let write=s.calls.shift();const original=write.options.body;assert.equal(JSON.parse(original).expected_version,3);write.reject(new Error('lost response'));await pending;
assert.equal(s.node('ps-fields').disabled,true);assert.equal(s.node('ps-retry').hidden,false);assert.equal(s.node('ps-status').textContent,'ps.uncertain');
await s.screen.onShow();assert.equal(s.calls.length,0,'revisiting preserves unresolved write');
pending=s.node('ps-retry').onclick();write=s.calls.shift();assert.equal(write.options.body,original);write.resolve({ok:true});await pending;assert.equal(s.calls.shift().go,'plan');
const conflict=setup();pending=conflict.screen.onShow();conflict.calls.shift().resolve({ok:true,json:async()=>budget});await pending;
pending=conflict.node('ps-form').submit();conflict.calls.shift().resolve({ok:false,status:409,json:async()=>({})});await pending;
assert.equal(conflict.node('ps-fields').disabled,true);assert.equal(conflict.node('ps-reload').hidden,false);assert.equal(conflict.node('ps-status').textContent,'ps.conflict');
const unpaid=setup();pending=unpaid.screen.onShow();unpaid.calls.shift().resolve({ok:true,json:async()=>budget});await pending;
pending=unpaid.node('ps-form').submit();unpaid.calls.shift().resolve({ok:false,status:422,json:async()=>({reason:'required payments remain before planning start'})});await pending;
assert.equal(unpaid.node('ps-status').textContent,'ps.unpaid');assert.equal(unpaid.node('ps-fields').disabled,false);assert.equal(unpaid.node('ps-retry').hidden,true);
const empty=setup();pending=empty.screen.onShow();empty.calls.shift().resolve({ok:true,json:async()=>({...budget,monthly_major:null,funding:null})});await pending;assert.equal(empty.node('ps-budget').hidden,false);assert.equal(empty.node('ps-fields').disabled,true);
const fail=setup();pending=fail.screen.onShow();fail.calls.shift().reject(new Error('offline'));await pending;assert.equal(fail.node('ps-status').textContent,'err.load');assert.equal(fail.node('ps-reload').hidden,false);assert.equal(fail.node('ps-fields').disabled,true);
console.log('Plan start handles December, missing budget, conflicts, unpaid obligations, failed loads and identical uncertain retries.');
for(const [error,message,reviewVisible] of [['payment_reconciliation_required','ps.review',true],['loan_schedule_invalid','ps.invalidLoan',false]]){
 const state=setup();let pending=state.screen.onShow();state.calls.shift().resolve({ok:true,json:async()=>budget});await pending;
 pending=state.node('ps-form').submit();state.calls.shift().resolve({ok:false,status:422,json:async()=>({error})});await pending;
 assert.equal(state.node('ps-status').textContent,message);assert.equal(state.node('ps-review').hidden,!reviewVisible);
 assert.equal(state.node('ps-retry').hidden,true,'a definitive rejection is not an uncertain retry');assert.equal(state.node('ps-fields').disabled,false);
}
