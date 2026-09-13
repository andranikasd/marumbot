import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/loans.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map(),reads=[];
const field=id=>{if(!fields.has(id))fields.set(id,{hidden:true,children:[],textContent:'',style:{}});return fields.get(id);};
const env={document:{getElementById:field},register(){},addStrings(){},T:x=>x,
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
