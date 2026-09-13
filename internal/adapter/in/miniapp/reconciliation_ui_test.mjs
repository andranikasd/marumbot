import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const fields=new Map();
function field(id){if(!fields.has(id))fields.set(id,{value:'',checked:false,disabled:false,textContent:'',hidden:false,querySelectorAll(){return [...fields.values()];},addEventListener(type,fn){this[type]=fn;},reset(){for(const f of fields.values()){f.value='';f.checked=false;}}});return fields.get(id);}
let screen,response,sequence=0,current='reconcile';const calls=[],destinations=[];
const env={BigInt,Number,Map,JSON,Date,Error,encodeURIComponent,crypto:{randomUUID:()=>`key-${++sequence}`},document:{getElementById:field},
 register:s=>{screen=s;},currentScreen:()=>current,go:id=>destinations.push(id),addStrings(){},T:s=>s,invalidate(){},
 getJSON:async path=>path==='api/budget'?{currency:'AMD',version:2,funding:{}}:{loan_id:path.split('/')[2],loan:'Synthetic',currency:'AMD',currency_exponent:2,version:3,today:'2026-09-03'},
 api:(path,init)=>{if(init.method!=='POST')return Promise.resolve({ok:true,json:()=>env.getJSON(path)});calls.push({path,body:JSON.parse(init.body)});return response();}};
vm.createContext(env);
for(const file of ['budget-funding.js','reconcile.js']){
 const source=(await readFile(new URL('./web/js/screens/'+file,import.meta.url),'utf8')).replace(/^import .*;$/gm,'').replaceAll('export ','');vm.runInContext(source,env);
}
screen.onMount();
async function show(id){current='reconcile';await screen.onShow(null,{id});}
function fill(){for(const [id,value] of [['principal','600.00'],['payment','300.00'],['due','2026-10-15'],['cash','600.00'],['spent','400.00']])field('rec-'+id).value=value;field('rec-next').click();field('rec-review').click();field('rec-confirm').checked=true;}
const submit=()=>field('reconcile-form').submit({preventDefault(){}});
response=()=>Promise.reject(new TypeError('lost response'));await show('A');fill();await submit();
assert.ok(calls.length,field('rec-error').textContent+' '+JSON.stringify(vm.runInContext('context',env)));
const first=calls.at(-1).body;assert.equal(first.principal_minor,60000);assert.equal(first.spent_minor,40000);assert.equal(field('rec-principal').disabled,true);
await show('B');fill();field('rec-principal').value='900';await submit();const second=calls.at(-1).body;assert.notEqual(first.idempotency_key,second.idempotency_key);
await show('A');assert.equal(field('rec-principal').value,'600.00');
response=()=>Promise.resolve({ok:true,status:200});await submit();assert.deepEqual(calls.at(-1).body,first);assert.equal(destinations.at(-1),'activity');
await show('B');response=()=>Promise.resolve({ok:false,status:409});await submit();assert.equal(field('rec-save').disabled,true);
await show('B');fill();response=()=>Promise.resolve({ok:true,status:200});await submit();assert.notEqual(calls.at(-1).body.idempotency_key,second.idempotency_key);
console.log('Reconciliation preserves exact cash statements and retry identity across navigation');

// The guided journey validates each question before showing the review.
await show('guided');
assert.equal(field('rec-balance-block').hidden,false);
field('rec-next').click();assert.equal(field('rec-cash-block').hidden,true);
field('rec-principal').value='0';field('rec-principal').input();
assert.equal(field('rec-due').disabled,true);assert.equal(field('rec-zero').hidden,false);
field('rec-next').click();assert.equal(field('rec-cash-block').hidden,false);
field('rec-review').click();assert.equal(field('rec-review-block').hidden,true);
field('rec-cash').value='0';field('rec-spent').value='400.00';field('rec-review').click();
assert.equal(field('rec-review-block').hidden,false);assert.equal(field('rec-review-payment-row').hidden,true);
assert.equal(field('rec-review-spent').textContent,'400.00 AMD');
const beforeConfirm=calls.length;await submit();assert.equal(calls.length,beforeConfirm);
field('rec-confirm').checked=true;response=()=>Promise.resolve({ok:true,status:200});await submit();
assert.equal(calls.at(-1).body.principal_minor,0);assert.equal(calls.at(-1).body.next_due,'');assert.equal(calls.at(-1).body.next_payment_minor,0);
// A custom budget cycle uses the server's actual period, not the calendar month.
const originalRead=env.getJSON;
env.getJSON=async path=>path==='api/budget'?{currency:'AMD',version:2,spent_period_start:'2026-08-15',funding:{}}:originalRead(path);
await show('cycle');fill();await submit();assert.equal(calls.at(-1).body.spent_period_start,'2026-08-15');
// No budget offers setup rather than claiming an outage or allowing a partial save.
env.getJSON=async path=>path==='api/budget'?{today:'2026-09-03'}:originalRead(path);
await show('no-budget');assert.equal(field('rec-funding').hidden,false);assert.equal(field('rec-save').disabled,true);assert.equal(field('rec-next').disabled,true);
env.getJSON=originalRead;
await show('pending');fill();response=()=>Promise.resolve({ok:false,status:422,json:async()=>({error:'payment_reconciliation_required'})});await submit();
assert.equal(field('rec-error').textContent,'reconcile.pending');assert.equal(field('rec-history').hidden,false);
await show('overdue');field('rec-principal').value='600';field('rec-payment').value='300';field('rec-due').value='2026-09-03';field('rec-next').click();
assert.equal(field('rec-error').textContent,'reconcile.future');assert.equal(field('rec-cash-block').hidden,true);
console.log('Guided reconciliation covers review, zero balances, cycle periods, missing budgets and actionable failures');

await show('draft-A');fill();field('rec-cash').value='123.45';
await show('draft-B');field('rec-principal').value='876.00';
await show('draft-A');assert.equal(field('rec-cash').value,'123.45');assert.equal(field('rec-confirm').checked,true);assert.equal(field('rec-review-block').hidden,false);
await show('draft-B');assert.equal(field('rec-principal').value,'876.00');assert.equal(field('rec-balance-block').hidden,false);
const draftReads=env.getJSON;env.getJSON=async()=>{throw new Error('offline');};
await show('draft-A');assert.equal(field('rec-cash').value,'123.45','draft remains available while offline');
env.getJSON=draftReads;
console.log('Reconciliation drafts retain answers, review step and original statement context per loan');
