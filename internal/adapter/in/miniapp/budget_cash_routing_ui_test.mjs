import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const clean=s=>s.replace(/^import .*;\n/gm,'').replace(/^export /gm,'');
const env={T:k=>k,addStrings:()=>{},crypto:{randomUUID:()=> 'cash-event-id'}};
vm.createContext(env);
vm.runInContext(clean(await readFile(new URL('./web/js/screens/budget-funding.js',import.meta.url),'utf8')),env);
vm.runInContext(clean(await readFile(new URL('./web/js/screens/budget-cash-routing.js',import.meta.url),'utf8')),env);
const parse=(text,options)=>env.minorAmount(text,2,options);
const eligible=new Set(['loan-a','loan-b']);
const base={minor:10001,on:'2026-09-03',expected:false};
const read=value=>env.routingValue({mode:'pool',loanID:'',splits:[],until:'',minimum:'',fromOpening:false,...value},base,'2026-09-03',parse,eligible);
assert.deepEqual(JSON.parse(JSON.stringify(read({mode:'split',splits:[{loanID:'loan-a',amount:'33.33'},{loanID:'loan-b',amount:'66.68'}],until:'2026-09-10',minimum:'120.50',fromOpening:true}))),{
 routing:{splits:[{loan_id:'loan-a',minor:3333},{loan_id:'loan-b',minor:6668}],hold_until:'2026-09-10',hold_minimum_minor:12050},from_opening:true,
});
assert.equal(read({mode:'loan',loanID:'loan-b'}).routing.loan_id,'loan-b');
assert.equal(read({mode:'hold',until:'2026-09-10'}).routing.hold_until,'2026-09-10');
for(const value of [
 {mode:'loan',loanID:'foreign-loan'},
 {mode:'split',splits:[{loanID:'loan-a',amount:'100'}]},
 {mode:'split',splits:[{loanID:'loan-a',amount:'33.33'},{loanID:'loan-a',amount:'66.68'}]},
 {mode:'hold'}, {mode:'hold',until:'2026-09-02'}, {mode:'hold',until:'2026-09-31'},
 {mode:'hold',minimum:'0'}, {mode:'pool',fromOpening:true},
]) assert.throws(()=>read(value));
base.expected=true;
assert.throws(()=>read({mode:'loan',loanID:'loan-a',fromOpening:true}));
base.expected=false;base.on='2026-09-04';
assert.throws(()=>read({mode:'loan',loanID:'loan-a',fromOpening:true}));
assert.equal(env.minorAmount('0',2),0,'zero funding is an explicit valid declaration');
assert.throws(()=>env.minorAmount('',2),'blank is not declared zero');
console.log('Cash routing: exact split/earmark/AND hold/retained declarations and invalid targets/dates checked.');
