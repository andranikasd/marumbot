// Exercise the real screen lifecycle with controlled transport responses.
// Browser checks cover DOM layout; these cover delayed/lost responses.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/payment.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map();
function field(id){if(!fields.has(id))fields.set(id,{value:'',disabled:false,textContent:'',addEventListener(type,fn){this[type]=fn;},reset(){for(const el of fields.values())el.value='';}});return fields.get(id);}
let screen,sequence=0,response,confirm=true;
const calls=[],destinations=[];
const env={BigInt,Number,Map,JSON,encodeURIComponent,Error,crypto:{randomUUID:()=>`key-${++sequence}`},
 document:{getElementById:field,querySelectorAll:()=>[...fields.values()]},
 register:s=>{screen=s;},go:(id,params)=>destinations.push(params?{id,params}:id),addStrings(){},T:s=>s,toast(){},invalidate(){},confirmDialog:async()=>confirm,
 getJSON:async path=>({loan_id:path.split('/')[2],loan:'Synthetic',currency:'AMD',currency_exponent:2,version:0,today:'2026-09-03'}),
 api:(path,init)=>{if(init.method!=='POST')return Promise.resolve({ok:true,json:()=>env.getJSON(path)});calls.push({path,body:JSON.parse(init.body)});return response();}};
vm.runInNewContext(source,env);
screen.onMount();
const show=id=>screen.onShow(null,{id});
const submit=()=>field('payment-form').submit({preventDefault(){}});
function amount(value){field('pay-amount').value=value;}
const ok=()=>Promise.resolve({ok:true,status:200,json:async()=>({})});
const lost=()=>Promise.reject(new TypeError('response lost'));
response=lost;await show('A');amount('123.45');await submit();
const originalA=calls.at(-1).body;
assert.equal(field('pay-amount').disabled,true);
await show('B');amount('25.00');await submit();
const originalB=calls.at(-1).body;
await show('A');assert.equal(field('pay-amount').value,'123.45');assert.equal(field('pay-amount').disabled,true);
let finish;
response=()=>new Promise(resolve=>{finish=resolve;});
const inFlight=submit();
assert.deepEqual(calls.at(-1).body,originalA);
await show('B');assert.equal(field('pay-amount').value,'25.00');
finish(await ok());await inFlight;
assert.deepEqual(destinations,[], 'A completion must not navigate away from B');
await show('B');assert.equal(field('pay-amount').disabled,true);
response=ok;await submit();
assert.deepEqual(calls.at(-1).body,originalB,'A completion must not erase B retry identity');
assert.equal(calls.at(-1).path,'api/loans/B/payments');
// Declining a duplicate is definitive: permit edits and submit the new amount.
await show('C');amount('10.00');confirm=false;
response=()=>Promise.resolve({ok:false,status:409,json:async()=>({error:'possible_duplicate_payment'})});
await submit();assert.equal(field('pay-amount').disabled,false);assert.equal(field('pay-save').disabled,false);
amount('11.00');response=ok;await submit();assert.equal(calls.at(-1).body.amount_minor,1100);
console.log('Payment retries preserve source facts across navigation and uncertain responses');

await show('posted');amount('12.00');field('pay-posting').value='posted';field('pay-value').value='2026-09-03';await submit();
assert.equal(field('payment-form').hidden,true);assert.equal(field('pay-done').hidden,false);assert.equal(field('pay-check').hidden,false);
field('pay-check').onclick();assert.equal(destinations.at(-1).id,'reconcile');assert.equal(destinations.at(-1).params.id,'posted');
await show('pending');amount('12.00');await submit();assert.equal(field('pay-check').hidden,true);assert.equal(field('pay-done-message').textContent,'payment.done.pending');
await show('dates');amount('12.00');field('pay-posting').value='posted';field('pay-value').value='';const before=calls.length;await submit();assert.equal(calls.length,before);assert.equal(field('pay-error').textContent,'payment.dateError');
console.log('Saved payments give the correct next action and reject missing bank dates');

// Language/settings and other-loan navigation preserve separate unsaved drafts.
await show('draft-A');amount('19.25');field('pay-posting').value='posted';field('pay-value').value='2026-09-02';field('pay-allocation-wrap').open=true;
await show('draft-B');amount('42.00');
await show('draft-A');assert.equal(field('pay-amount').value,'19.25');assert.equal(field('pay-value').value,'2026-09-02');assert.equal(field('pay-allocation-wrap').open,true);
await show('draft-B');assert.equal(field('pay-amount').value,'42.00');
const fact={id:'correction-fact',amount_minor:800,transaction_date:'2026-09-01',value_date:'',kind:'payment_reported'};
await screen.onShow(null,{id:'draft-A',fact});assert.equal(field('pay-amount').value,'8.00','new and correction drafts are separate');
amount('9.00');await show('draft-B');await screen.onShow(null,{id:'draft-A',fact});assert.equal(field('pay-amount').value,'9.00');
field('pay-reload').click();assert.equal(destinations.at(-1).params.fact.id,fact.id,'reload must retain correction identity');
console.log('Payment drafts survive navigation per loan and correction; reload retains the corrected fact');
