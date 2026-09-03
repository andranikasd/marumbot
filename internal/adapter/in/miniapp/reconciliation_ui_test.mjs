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
function fill(){for(const [id,value] of [['principal','600.00'],['payment','300.00'],['due','2026-10-15'],['cash','600.00'],['spent','400.00']])field('rec-'+id).value=value;field('rec-confirm').checked=true;}
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
