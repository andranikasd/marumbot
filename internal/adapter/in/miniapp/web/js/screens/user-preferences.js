"use strict";
import {api} from '../api.js';
import {addStrings,T} from '../i18n.js';
addStrings({
 'prefs.title':'Հիշեցումների կարգավորումներ','prefs.zone':'IANA ժամային գոտի','prefs.quiet':'Լուռ ժամեր','prefs.start':'Սկիզբ','prefs.end':'Ավարտ','prefs.required':'Պարտադիր վճարման հիշեցումները միացված են։ Լուռ ժամերին դրանք սպասում են։','prefs.save':'Պահպանել','prefs.saved':'Պահպանված է','prefs.error':'Պահպանումը հաստատված չէ։ Կրկին փորձեք։','prefs.invalid':'Ստուգեք ժամային գոտին և լուռ ժամերը։','prefs.conflict':'Կարգավորումները փոխվել են։ Վերաբացեք էջը։'
},{
 'prefs.title':'Reminder settings','prefs.zone':'IANA timezone','prefs.quiet':'Quiet hours','prefs.start':'Start','prefs.end':'End','prefs.required':'Required payment reminders are enabled. During quiet hours, delivery waits.','prefs.save':'Save settings','prefs.saved':'Saved','prefs.error':'Save unconfirmed. Retry to check the same request.','prefs.invalid':'Check the timezone and quiet hours.','prefs.conflict':'Settings changed. Reopen this page.'
});
export const preferencesHTML=`<form id="prefs-form" class="card stack"><h2 data-i18n="prefs.title"></h2><p data-i18n="prefs.required"></p><label for="prefs-zone" data-i18n="prefs.zone"></label><input id="prefs-zone" placeholder="Asia/Yerevan" required><label><input id="prefs-quiet" type="checkbox"><span data-i18n="prefs.quiet"></span></label><label for="prefs-start" data-i18n="prefs.start"></label><input id="prefs-start" type="time" required><label for="prefs-end" data-i18n="prefs.end"></label><input id="prefs-end" type="time" required><button id="prefs-save" data-i18n="prefs.save" disabled></button><p id="prefs-error" role="status"></p></form>`;
const $=id=>document.getElementById(id);
let saved,request,busy=false,generation=0;
const time=minutes=>String(Math.floor(minutes/60)).padStart(2,'0')+':'+String(minutes%60).padStart(2,'0');
const minutes=value=>{const[h,m]=value.split(':').map(Number);return h*60+m;};
function lock(value){for(const el of document.querySelectorAll('#prefs-form input'))el.disabled=value;}
function fill(p){$('prefs-zone').value=p.timezone;$('prefs-quiet').checked=p.quiet_enabled;$('prefs-start').value=time(p.quiet_start);$('prefs-end').value=time(p.quiet_end);}
export function mountPreferences(){ $('prefs-form').onsubmit=async e=>{
 e.preventDefault();if(busy||!saved)return;
 if(!request)request={timezone:$('prefs-zone').value.trim(),quiet_enabled:$('prefs-quiet').checked,quiet_start:minutes($('prefs-start').value),quiet_end:minutes($('prefs-end').value),version:saved.version,idempotency_key:crypto.randomUUID()};
 busy=true;lock(true);$('prefs-save').disabled=true;$('prefs-error').textContent='';
 try{const res=await api('api/settings/reminders',{method:'POST',body:JSON.stringify(request)});if(!res.ok){if([409,422].includes(res.status)){request=null;lock(false);}if(res.status===409)saved=null;throw new Error(res.status===409?'prefs.conflict':res.status===422?'prefs.invalid':'prefs.error');}saved=await res.json();request=null;fill(saved);lock(false);$('prefs-error').textContent=T('prefs.saved');}
 catch(err){$('prefs-error').textContent=T(['prefs.conflict','prefs.invalid'].includes(err.message)?err.message:'prefs.error');}
 finally{busy=false;$('prefs-save').disabled=!saved;}
};}
export async function showPreferences(){
 if(request||busy)return;const view=++generation;saved=null;lock(true);$('prefs-save').disabled=true;$('prefs-error').textContent='';
 try{const res=await api('api/settings/reminders');if(!res.ok)throw new Error();const p=await res.json();if(view!==generation)return;saved=p;fill(p);lock(false);$('prefs-save').disabled=false;}
 catch{if(view===generation)$('prefs-error').textContent=T('prefs.error');}
}
