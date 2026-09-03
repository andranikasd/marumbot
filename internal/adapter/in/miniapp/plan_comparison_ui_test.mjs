import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/plan-comparison.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map();
function field(id){if(!fields.has(id))fields.set(id,{disabled:false,textContent:'',innerHTML:'',style:{},append(){},setAttribute(){},addEventListener(type,fn){this[type]=fn;}});return fields.get(id);}
let screen,sequence=0,post;
const calls=[],navigations=[],planReads=[];
let planScreen,renderedPlan;
const planSource=(await readFile(new URL('./web/js/screens/plan.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const summary={payoff_date:'2027-01-15',months:12,interest_minor:100,fees_minor:0,cost_minor:100,peak_required_minor:1000,cost_delta_minor:0,months_delta:0};
const report={currency:'AMD',currency_exponent:2,active_revision:7,rows:[{strategy:'proposed',proposal:'base',summary},{strategy:'highest_rate',proposal:'selected-policy',summary},{strategy:'utilisation',refusal:'unsupported',summary:null}]};
const response=body=>({ok:true,status:200,json:async()=>body});
const env={Number,Map,JSON,Error,encodeURIComponent,crypto:{randomUUID:()=>`stable-key-${++sequence}`},
 document:{getElementById:field,querySelectorAll:()=>[]},register:s=>{screen=s;},currentScreen:()=> 'plan-comparison',go:(id,params)=>{navigations.push({id,params});planScreen.onShow(null,params);},addStrings(){},planEvidence:()=>'<details>evidence</details>',T:key=>key,esc:String,fmtMoney:String,fmtDate:String,invalidate(){},
 api:async(path,init={})=>{calls.push({path,body:init.body?JSON.parse(init.body):undefined});if(path==='api/plan')return response({proposal:'base'});if(path.startsWith('api/plan/comparisons?'))return response(report);return post();}};
const selectedSheet={proposal:'selected-policy',approved:true,currency:'AMD',currency_exponent:2,goal:'least_interest',as_of:'2026-01-15',summary:{...summary,payoff_date:'2027-03-15',interest_minor:250,strength:'named_strategies_only'},months:[]};
const optimizedSheet={...selectedSheet,proposal:'optimized-winner',summary:{...selectedSheet.summary,payoff_date:'2027-01-15',interest_minor:100}};
vm.runInNewContext(planSource,{...env,register:s=>{planScreen=s;},URLSearchParams,location:{search:'?goal=soonest'},chartHTML:'',drawChart:d=>{renderedPlan=d;},fmtMonth:String,sub:key=>key,
 document:{...env.document,querySelector:()=>null,createElement:()=>({append(){},innerHTML:''})},
 getJSON:async(path,onData)=>{planReads.push(path);const body=path==='api/plan'?selectedSheet:optimizedSheet;onData?.(body);return body;}});
vm.runInNewContext(source,env);
const root=field('root');screen.onMount(root);
await screen.onShow();
assert.deepEqual(calls.map(c=>c.path),['api/plan','api/plan/comparisons?proposal=base']);
assert.equal(field('comparison-activate').disabled,true,'preview alone cannot activate');
assert.match(field('comparison-rows').innerHTML,/value="2"\s+disabled/,'refusal must be unselectable');
root.change({target:{name:'comparison-method',value:'1'}});
post=async()=>{throw new TypeError('response lost');};
await field('comparison-activate').click();
const first=calls.at(-1).body;
assert.equal(first.proposal,'selected-policy','must activate selected candidate, not the original proposal');
assert.equal(first.expected_revision,7);
post=async()=>response({revision:8});
await field('comparison-activate').click();
assert.deepEqual(calls.at(-1).body,first,'uncertain activation must retain exact request identity');
assert.equal(field('comparison-message').textContent,'compare.activated');
assert.equal(navigations.length,1);
assert.equal(navigations[0].id,'plan');
assert.equal(navigations[0].params.resetGoal,true);
assert.equal(planReads[0],'api/plan','reopening must clear the previous explicit goal');
assert.equal(renderedPlan.proposal,'selected-policy','the visible plan must retain the selected baseline');
assert.equal(field('pl-date').textContent,selectedSheet.summary.payoff_date);
assert.equal(field('pl-cost').textContent,'2.5');
assert.equal(field('pl-approved').hidden,false);
assert.equal(field('comparison-activate').disabled,true);
await screen.onShow();root.change({target:{name:'comparison-method',value:'1'}});
post=async()=>({ok:false,status:409});await field('comparison-activate').click();
assert.equal(field('comparison-activate').disabled,true,'stale comparison must be refreshed before activation');
assert.equal(field('comparison-error').textContent,'compare.stale');
assert.equal(navigations.length,1,'failed activation must not navigate');
console.log('Comparison activation preserves retries and reopens the selected baseline with a reset goal.');
