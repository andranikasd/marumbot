import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const clean=s=>s.replace(/^import .*;$/gm,'').replaceAll('export ','');
const budgetSource=clean(await readFile(new URL('./web/js/screens/budget-edit.js',import.meta.url),'utf8'));
const policySource=clean(await readFile(new URL('./web/js/screens/budget-policy.js',import.meta.url),'utf8'));
const response=(status=200,body={})=>({status,ok:status>=200&&status<300,json:async()=>body});
function harness(kind,policyMode=false){
 const fields=new Map(),calls=[];let screen,current=kind==='policy'?'budget-policy':'budget-edit',post=async()=>response(),get=null,key=0;
 function field(id){if(!fields.has(id)){let value='';const f={id,disabled:false,hidden:false,readOnly:false,dataset:{},style:{},children:[],textContent:'',
 get value(){return value;},set value(v){value=String(v);},setAttribute(){},focus(){},append(...items){this.children.push(...items);},replaceChildren(...items){this.children=items;},addEventListener(type,fn){this[type]=fn;},
 querySelector(q){return field(id+q);},querySelectorAll(q){return q==='[data-section]'?['budget','funding','months'].map(section=>{const b=field('tab-'+section);b.dataset.section=section;return b;}):[];}};fields.set(id,f);}return fields.get(id);}
 const fundingData={monthly_minor:300000,spent_minor:2000,cash_through:'2026-09-03',events:[{on:'2026-10-01',minor:1000,expected:false,routing:{hold_until:'2026-10-02'}}]};
 const budget={version:7,today:'2026-09-03',currency:'AMD',currency_exponent:2,monthly_major:3000,base_monthly_major:3000,pay_day:5,opening_major:500,reserve_major:20,overrides:{},funding:fundingData};
 const policy={version:7,today:'2026-09-03',currency:'AMD',currency_exponent:2,monthly_minor:300000,explicit_funding:true,policies:policyMode?[{version:7,effective_from:'2026-09-01',monthly_minor:300000,carry_rule:'carry_cash',released_payment_rule:'roll_all'}]:[]};
 const env={Map,Set,Date,Number,JSON,Error,Math,BigInt,crypto:{randomUUID:()=>`key-${++key}`},document:{getElementById:field,createElement:()=>field('created-'+fields.size)},
 register:s=>{screen=s;},currentScreen:()=>current,go:id=>{current=id;},addStrings(){},T:k=>k,sub:k=>k,fmtMoney:String,haptic:{bad(){},tap(){},ok(){}},toast(){},invalidate(){},budgetHelpHTML:"",fundingHTML:'',
 majorAmount:v=>Number(v),minorAmount:v=>Number(v)*100,minorText:(n,e)=>String(n/10**e),validMonth:v=>/^\d{4}-\d\d$/.test(v),validDate:v=>typeof v==='string'&&/^\d{4}-\d\d-\d\d$/.test(v),
 createFunding:()=>({load(){field('funding-mode').value='separate';},read:()=>({ok:true,value:fundingData})}),
 api:async(path,init={})=>{calls.push({path,body:init.body});if(init.method==='POST')return post(path,init);if(get)return get(path);return response(200,path==='api/budget'?budget:path==='api/loans'?{loans:[]}:policy);}};
 vm.createContext(env);vm.runInContext(kind==='policy'?policySource:budgetSource,env);screen.onMount();
 return {field,calls,screen,env,budget,fundingData,policy,setPost:f=>post=f,setGet:f=>get=f,setCurrent:c=>current=c,getKey:()=>key,submit:()=>kind==='policy'?field('bp-form').onsubmit({preventDefault(){}}):field('budget-form').submit({preventDefault(){}}),retry:()=>field(kind==='policy'?'bp-retry':'budget-save-retry').onclick({preventDefault(){}}),reload:()=>field(kind==='policy'?'bp-reload':'budget-reload').onclick()};
}
for(const mode of ['configuration','funding','policy']){
 const isPolicy=mode==='policy',h=harness(isPolicy?'policy':'budget',mode==='funding'),prefix=isPolicy?'bp-':'budget-',screen=isPolicy?'budget-policy':'budget-edit';
 await h.screen.onShow(null,{section:'funding'});
 const html=h.screen.html;
 assert.ok(html.indexOf(`id="${isPolicy?"bp-retry":"budget-save-retry"}"`)<html.indexOf(`id="${prefix}fields"`),'retry must be outside the disabled fieldset');
 if(!isPolicy)assert.equal(h.field('budget-panel-funding').hidden,false,'funding deep link uses the navigator second parameter');
 // Start a save without marking the form dirty: navigation still must not reload it.
 let rejectPost;h.setPost(()=>new Promise((resolve,reject)=>{rejectPost=reject;}));const saving=h.submit();
 assert.equal(h.field(prefix+'fields').disabled,true,'freeze inputs while request is in flight');assert.equal(h.field((isPolicy?'bp-retry':'budget-save-retry')).disabled,true);
 assert.equal(h.field(prefix+'reload').disabled,true);const initial=h.calls.at(-1);const original=JSON.parse(initial.body);
 assert.ok(original.idempotency_key);assert.equal(original.expected_version,7);
 assert.equal(initial.path,isPolicy?'api/budget/policies':mode==='funding'?'api/budget/funding':'api/budget');
 if(mode==='configuration')assert.equal(original.as_of,'2026-09-03');else assert.equal(original.as_of,undefined);
 h.setCurrent('loans');await h.screen.onShow();rejectPost(new TypeError('response lost'));await saving;
 assert.equal(h.field(prefix+'fields').disabled,true,'uncertain outcome must keep fields frozen');assert.equal(h.field((isPolicy?'bp-retry':'budget-save-retry')).hidden,false);assert.equal(h.field((isPolicy?'bp-retry':'budget-save-retry')).disabled,false);
 const count=h.calls.length;h.setCurrent(screen);await h.screen.onShow(null,{section:'funding'});await h.reload();
 assert.equal(h.calls.length,count,'navigation and forced reload must not discard pending request');
 // Even externally changed values and next business date cannot change the retry.
 h.field(isPolicy?'bp-limit':'monthly').value='9999';h.fundingData.events[0].minor=99999;h.budget.today='2026-09-04';h.policy.today='2026-09-04';
 h.setPost(async()=>response(503));await h.retry();assert.equal(h.calls.at(-1).body,initial.body);assert.equal(h.field(prefix+'fields').disabled,true);
 h.setPost(async()=>response());await h.retry();assert.equal(h.calls.at(-1).body,initial.body,'retry must be byte-for-byte identical');assert.equal(h.getKey(),1);assert.equal(h.field((isPolicy?'bp-retry':'budget-save-retry')).hidden,true);
 // A definitive refusal permits a corrected request with a new identity.
 h.setCurrent(screen);await h.reload();h.setPost(async()=>response(422));await h.submit();const rejected=JSON.parse(h.calls.at(-1).body);
 assert.equal(h.field(prefix+'fields').disabled,false,'definitive validation refusal unlocks edit');assert.equal(h.field((isPolicy?'bp-retry':'budget-save-retry')).hidden,true);assert.equal(h.field(prefix+'reload').disabled,false);
 const gets=h.calls.length;await h.screen.onShow();assert.equal(h.calls.length,gets,'rejected draft should survive navigation');
 h.field(isPolicy?'bp-limit':'monthly').value='4000';h.setPost(async()=>response(409));await h.submit();assert.notEqual(JSON.parse(h.calls.at(-1).body).idempotency_key,rejected.idempotency_key);
 assert.equal(h.field(prefix+'reload').disabled,false,'conflict permits explicit reload');const before=h.calls.length;await h.screen.onShow();assert.equal(h.calls.length,before,'conflict must not auto-discard draft');await h.reload();assert.ok(h.calls.length>before);
 console.log(mode+': exact key/payload/date survives lost response, 503 and navigation; definitive4xx permits correction/reload.');
}
const h=harness('policy');await h.screen.onShow();h.setPost(async()=>response(422,{error:'unsupported',reason:'until_goal_then_release'}));await h.submit();assert.equal(h.field('bp-status').textContent,'bp.goalUnsupported');
assert.match(h.screen.html,/data-i18n="bp.retry"/);assert.match(h.screen.html,/data-i18n="bp.goalUnsupported"/);

// No response is needed before the other independent context reads start.
const parallel=harness('budget'), resolves=new Map();
parallel.setGet(path=>new Promise(resolve=>resolves.set(path,resolve)));
const loading=parallel.screen.onShow();
assert.deepEqual([...resolves.keys()],['api/budget','api/budget/policies','api/loans']);
assert.equal(parallel.field('budget-fields').disabled,true);
resolves.get('api/loans')(response(200,{loans:[]}));
resolves.get('api/budget/policies')(response(200,parallel.policy));
assert.equal(parallel.field('budget-fields').disabled,true,'context alone cannot unlock the financial form');
resolves.get('api/budget')(response(200,parallel.budget));
await loading;
assert.equal(parallel.field('budget-fields').disabled,false);
parallel.field('budget-next').onclick();
assert.equal(parallel.field('budget-panel-funding').hidden,false);
assert.equal(parallel.field('budget-save').hidden,false);
console.log('Budget context starts in one request wave; fields unlock only after complete loading.');

const changedContext=harness('budget');changedContext.policy.version++;
await changedContext.screen.onShow();
assert.equal(changedContext.field('budget-fields').disabled,true,'concurrent reads of different budget revisions must not enable saving');
assert.equal(changedContext.field('budget-status').textContent,'be.load');
