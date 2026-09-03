"use strict";
import {register,go,currentScreen} from '../nav.js';
import {api} from '../api.js';
import {addStrings,T} from '../i18n.js';
addStrings({
 'reminder.required':'Պարտադիր վճարման հիշեցում','reminder.due':'Պայմանագրային վճարման օրը','reminder.delivery':'Հիշեցման ժամը','reminder.notice':'Հետաձգվում է միայն հիշեցումը։ Վճարման օրը չի փոխվում։','reminder.snooze':'Հիշեցնել ավելի ուշ','reminder.until':'Հիշեցնել այս ժամին (սարքի ժամային գոտով, մինչև 7 օր)','reminder.pay':'Գրանցել վճարումը','reminder.saved':'Հիշեցումը հետաձգված է','reminder.failed':'Չհաջողվեց հաստատել։ Կրկին փորձեք։','reminder.conflict':'Հիշեցումը փոխվել է։ Վերաբացեք էջը։'
},{
 'reminder.required':'Required payment reminder','reminder.due':'Contract payment due date','reminder.delivery':'Reminder delivery','reminder.notice':'Snooze changes only the reminder. Your payment due date stays unchanged.','reminder.snooze':'Snooze reminder','reminder.until':'Remind me at (device timezone, within 7 days)','reminder.pay':'Record payment','reminder.saved':'Reminder snoozed','reminder.failed':'Could not confirm. Retry to check the same request.','reminder.conflict':'Reminder changed. Reopen this page.'
});
const $=id=>document.getElementById(id);
let occurrence,request,busy=false,generation=0;
const pending=new Map();
register({id:'reminder',parent:'more',titleKey:'reminder.required',html:`<div class="stack"><div class="card"><p data-i18n="reminder.notice"></p><dl><dt data-i18n="reminder.due"></dt><dd id="reminder-due"></dd><dt data-i18n="reminder.delivery"></dt><dd id="reminder-delivery"></dd></dl><button id="reminder-pay" data-i18n="reminder.pay" disabled></button></div><form id="reminder-form" class="card stack"><label for="reminder-until" data-i18n="reminder.until"></label><input id="reminder-until" type="datetime-local" required><button id="reminder-submit" data-i18n="reminder.snooze" disabled></button></form><p id="reminder-error" role="status"></p></div>`,onMount(){
 $('reminder-pay').onclick=()=>{if(occurrence)go('payment',{id:occurrence.loan_id});};
 $('reminder-form').onsubmit=async e=>{
  e.preventDefault();if(busy||!occurrence)return;
  const id=occurrence.id,view=generation;
  if(!request){const until=new Date($('reminder-until').value);if(!Number.isFinite(until.getTime()))return;request={until:until.toISOString(),expected_version:occurrence.version,idempotency_key:crypto.randomUUID()};pending.set(id,request);}
  busy=true;$('reminder-submit').disabled=true;$('reminder-until').disabled=true;
  try{const res=await api(`api/reminders/${encodeURIComponent(id)}/snooze`,{method:'POST',body:JSON.stringify(request)});
   if(!res.ok){if(res.status===409||res.status===422||res.status===404){pending.delete(id);if(view===generation){request=null;$('reminder-until').disabled=false;}}throw new Error(res.status===409?'reminder.conflict':'reminder.failed');}
   const saved=await res.json();pending.delete(id);if(view!==generation||currentScreen()!=='reminder')return;
   occurrence=saved;request=null;$('reminder-until').disabled=false;render();$('reminder-error').textContent=T('reminder.saved');
  }catch(err){if(view===generation)$('reminder-error').textContent=T(err.message==='reminder.conflict'?'reminder.conflict':'reminder.failed');}
  finally{busy=false;if(currentScreen()==='reminder')$('reminder-submit').disabled=!occurrence||occurrence.status==='canceled';}
 };
},async onShow(_,params){
 const view=++generation;occurrence=null;request=null;$('reminder-error').textContent='';$('reminder-due').textContent='';$('reminder-delivery').textContent='';$('reminder-pay').disabled=true;$('reminder-submit').disabled=true;
 try{if(!params?.id)throw new Error();const res=await api(`api/reminders/${encodeURIComponent(params.id)}`);if(!res.ok)throw new Error();const result=await res.json();if(view!==generation||currentScreen()!=='reminder')return;occurrence=result;request=pending.get(result.id)||null;$('reminder-until').disabled=!!request;render();if(result.status==='canceled')return;$('reminder-pay').disabled=false;$('reminder-submit').disabled=busy;}
 catch{if(view===generation)$('reminder-error').textContent=T('reminder.failed');}
}});
function render(){ $('reminder-due').textContent=occurrence.due_date;$('reminder-delivery').textContent=new Date(occurrence.target_send_at).toLocaleString(); }
