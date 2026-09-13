import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const directory=new URL('./web/js/screens/',import.meta.url);
const helpers=(await readFile(new URL('projection-format.js',directory),'utf8')).replace(/^export /gm,'');
const helpersContext=vm.createContext({});vm.runInContext(helpers,helpersContext);
for(const [text,exponent,want] of [['0',2,0],['25 000',0,25000],['12,34',2,1234],['0.01',2,1],['12.345',2,null],['-1',2,null],['',2,null],['1e3',2,null],['1,2.3',2,null],['9007199254740992',0,null],['90071992547409.91',2,9007199254740991]])assert.equal(helpersContext.extraMinor(text,exponent),want,text);
assert.equal(helpersContext.extraText(9007199254740991,2),'90071992547409.91');
function setup(file){
 const fields=new Map(),calls=[];let screen,id=0,active=file==='extra.js'?'extra':'simple-plan';
 const node=key=>{if(!fields.has(key))fields.set(key,{value:'',dataset:{},hidden:false,disabled:false,textContent:'',innerHTML:'',focus(){this.focused=true;},addEventListener(name,fn){this[name]=fn;}});return fields.get(key);};
 const env={document:{getElementById:node},register:s=>screen=s,addStrings(){},T:k=>k,sub:(k,p)=>k+':'+Object.values(p).join(','),esc:String,fmtMoney:(n,c)=>`${c} ${n}`,fmtMonth:String,fmtDate:String,icon:()=>'',haptic:{ok(){},bad(){}},invalidate(){},currentScreen:()=>active,go:(target,params)=>calls.push({go:target,params}),structuredClone,crypto:{randomUUID:()=>String(++id)},api:(path,options)=>new Promise((resolve,reject)=>calls.push({path,options,resolve,reject}))};
 vm.createContext(env);vm.runInContext(helpers,env);
 return readFile(new URL(file,directory),'utf8').then(src=>{vm.runInContext(src.replace(/^import .*;$/gm,''),env);screen.onMount?.();return {node,calls,screen,setActive:s=>active=s};});
}
const settings={enabled:false,version:3,currencies:{AMD:{extra_minor:25000,overrides:{'2026-11':0}},USD:{extra_minor:100,overrides:{}}}};
const projection={today:'2026-09-14',settings_version:3,enabled:false,currencies:[{currency:'AMD',exponent:0,start_month:'2026-10',complete:true,finish:'2027-06',months:[{month:'2026-10',required_minor:75000,extra_minor:25000,total_minor:100000,loans:[{id:'a',name:'Loan',due:'2026-10-15',required_minor:75000,extra_minor:25000}]},{month:'2026-11',required_minor:75000,extra_minor:0,total_minor:75000,loans:[]}]},{currency:'USD',exponent:2,start_month:'2026-10',complete:true,months:[{month:'2026-10',required_minor:10000,extra_minor:100,total_minor:10100,loans:[]}]}]};
async function loadExtra(s,params,saved=settings){const wait=s.screen.onShow(null,params);s.calls.shift().resolve({ok:true,json:async()=>saved});s.calls.shift().resolve({ok:true,json:async()=>projection});await wait;}
const state=await setup('extra.js');await loadExtra(state);assert.equal(state.node('extra-fields').disabled,false);assert.equal(state.node('extra-consent').hidden,false);
state.node('extra-coverage').input({target:{dataset:{cover:'AMD'},value:'50 000'}});
assert.equal(state.node('extra-gap-AMD').textContent,'extra.gap:AMD 25000','approved 75,000 required minus 50,000 available = 25,000 shortfall');
state.node('extra-coverage').input({target:{dataset:{cover:'AMD'},value:'75 000'}});assert.equal(state.node('extra-gap-AMD').textContent,'extra.covered');
state.node('extra-content').input({target:{dataset:{currency:'AMD'},value:'0'}});
assert.equal(state.node('extra-total-AMD').textContent,'AMD 75000');
let wait=state.node('extra-form').submit({preventDefault(){}}),write=state.calls.shift(),body=write.options.body;
assert.equal(JSON.parse(body).currencies.AMD.extra_minor,0,'zero is a valid extra');assert.equal(JSON.parse(body).currencies.USD.extra_minor,100,'currency amounts remain separate');assert.equal(JSON.parse(body).currencies.AMD.overrides['2026-11'],0,'saving default preserves overrides');
write.reject(new Error('lost'));await wait;assert.equal(state.node('extra-fields').disabled,true);assert.equal(state.node('extra-status').textContent,'extra.uncertain');
await state.screen.onShow(null,{month:'2026-11',currency:'USD'});assert.equal(state.calls.length,0,'unresolved save survives different navigation');
wait=state.node('extra-retry').onclick();write=state.calls.shift();assert.equal(write.options.body,body,'retry uses identical source, version and key');write.resolve({ok:true});await wait;assert.equal(state.calls.shift().go,'simple-plan');
const monthly=await setup('extra.js');await loadExtra(monthly,{month:'2026-11',currency:'AMD'},{...settings,enabled:true});
wait=monthly.node('extra-reset').onclick();write=monthly.calls.shift();const reset=JSON.parse(write.options.body);assert.equal(Object.hasOwn(reset.currencies.AMD.overrides,'2026-11'),false);assert.equal(reset.currencies.AMD.extra_minor,25000);write.resolve({ok:true});await wait;
const conflict=await setup('extra.js');await loadExtra(conflict);wait=conflict.node('extra-form').submit({preventDefault(){}});conflict.calls.shift().resolve({ok:false,status:409});await wait;assert.equal(conflict.node('extra-fields').disabled,true);assert.equal(conflict.node('extra-reload').hidden,false);
const invalid=await setup('extra.js');await loadExtra(invalid);invalid.node('extra-content').input({target:{dataset:{currency:'AMD'},value:'1.5'}});await invalid.node('extra-form').submit({preventDefault(){}});assert.equal(invalid.calls.length,0,'never round excess precision');assert.equal(invalid.node('extra-status').textContent,'extra.invalid');
const plan=await setup('simple-plan.js');wait=plan.screen.onShow(null,{});plan.calls.shift().resolve({ok:true,json:async()=>({...projection,enabled:true})});plan.calls.shift().resolve({ok:true,json:async()=>({loans:[{}]})});await wait;
assert.match(plan.node('sp-content').innerHTML,/AMD 100000/);assert.match(plan.node('sp-content').innerHTML,/sp-currency/);assert.doesNotMatch(plan.node('sp-content').innerHTML,/data-go="paid-months"/,'future months cannot be marked paid');
plan.node('sp-change').onclick();assert.deepEqual(JSON.parse(JSON.stringify(plan.calls.shift().params)),{currency:'AMD',month:'2026-10'});
const empty=await setup('simple-plan.js');wait=empty.screen.onShow(null,{});empty.calls.shift().resolve({ok:true,json:async()=>({...projection,currencies:[]})});empty.calls.shift().resolve({ok:true,json:async()=>({loans:[]})});await wait;assert.equal(empty.calls.shift().go,'welcome');
const failed=await setup('simple-plan.js');wait=failed.screen.onShow(null,{});failed.calls.shift().reject(new Error('offline'));failed.calls.shift().resolve({ok:true,json:async()=>({loans:[]})});await wait;assert.match(failed.node('sp-content').innerHTML,/err.load/);assert.doesNotMatch(failed.node('sp-content').innerHTML,/sp.repaid/);
console.log('Projection UI: exact money, currency separation, zero extras, replacement/reset, identical retries, conflicts, future paid guard, first use and failed loads.');

{
 const limited=await setup('simple-plan.js');const p=structuredClone(projection);p.enabled=true;
 p.currencies[0].complete=false;p.currencies[0].reason='accrued_interest_needed';
 p.currencies[0].months[0].extra_minor=0;p.currencies[0].months[0].requested_extra_minor=25000;p.currencies[0].months[0].unallocated_extra_minor=25000;p.currencies[0].months[0].total_minor=75000;
 const waiting=limited.screen.onShow(null,{});limited.calls.shift().resolve({ok:true,json:async()=>p});limited.calls.shift().resolve({ok:true,json:async()=>({loans:[{}]})});await waiting;
 const html=limited.node('sp-content').innerHTML;
 assert.match(html,/sp.reason.accrued_interest_needed/);assert.match(html,/sp.known.required/);assert.match(html,/sp.unallocated/);assert.doesNotMatch(html,/sp.finish/,'missing accrued interest suppresses finish despite a stale populated finish property');
}

{
 const review=await setup('simple-plan.js');const waiting=review.screen.onShow(null,{});
 review.calls.shift().resolve({ok:false,status:422,json:async()=>({error:'payment_reconciliation_required'})});review.calls.shift().resolve({ok:true,json:async()=>({loans:[{balance_major:0,needs_reconciliation:true}]})});await waiting;
 assert.match(review.node('sp-content').innerHTML,/sp.reason.payment_reconciliation_required/);assert.match(review.node('sp-content').innerHTML,/data-go="activity"/);assert.doesNotMatch(review.node('sp-content').innerHTML,/sp.repaid/);
}
