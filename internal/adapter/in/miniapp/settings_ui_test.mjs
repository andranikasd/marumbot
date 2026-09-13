import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/more.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
const fields=new Map();
const field=id=>{if(!fields.has(id))fields.set(id,{value:'hy',disabled:false,textContent:'',addEventListener(name,handler){this[name]=handler;}});return fields.get(id);};
let screen,fail=false,refreshes=0;
const env={mountPreferences(){},showPreferences(){},preferencesHTML:'',register:value=>screen=value,
 addStrings(){},T:key=>key,icon:()=>'',lang:'hy',setLanguage:value=>{env.lang=value;},refreshLanguage:()=>refreshes++,
 document:{getElementById:field},api:async(path,request)=>{assert.equal(path,'api/settings');assert.equal(request.method,'POST');return {ok:!fail,json:async()=>JSON.parse(request.body)};}};
vm.createContext(env);vm.runInContext(source,env);screen.onMount();screen.onShow();
const select=field('settings-language');select.value='en';await select.change();
assert.equal(env.lang,'en');assert.equal(select.disabled,false);assert.equal(refreshes,1);
fail=true;select.value='hy';await select.change();
assert.equal(env.lang,'en','failed server save must not claim a persisted language change');
assert.equal(select.value,'en');assert.equal(select.disabled,false);assert.equal(field('settings-error').textContent,'err.save');
fail=false;select.value='hy';await select.change();
assert.equal(env.lang,'hy');assert.equal(field('settings-error').textContent,'');
console.log('Language saves switch the UI; failures retain the saved choice and allow retry.');

let complete;
env.api=()=>new Promise(resolve=>{complete=resolve;});
select.value='en';const saving=select.change();
screen.onShow();assert.equal(select.value,'en','revisiting More must retain the in-flight choice');
complete({ok:true,json:async()=>({locale:'en'})});await saving;
assert.equal(env.lang,'en');assert.equal(select.value,'en');assert.equal(select.disabled,false);
env.api=async()=>({ok:true,json:async()=>({locale:'unsupported'})});
select.value='hy';await select.change();
assert.equal(env.lang,'en');assert.equal(select.value,'en');assert.equal(field('settings-error').textContent,'err.save');
