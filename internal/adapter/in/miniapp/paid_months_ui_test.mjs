import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const read=async name=>(await readFile(new URL('./web/js/'+name,import.meta.url),'utf8')).replace(/^import .*;$/gm,'').replace(/^export /gm,'');
const source=await read('screens/paid-months.js'),mutation=await read('loan-mutations.js'),money=await read('screens/budget-funding.js');
function setup(){
 const fields=new Map(),calls=[];let screen,key=0;const node=id=>{if(!fields.has(id))fields.set(id,{value:'',textContent:'',checked:false,hidden:false,disabled:false,replaceChildren(...children){this.children=children;},addEventListener(name,fn){this[name]=fn;}});return fields.get(id);};
 const env={document:{getElementById:node,createElement:()=>({})},register:s=>screen=s,addStrings(){},T:k=>k,sub:(k,v)=>k+Object.values(v).join(),fmtFull:String,toast(){},haptic:{bad(){},ok(){}},invalidate(){},go:(id,p)=>calls.push({go:id,params:p}),currentScreen:()=> 'paid-months',crypto:{randomUUID:()=>String(++key)},api:(path,options)=>new Promise((resolve,reject)=>calls.push({path,options,resolve,reject}))};
 vm.createContext(env);vm.runInContext(money,env);vm.runInContext(mutation,env);vm.runInContext(source,env);screen.onMount();return {node,calls,screen};
}
const doc={id:'loan1',name:'Bank',version:2,today:'2026-09-20',currency:'AMD',currency_exponent:2,balance_major:100,payment_major:10,next_dates:{'2026-09':'2026-10-15'},needs_reconciliation:false,paid_through:'2026-08'};
const s=setup();let wait=s.screen.onShow(null,{id:'loan1'});s.calls.shift().resolve({ok:true,json:async()=>doc});await wait;
assert.equal(s.node('pm-current').hidden,false);assert.match(s.node('pm-current').textContent,/2026-08/);
assert.equal(s.node('pm-balance').value,'100');assert.equal(s.node('pm-confirm').checked,false);
await s.node('pm-form').submit();assert.equal(s.calls.length,0,'prefilled bank values require explicit confirmation');assert.equal(s.node('pm-status').textContent,'pm.invalid');
s.node('pm-confirm').checked=true;s.node('pm-balance').value='0.001';await s.node('pm-form').submit();assert.equal(s.calls.length,0,'sub-minor precision cannot become a different balance');
s.node('pm-balance').value='90.50';wait=s.node('pm-form').submit();let sent=s.calls.shift(),original=sent.options;
assert.equal(JSON.parse(sent.options.body).balance_major,90.5);assert.equal(sent.options.headers['If-Match'],'2');sent.reject(new Error('lost response'));await wait;
assert.equal(s.node('pm-fields').disabled,true);assert.equal(s.node('pm-retry').hidden,false);
await s.screen.onShow(null,{id:'loan1'});assert.equal(s.calls.length,0);assert.equal(s.node('pm-balance').value,'90.50');
wait=s.node('pm-retry').onclick();sent=s.calls.shift();assert.deepEqual(sent.options,original);sent.resolve({ok:true});await wait;assert.equal(s.calls.shift().go,'loan');
const conflict=setup();wait=conflict.screen.onShow(null,{id:'loan1'});conflict.calls.shift().resolve({ok:true,json:async()=>({...doc})});await wait;conflict.node('pm-confirm').checked=true;
wait=conflict.node('pm-form').submit();conflict.calls.shift().resolve({ok:false,status:409,json:async()=>({error:'loan_conflict'})});await wait;assert.equal(conflict.node('pm-fields').disabled,true);assert.equal(conflict.node('pm-reload').hidden,false);
const review=setup();wait=review.screen.onShow(null,{id:'loan1'});review.calls.shift().resolve({ok:true,json:async()=>({...doc,needs_reconciliation:true})});await wait;assert.equal(review.node('pm-fields').disabled,true);assert.equal(review.node('pm-review').hidden,false);
const full=setup();wait=full.screen.onShow(null,{id:'loan1'});full.calls.shift().resolve({ok:true,json:async()=>({...doc,next_dates:{}})});await wait;full.node('pm-confirm').checked=true;full.node('pm-balance').value='0';full.node('pm-payment').value='0';wait=full.node('pm-form').submit();sent=full.calls.shift();assert.equal(JSON.parse(sent.options.body).balance_major,0);sent.resolve({ok:true});await wait;
console.log('Paid months require confirmation and exact inputs; handle full repayment, reconciliation, conflicts and identical uncertain retries.');
const inconsistent=setup();wait=inconsistent.screen.onShow(null,{id:'loan1'});inconsistent.calls.shift().resolve({ok:true,json:async()=>({...doc})});await wait;inconsistent.node('pm-confirm').checked=true;
wait=inconsistent.node('pm-form').submit();inconsistent.calls.shift().resolve({ok:false,status:422,json:async()=>({error:'invalid_paid_month'})});await wait;
assert.equal(inconsistent.node('pm-status').textContent,'pm.rejected');assert.equal(inconsistent.node('pm-fields').disabled,false);assert.equal(inconsistent.node('pm-reload').hidden,false);assert.equal(inconsistent.node('pm-retry').hidden,true);
