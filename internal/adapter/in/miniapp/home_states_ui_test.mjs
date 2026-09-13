import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/home.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const root={innerHTML:'',attrs:{},setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];}};
let screen;const reads=[],translations={};
const env={register:s=>screen=s,document:{getElementById:()=>root},addStrings(hy,en){for(const key of Object.keys(en)){assert.ok(hy[key],key+' needs Armenian');assert.ok(en[key],key+' needs English');translations[key]=en[key];}},T:k=>k,esc:String,icon:()=>'',fmtMoney:String,fmtFull:String,sub:k=>k,
 getJSON:(path,callback)=>new Promise((resolve,reject)=>reads.push({path,resolve:body=>{try{callback(body);resolve();}catch(e){reject(e);}},reject}))};
vm.createContext(env);vm.runInContext(source,env);
function resolve(path,body){const index=reads.findIndex(r=>r.path===path);assert.ok(index>=0,path);reads.splice(index,1)[0].resolve(body);}
function reject(path){const index=reads.findIndex(r=>r.path===path);assert.ok(index>=0,path);reads.splice(index,1)[0].reject(new Error('offline'));}
async function show(loans,budget={},today='2026-09-15'){
 const pending=screen.onShow();assert.equal(root.attrs['aria-busy'],'true');assert.match(root.innerHTML,/loading/);
 resolve('api/loans',{loans,today});resolve('api/budget',budget);await pending;assert.equal(root.attrs['aria-busy'],undefined);
}
const active={id:'loan-a',name:'A loan',balance_major:100,currency:'USD'};
await show([]);assert.match(root.innerHTML,/home.start/);assert.match(root.innerHTML,/class="cta" data-go="add"/);assert.match(root.innerHTML,/data-go="budget-edit"/);assert.doesNotMatch(root.innerHTML,/home.none/);
await show([active]);assert.match(root.innerHTML,/home.step.loan.*home.saved/);assert.match(root.innerHTML,/class="cta" data-go="budget-edit"/);assert.match(root.innerHTML,/home.none.help/);
await show([], {monthly_major:0});assert.match(root.innerHTML,/data-go="add"/);assert.doesNotMatch(root.innerHTML,/data-go="budget-edit"/);
await show([{balance_major:0}],{monthly_major:100});assert.match(root.innerHTML,/home.settled/);assert.doesNotMatch(root.innerHTML,/home.empty/);assert.match(root.innerHTML,/class="cta" data-go="loans"/);assert.doesNotMatch(root.innerHTML,/home.view.plan/);
await show([{...active,needs_reconciliation:true}],{monthly_major:100});assert.match(root.innerHTML,/class="cta" data-go="activity"/);assert.match(root.innerHTML,/home.review.help/);assert.doesNotMatch(root.innerHTML,/home.settled/);
await show([{...active,next_due:'2026-10-15',next_payment_major:10}],{monthly_major:100});assert.doesNotMatch(root.innerHTML,/home.start|home.date.passed/);assert.match(root.innerHTML,/home.view.plan/);assert.match(root.innerHTML,/class="cta" data-go="loan" data-arg="loan-a"/);
await show([{...active,next_due:'2026-09-01',next_payment_major:10}],{monthly_major:100});assert.match(root.innerHTML,/home.date.passed/);
await show([{...active,next_due:'2026-10-15'}],{monthly_major:100});assert.match(root.innerHTML,/home.none.help/);assert.doesNotMatch(root.innerHTML,/undefined/);
await show([{...active,next_due:'2026-10-15',next_payment_major:10,needs_reconciliation:true}],{monthly_major:100});assert.doesNotMatch(root.innerHTML,/home.next/);
const budgetFail=screen.onShow();resolve('api/loans',{loans:[active]});reject('api/budget');await budgetFail;assert.match(root.innerHTML,/home.money.failed/);assert.match(root.innerHTML,/A loan/);assert.doesNotMatch(root.innerHTML,/home.step.money|data-go="budget-edit"/);
const loanFail=screen.onShow();reject('api/loans');resolve('api/budget',{});await loanFail;assert.match(root.innerHTML,/err.load/);assert.doesNotMatch(root.innerHTML,/home.empty/);
const malformed=screen.onShow();resolve('api/loans',{});resolve('api/budget',{});await malformed;assert.match(root.innerHTML,/err.load/);
const first=screen.onShow(),second=screen.onShow();
reads.splice(2,2).forEach(r=>r.resolve(r.path==='api/loans'?{loans:[]}:{}));await second;
const html=root.innerHTML;reads.splice(0).forEach(r=>r.reject(new Error('late')));await first;assert.equal(root.innerHTML,html);assert.equal(root.attrs['aria-busy'],undefined);
for(const key of source.matchAll(/T\('(home\.[^']+)'\)/g))assert.ok(translations[key[1]],key[1]+' translation');
console.log('Home guides new users, preserves partial errors, and distinguishes repaid, incomplete, overdue and unconfirmed loans.');
