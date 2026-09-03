import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/plan-scenarios.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'').replace('export function scenarioMinor','function scenarioMinor');
const fields=new Map();
function field(id){if(!fields.has(id))fields.set(id,{disabled:false,hidden:false,value:'',textContent:'',innerHTML:'',checked:false,reset(){},addEventListener(type,fn){this[type]=fn;}});return fields.get(id);}
let screen,seq=0,loseResponse=false;
const calls=[];
const sheet={proposal:'source',currency:'AMD',currency_exponent:2,as_of:'2026-01-15',summary:{payoff_date:'2027-01-15',interest_minor:100}};
const view={id:'a'.repeat(64),result_hash:'b'.repeat(64),revision:5,sheet,changes:{},outdated:false};
const env={Number,BigInt,JSON,Error,encodeURIComponent,crypto:{randomUUID:()=>`key-${++seq}`},
 document:{getElementById:field,querySelectorAll:()=>[]},register:s=>{screen=s;},addStrings(){},planEvidence:()=>"<details>evidence</details>",T:k=>k,esc:String,fmtMoney:String,fmtDate:String,invalidate(){},
 api:async(path,init={})=>{const body=init.body?JSON.parse(init.body):undefined;calls.push({path,body});if(path.endsWith('/activate')&&loseResponse)throw new TypeError('lost');const data=path==='api/plan'?sheet:path==='api/scenarios'&&!body?{scenarios:[]}:view;return {ok:true,status:200,json:async()=>data};}};
vm.runInNewContext(source,env);
assert.equal(vm.runInNewContext('scenarioMinor("123.45",2)',env),12345);
assert.throws(()=>vm.runInNewContext('scenarioMinor("9007199254740992",0)',env));
assert.throws(()=>vm.runInNewContext('scenarioMinor("1.001",2)',env));
const root=field('root');screen.onMount(root);await screen.onShow();
field('sc-monthly').value='100.25';await field('sc-form').onsubmit({preventDefault(){}});
assert.equal(calls.at(-1).path,'api/scenarios/preview');assert.equal(calls.at(-1).body.changes.monthly_minor,10025);
assert.equal(field('sc-activate').hidden,true,'preview must not activate');assert.equal(field('sc-save').hidden,false);
await field('sc-save').onclick();assert.equal(field('sc-activate').hidden,false);
const save=calls.find(c=>c.path==='api/scenarios'&&c.body);assert.equal(save.body.result_hash,view.result_hash,'save must bind reviewed result');
loseResponse=true;await field('sc-activate').onclick();const first=calls.at(-1).body;
assert.equal(first.expected_revision,5);assert.equal(first.id,view.id);
loseResponse=false;await field('sc-activate').onclick();assert.deepEqual(calls.at(-1).body,first,'retry identity must survive response loss');assert.equal(field('sc-status').textContent,'sc.done');
field('sc-form').input();assert.equal(field('sc-activate').hidden,true,'edits invalidate prior calculation');assert.equal(field('sc-save').hidden,true);
console.log('Scenario preview/save/activation, decimal input and uncertain retry verified.');
