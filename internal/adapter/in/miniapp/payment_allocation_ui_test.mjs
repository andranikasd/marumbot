import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/payment.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map();
const field=id=>{if(!fields.has(id))fields.set(id,{value:'',disabled:false,textContent:'',addEventListener(type,fn){this[type]=fn;},reset(){for(const f of fields.values())f.value='';}});return fields.get(id);};
let screen,sequence=0,current='payment',response,confirmation=async()=>true;
const calls=[],destinations=[];
const env={BigInt,Number,Map,JSON,encodeURIComponent,Error,crypto:{randomUUID:()=>`key-${++sequence}`},document:{getElementById:field,querySelectorAll:()=>[...fields.values()]},register:s=>screen=s,currentScreen:()=>current,go:id=>destinations.push(id),addStrings(){},T:s=>s,toast(){},invalidate(){},confirmDialog:()=>confirmation(),getJSON:async path=>({loan_id:path.split('/')[2],loan:'Fixture',currency:'AMD',currency_exponent:2,version:0,today:'2026-09-24'}),api:(path,init)=>{calls.push({path,body:JSON.parse(init.body)});return response();}};
vm.runInNewContext(source,env);screen.onMount();
const show=id=>{current='payment';return screen.onShow(null,{id});};
const submit=()=>field('payment-form').submit({preventDefault(){}});
const result=(ok=true)=>({ok,status:ok?200:409,json:async()=>ok?{}:{error:'possible_duplicate_payment'}});
await show('golden');field('pay-amount').value='125079.40';field('pay-posting').value='posted';field('pay-value').value='2026-09-24';field('pay-allocation').value='known';
field('pay-principal').value='56034.10';field('pay-interest').value='69045.30';field('pay-fees').value='';
response=async()=>result();await submit();assert.equal(calls.length,0,'missing fees cannot mean zero');
field('pay-fees').value='0';await submit();assert.deepEqual(calls.at(-1).body.allocation,{principal_minor:5603410,interest_minor:6904530,fees_minor:0});
await show('unknown');field('pay-amount').value='1';await submit();assert.equal('allocation' in calls.at(-1).body,false);
// A's completion cannot unlock B while B has its own request in flight.
let finishA,finishB;
await show('A');field('pay-amount').value='1';response=()=>new Promise(resolve=>finishA=resolve);const a=submit();
await show('B');field('pay-amount').value='2';response=()=>new Promise(resolve=>finishB=resolve);const b=submit();
const before=calls.length;finishA(result());await a;assert.equal(field('pay-save').disabled,true);await submit();assert.equal(calls.length,before,'A must not clear B busy state');
finishB(result());await b;
// Navigating away during duplicate confirmation must not issue a second POST.
await show('duplicate');field('pay-amount').value='3';response=async()=>result(false);
let answer,entered;const waiting=new Promise(resolve=>entered=resolve);confirmation=()=>{entered();return new Promise(resolve=>answer=resolve);};
const duplicate=submit();await waiting;await show('other');const count=calls.length;answer(true);await duplicate;assert.equal(calls.length,count);
// Going to another screen without opening another payment must not navigate back.
await show('away');field('pay-amount').value='4';response=()=>new Promise(resolve=>finishA=resolve);const away=submit();current='loan';const routes=destinations.length;finishA(result());await away;assert.equal(destinations.length,routes);
console.log('Allocation fixture, explicit zeros, and delayed navigation guards passed');
