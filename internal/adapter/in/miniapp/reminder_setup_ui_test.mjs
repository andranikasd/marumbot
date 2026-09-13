import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/reminder-setup.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'');
function setup({permission=true,allowed=true}={}){
 const fields=new Map(),calls=[];let screen,id=0,active='reminder-setup',permissionCalls=0;
 const node=key=>{if(!fields.has(key))fields.set(key,{value:'',hidden:false,disabled:false,textContent:'',checked:false,addEventListener(name,fn){this[name]=fn;}});return fields.get(key);};
 const env={document:{getElementById:node,querySelectorAll:()=>['notify-enabled','notify-lead','notify-time'].map(node)},register:s=>screen=s,addStrings(){},T:k=>k,sub:(k,p)=>k+':'+p.days,crypto:{randomUUID:()=>String(++id)},setTimeout,clearTimeout,currentScreen:()=>active,go:target=>calls.push({go:target}),tg:{initDataUnsafe:{user:{allows_write_to_pm:permission}},requestWriteAccess:fn=>{permissionCalls++;fn(allowed);}},api:(path,options)=>new Promise((resolve,reject)=>calls.push({path,options,resolve,reject}))};
 vm.createContext(env);vm.runInContext(source,env);screen.onMount();return {node,calls,screen,setActive:s=>active=s,get permissions(){return permissionCalls;}};
}
const prefs={version:2,timezone:'Asia/Yerevan',reminders_enabled:false,reminder_lead_days:1,reminder_minute:540,quiet_enabled:true,quiet_start:1320,quiet_end:480};
async function load(s,p=prefs){const wait=s.screen.onShow();s.calls.shift().resolve({ok:true,json:async()=>p});await wait;}
const s=setup();await load(s);assert.equal(s.node('notify-time').value,'09:00');assert.equal(s.node('notify-zone').textContent,'Asia/Yerevan');assert.equal(s.node('notify-time').disabled,true,'timing is optional when reminders are off');
s.node('notify-enabled').checked=true;s.node('notify-form').change();s.node('notify-time').value='08:30';s.node('notify-form').input();
await s.screen.onShow();assert.equal(s.calls.length,0);assert.equal(s.node('notify-time').value,'08:30','returning keeps edits');
let wait=s.node('notify-form').onsubmit();await Promise.resolve();let write=s.calls.shift(),body=write.options.body;
assert.equal(JSON.parse(body).reminder_minute,510);assert.equal(JSON.parse(body).quiet_start,1320,'hidden quiet-hour source stays intact');
write.reject(new Error('lost'));await wait;assert.equal(s.node('notify-save').textContent,'retry');assert.equal(s.node('notify-time').disabled,true);
wait=s.node('notify-form').onsubmit();write=s.calls.shift();assert.equal(write.options.body,body,'retry uses identical command');s.setActive('loans');write.resolve({ok:true,json:async()=>({...prefs,reminders_enabled:true,version:3,reminder_minute:510})});await wait;assert.equal(s.calls.length,0,'late save never steals navigation');
const denied=setup({permission:false,allowed:false});await load(denied);denied.node('notify-enabled').checked=true;await denied.node('notify-form').onsubmit();assert.equal(denied.permissions,1);assert.equal(denied.calls.length,0,'denied permission writes no enabled preference');assert.equal(denied.node('notify-error').textContent,'notify.permission');assert.equal(denied.node('notify-save').disabled,false);
const off=setup({permission:false});await load(off,{...prefs,reminders_enabled:true});off.node('notify-enabled').checked=false;off.node('notify-time').value='';wait=off.node('notify-form').onsubmit();write=off.calls.shift();assert.equal(JSON.parse(write.options.body).reminders_enabled,false);assert.equal(off.permissions,0,'opt-out never asks for permission');write.resolve({ok:true,json:async()=>prefs});await wait;assert.equal(off.calls.shift().go,'simple-plan');
const invalid=setup();await load(invalid);invalid.node('notify-enabled').checked=true;invalid.node('notify-time').value='24:00';await invalid.node('notify-form').onsubmit();assert.equal(invalid.calls.length,0);assert.equal(invalid.node('notify-error').textContent,'notify.invalid');
const conflict=setup();await load(conflict);wait=conflict.node('notify-form').onsubmit();conflict.calls.shift().resolve({ok:false,status:409});await wait;assert.equal(conflict.node('notify-save').disabled,true);assert.equal(conflict.node('notify-retry').hidden,false);wait=conflict.node('notify-retry').onclick();conflict.calls.shift().resolve({ok:true,json:async()=>({...prefs,version:4})});await wait;assert.equal(conflict.node('notify-save').disabled,false);
const failed=setup();wait=failed.screen.onShow();failed.calls.shift().reject(new Error('offline'));await wait;assert.equal(failed.node('notify-retry').hidden,false);assert.equal(failed.node('notify-save').disabled,true);
console.log('Reminder setup: permission denial, opt-out, time validation, timezone, retained drafts, identical uncertain retries, conflicts and late navigation.');

for(const missing of ['omitted','null'])for(const enabled of [false,true]){
 const initial={...prefs,reminders_enabled:enabled};
 if(missing==='omitted'){delete initial.reminder_lead_days;delete initial.reminder_minute;}
 else{initial.reminder_lead_days=null;initial.reminder_minute=null;}
 const app=setup();await load(app,initial);
 assert.equal(app.node('notify-save').disabled,false,`${missing} timing is valid for enabled=${enabled}`);
 assert.equal(app.node('notify-lead').value,'1');assert.equal(app.node('notify-time').value,'09:00');
 app.node('notify-enabled').checked=false;
 const waiting=app.node('notify-form').onsubmit();const command=app.calls.shift();const payload=JSON.parse(command.options.body);
 if(missing==='omitted'){assert.equal(Object.hasOwn(payload,'reminder_lead_days'),false);assert.equal(Object.hasOwn(payload,'reminder_minute'),false);}
 else{assert.equal(payload.reminder_lead_days,null);assert.equal(payload.reminder_minute,null);}
 command.resolve({ok:true,json:async()=>({...initial,reminders_enabled:false})});await waiting;
}
{
 const initial={...prefs};delete initial.reminder_lead_days;delete initial.reminder_minute;
 const app=setup();await load(app,initial);app.node('notify-enabled').checked=true;
 const waiting=app.node('notify-form').onsubmit();await Promise.resolve();const command=app.calls.shift();const payload=JSON.parse(command.options.body);
 assert.equal(payload.reminder_lead_days,1);assert.equal(payload.reminder_minute,540,'enabling saves the explicitly displayed paired defaults');
 command.resolve({ok:true,json:async()=>({...prefs,reminders_enabled:true})});await waiting;
}
{
 const app=setup();await load(app,{...prefs,reminders_enabled:true,reminder_lead_days:7});
 assert.equal(app.node('notify-custom').hidden,false);assert.equal(app.node('notify-custom').textContent,'notify.days:7');assert.equal(app.node('notify-lead').value,'7');
 const waiting=app.node('notify-form').onsubmit();const command=app.calls.shift();assert.equal(JSON.parse(command.options.body).reminder_lead_days,7);
 command.resolve({ok:true,json:async()=>({...prefs,reminders_enabled:true,reminder_lead_days:7})});await waiting;
}
console.log('Reminder source compatibility: omitted/null timing, legacy enabled, display-only defaults, opt-out preservation, paired opt-in and custom seven-day lead.');
