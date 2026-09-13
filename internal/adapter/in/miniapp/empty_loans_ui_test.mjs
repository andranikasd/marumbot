import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/loans.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map(),reads=[];
const field=id=>{if(!fields.has(id))fields.set(id,{hidden:true,children:[],textContent:'',style:{}});return fields.get(id);};
const env={document:{getElementById:field},register(screen){env.screen=screen;},addStrings(){},T:x=>x,
 getJSON:(path,callback)=>new Promise((resolve,reject)=>reads.push({resolve:body=>{callback(body);resolve(body);},reject}))};
vm.createContext(env);vm.runInContext(source,env);
let request=env.load();reads.shift().resolve({loans:[]});await request;
assert.equal(field('manage-empty').hidden,false);assert.equal(field('manage-error').hidden,true);
const older=env.load(),newer=env.load();
reads.pop().resolve({loans:[]});await newer;
reads.shift().reject(new Error('old failed read'));await older;
assert.equal(field('manage-empty').hidden,false);assert.equal(field('manage-error').hidden,true,'older error cannot overwrite a successful empty state');
request=env.load();reads.shift().reject(new Error('server unavailable'));await request;
assert.equal(field('manage-empty').hidden,true);assert.equal(field('manage-error').hidden,false,'genuine failures must not claim there are no loans');
console.log('Empty loans show onboarding; failed reads show errors and late failures cannot replace newer results.');

// Summary uses the account's server month, never the device's month. A later
// confirmed instalment is not due twice; missing or stale facts stay unknown.
env.fmtMoney=(value,currency)=>`${currency} ${value}`;env.fmtDate=value=>value;env.fmtFull=value=>value;env.sub=value=>value;
const paid={currency:'USD',balance_major:1200,next_due:'2026-10-15',next_payment_major:100,balance_as_of:'2026-09-20',name:'Paid'};
const unpaid={...paid,name:'Unpaid',next_due:'2026-09-25'};
env.summarise([paid],'2026-09-20');assert.equal(field('m-required').textContent,'USD 0');
env.summarise([paid,unpaid],'2026-09-20');assert.equal(field('m-required').textContent,'USD 100');
env.summarise([paid],undefined);assert.equal(field('m-required').textContent,'—');
env.summarise([{...paid,next_due:'2026-08-15'}],'2026-09-20');assert.equal(field('m-required').textContent,'—');
env.summarise([{...paid,needs_reconciliation:true}],'2026-09-20');assert.equal(field('m-required').textContent,'—');
env.summarise([paid],'2026-10-01');assert.equal(field('m-required').textContent,'USD 100');
console.log('Required this month excludes future confirmed payments and marks missing/overdue/reconciliation data unknown.');

// The navigator passes the mounted element first and route parameters second.
env.loanCard=()=>({});field('manage-list').append=()=>{};
request=env.screen.onShow({}, {setup:true});reads.shift().resolve({loans:[paid],today:'2026-09-20'});await request;
assert.equal(field('manage-continue').hidden,false,'saved wizard loans must offer Continue to monthly extra');
request=env.screen.onShow({}, null);reads.shift().resolve({loans:[paid],today:'2026-09-20'});await request;
assert.equal(field('manage-continue').hidden,true,'ordinary loan management must not force onboarding');
