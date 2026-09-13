import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/home.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const root={innerHTML:'',attrs:{},setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];}};
let screen;const reads=[];
const env={register:s=>screen=s,document:{getElementById:()=>root},addStrings(){},T:k=>k,esc:String,icon:()=>'',fmtMoney:String,fmtFull:String,sub:k=>k,
 getJSON:(_path,callback)=>new Promise((resolve,reject)=>reads.push({resolve:body=>{callback(body);resolve();},reject}))};
vm.createContext(env);vm.runInContext(source,env);
async function show(loans){const pending=screen.onShow();assert.equal(root.attrs['aria-busy'],'true');assert.match(root.innerHTML,/loading/);reads.shift().resolve({loans});await pending;assert.equal(root.attrs['aria-busy'],undefined);}
await show([]);assert.match(root.innerHTML,/home.empty/);assert.match(root.innerHTML,/class="cta" data-go="add"/);
await show([{balance_major:0}]);assert.match(root.innerHTML,/home.settled/);assert.doesNotMatch(root.innerHTML,/home.empty/);assert.match(root.innerHTML,/class="cta" data-go="loans"/);
await show([{balance_major:100}]);assert.match(root.innerHTML,/home.none/);assert.match(root.innerHTML,/class="cta" data-go="loans"/);
await show([{balance_major:100,needs_reconciliation:true}]);assert.match(root.innerHTML,/class="cta" data-go="activity"/);assert.doesNotMatch(root.innerHTML,/home.settled/);
const first=screen.onShow(),second=screen.onShow();reads.pop().resolve({loans:[]});await second;const html=root.innerHTML;reads.shift().reject(new Error('late'));await first;assert.equal(root.innerHTML,html);
const failed=screen.onShow();reads.shift().reject(new Error('offline'));await failed;assert.match(root.innerHTML,/err.load/);assert.doesNotMatch(root.innerHTML,/home.empty/);assert.equal(root.attrs['aria-busy'],undefined);
console.log('Home distinguishes onboarding, repaid loans, no due date, reconciliation and failed reads.');
