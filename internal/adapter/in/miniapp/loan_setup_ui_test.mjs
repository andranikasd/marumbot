import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
import {extraMinor,extraText} from './web/js/screens/projection-format.js';
const source=(await readFile(new URL('./web/js/screens/loan-setup.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
function setup(){
 const fields=new Map(),calls=[];let screen,pending=null;const node=id=>{if(!fields.has(id))fields.set(id,{value:'',checked:false,hidden:false,disabled:false,textContent:'',children:[],replaceChildren(){this.children=[];},append(...n){this.children.push(...n);},addEventListener(name,fn){this[name]=fn;}});return fields.get(id);};
 const env={document:{getElementById:node,createElement:()=>({append(){}})},register:s=>screen=s,addStrings(a,b){assert.deepEqual(Object.keys(a).sort(),Object.keys(b).sort());},T:k=>k,sub:(k,p)=>`${k}:${p.step}`,extraMinor,extraText,num:Number,fmtMoney:(n,c)=>n+c,fmtDate:d=>d,haptic:{ok(){}},invalidate(){},currentScreen:()=> 'loan-setup',go:(id,params)=>calls.push({go:id,params}),api:()=>Promise.resolve({ok:true,json:async()=>({today:'2026-09-14'})}),loanMutation:(path,method,body)=>{pending=body;return new Promise((resolve,reject)=>calls.push({path,method,body,resolve:r=>{if(r.ok||r.status<500)pending=null;resolve(r);},reject}));},showLoanRetry(button,path,done){button.hidden=!pending;button.onclick=async()=>{const r=await env.loanMutation(path,'POST',pending);if(r.ok)done();};}};
 vm.createContext(env);vm.runInContext(source,env);screen.onMount();return {node:id=>node('ls-'+id),calls,screen};
}
const s=setup();await s.screen.onShow();assert.equal(s.node('fields').disabled,false);
const submit=()=>s.node('form').submit({preventDefault(){}});
await submit();assert.equal(s.node('step-0').hidden,false,'blank amounts rejected');
for(const [id,value] of Object.entries({original:'1000',remaining:'700',currency:'USD',start:'2026-01-15',end:'2027-01-15',payment:'100',due:'2026-10-15',method:'annuity'}))s.node(id).value=value;
await submit();assert.equal(s.node('step-1').hidden,false);s.screen.onLanguage();assert.equal(s.node('remaining').value,'700','language preserves draft');
await submit();await submit();assert.equal(s.node('step-3').hidden,false);assert.equal(s.node('limited').hidden,false,'unknown rate has limited claim');
await submit();assert.equal(s.calls.length,0,'bank confirmation required');s.node('confirm').checked=true;
let work=submit();let call=s.calls.shift();assert.equal(call.body.rate_percent,null);assert.equal(call.body.remaining_major,'700.00');assert.equal(call.body.next_due_date,'2026-10-15');const first=JSON.stringify(call.body);call.reject(new TypeError('lost response'));await work;
assert.equal(s.node('fields').disabled,true);await s.screen.onShow();assert.equal(s.node('remaining').value,'700');
work=s.node('retry').onclick();call=s.calls.shift();assert.equal(JSON.stringify(call.body),first);call.resolve({ok:true,status:201});await work;
assert.equal(s.calls.shift().go,'loans');assert.equal(s.node('remaining').value,'');
console.log('Loan setup: four prompts, language draft, bank confirmation, unknown-rate disclosure, paid-month date, exact lost-response retry.');
const bank=setup();await bank.screen.onShow();
for(const [id,value] of Object.entries({original:'1000',remaining:'700',currency:'USD',start:'2026-01-15',end:'2027-01-15',payment:'100',due:'2026-10-15',method:'annuity',accrued:'0'}))bank.node(id).value=value;
const advance=()=>bank.node('form').submit({preventDefault(){}});await advance();await advance();await advance();bank.node('confirm').checked=true;
work=advance();call=bank.calls.shift();assert.equal(call.body.accrued_interest_major,'0.00','explicit bank zero differs from unknown');call.resolve({ok:true,status:201});await work;
const settled=setup();await settled.screen.onShow();
for(const [id,value] of Object.entries({original:'1000',remaining:'0',currency:'USD',start:'2026-01-15',end:'2027-01-15',accrued:'1'}))settled.node(id).value=value;
const stepSettled=()=>settled.node('form').submit({preventDefault(){}});await stepSettled();await stepSettled();await stepSettled();assert.equal(settled.node('step-2').hidden,false,'interest-only debt cannot be submitted as repaid');
