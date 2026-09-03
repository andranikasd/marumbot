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
 register:s=>{screen=s;},go:id=>destinations.push(id),addStrings(){},T:s=>s,toast(){},invalidate(){},confirmDialog:async()=>confirm,
 getJSON:async path=>({loan_id:path.split('/')[2],loan:'Synthetic',currency:'AMD',currency_exponent:2,version:0,today:'2026-09-03'}),
 api:(path,init)=>{calls.push({path,body:JSON.parse(init.body)});return response();}};
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
