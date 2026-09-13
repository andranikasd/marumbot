"use strict";
import {api} from '../api.js';
import {tg} from '../core.js';
import {register,go,currentScreen} from '../nav.js';
import {addStrings,T,sub} from '../i18n.js';
addStrings({
 'notify.title':'Հիշեցնե՞լ վճարման մասին','notify.desc':'Ընտրեք մեկ հիշեցում յուրաքանչյուր վճարման համար։','notify.enable':'Միացնել հիշեցումները','notify.when':'Երբ','notify.before':'Մեկ օր առաջ','notify.day':'Վճարման օրը','notify.time':'Ժամը','notify.save':'Պահպանել','notify.later':'Ոչ հիմա','notify.permission':'Telegram-ում թույլ տվեք բոտին հաղորդագրություններ ուղարկել։ Եթե առաջարկ չկա, բացեք բոտի զրույցը և սեղմեք Start։','notify.error':'Պահպանումը հաստատված չէ։ Կրկին փորձեք նույն հարցումը։','notify.conflict':'Կարգավորումները փոխվել են։ Վերբեռնեք վերջին տարբերակը։','notify.invalid':'Ընտրեք հիշեցման օրը և վավեր ժամը։','notify.days':'{days} օր առաջ'
},{
 'notify.title':'Want a payment reminder?','notify.desc':'Choose one reminder for each payment.','notify.enable':'Enable reminders','notify.when':'When','notify.before':'One day before','notify.day':'On the due date','notify.time':'Time','notify.save':'Save','notify.later':'Not now','notify.permission':'Allow the bot to send messages in Telegram. If no prompt appears, open the bot chat and tap Start.','notify.error':'Save unconfirmed. Retry the same request.','notify.conflict':'Settings changed. Reload the latest version.','notify.invalid':'Choose a reminder day and a valid time.','notify.days':'{days} days before'
});
const $=id=>document.getElementById(id);
let saved=null,request=null,busy=false,loading=false,dirty=false,conflict=false,errorKey='',view=0;
function controls(){
 for(const el of document.querySelectorAll('#notify-form input,#notify-form select'))el.disabled=busy||loading||!!request||!saved;
 const locked=busy||loading||!!request||!saved;
 $('notify-lead').disabled=locked||!$('notify-enabled').checked;
 $('notify-time').disabled=locked||!$('notify-enabled').checked;
 $('notify-save').disabled=busy||loading||!saved;
 $('notify-save').textContent=T(request?'retry':'notify.save');
 $('notify-retry').hidden=!!request||busy||loading||(!conflict&&!!saved);
 $('notify-error').textContent=T(errorKey)||'';
}
function valid(data){
 if(!data||!Number.isSafeInteger(data.version)||data.version<0||typeof data.timezone!=='string'||typeof data.reminders_enabled!=='boolean')return false;
 const lead=data.reminder_lead_days,minute=data.reminder_minute;
 return lead==null&&minute==null||Number.isInteger(lead)&&lead>=0&&lead<=7&&Number.isInteger(minute)&&minute>=0&&minute<1440;
}
function leadOptions(){
 const selected=$('notify-lead').value;
 const custom=saved?.reminder_lead_days;
 $('notify-custom').hidden=!(custom>=2&&custom<=7);
 $('notify-custom').value=custom==null?'':String(custom);
 $('notify-custom').textContent=custom>=2?sub('notify.days',{days:custom}):'';
 $('notify-lead').value=selected;
}
async function load(discard=false){
 if(request||busy||loading||(dirty&&!discard)){controls();return;}
 const generation=++view;saved=null;loading=true;errorKey='loading';controls();
 try{
  const r=await api('api/settings/reminders',{cache:'no-store'});if(!r.ok)throw new Error();
  const data=await r.json();if(generation!==view)return;if(!valid(data))throw new Error();
  saved=data;dirty=false;conflict=false;$('notify-enabled').checked=data.reminders_enabled;
  leadOptions();$('notify-lead').value=String(data.reminder_lead_days??1);const m=data.reminder_minute??540;
  $('notify-time').value=String(Math.floor(m/60)).padStart(2,'0')+':'+String(m%60).padStart(2,'0');
  $('notify-zone').textContent=data.timezone;errorKey='';
 }catch{if(generation===view)errorKey='err.load';}
 finally{if(generation===view){loading=false;controls();}}
}
async function permission(){
 if(tg?.initDataUnsafe?.user?.allows_write_to_pm)return true;
 if(!tg?.requestWriteAccess)return false;
 let timer;
 try{return await Promise.race([new Promise(resolve=>tg.requestWriteAccess(resolve)),new Promise(resolve=>{timer=setTimeout(()=>resolve(false),20000);})]);}
 finally{clearTimeout(timer);}
}
async function save(e){
 e?.preventDefault();if(busy||loading||!saved)return;
 busy=true;errorKey='';controls();
 try{
  if(!request){
   const enabled=$('notify-enabled').checked,time=$('notify-time').value,lead=Number($('notify-lead').value);
   if(enabled&&(!/^([01]\d|2[0-3]):[0-5]\d$/.test(time)||!Number.isInteger(lead)||lead<0||lead>7))throw new Error('notify.invalid');
   const [h,m]=time.split(':').map(Number);
   const candidate={timezone:saved.timezone,quiet_enabled:saved.quiet_enabled,quiet_start:saved.quiet_start,quiet_end:saved.quiet_end,version:saved.version,reminders_enabled:enabled,reminder_lead_days:enabled?lead:saved.reminder_lead_days,reminder_minute:enabled?h*60+m:saved.reminder_minute,idempotency_key:crypto.randomUUID()};
   if(enabled&&!saved.reminders_enabled&&!await permission())throw new Error('notify.permission');
   request=candidate;
  }
  const r=await api('api/settings/reminders',{method:'POST',body:JSON.stringify(request)});
  if(!r.ok){
   if(r.status>=400&&r.status<500){request=null;if(r.status===409){saved=null;dirty=false;conflict=true;}}
   throw new Error(r.status===409?'notify.conflict':'notify.error');
  }
  const result=await r.json();if(!valid(result))throw new Error('notify.error');
  saved=result;request=null;dirty=false;errorKey='';
  if(currentScreen()==='reminder-setup')go('simple-plan');
 }catch(err){errorKey=['notify.permission','notify.conflict','notify.invalid'].includes(err.message)?err.message:'notify.error';}
 finally{busy=false;controls();}
}
register({id:'reminder-setup',parent:'more',titleKey:'notify.title',html:`<form id="notify-form" class="stack" novalidate><p class="mute" data-i18n="notify.desc"></p><label class="check-row"><input type="checkbox" id="notify-enabled"><span data-i18n="notify.enable"></span></label><label for="notify-lead" data-i18n="notify.when"></label><select id="notify-lead"><option value="1" data-i18n="notify.before"></option><option value="0" data-i18n="notify.day"></option><option id="notify-custom" hidden></option></select><label for="notify-time" data-i18n="notify.time"></label><input type="time" id="notify-time" required><small id="notify-zone" class="mute"></small><button class="cta" id="notify-save" data-i18n="notify.save" disabled></button><button class="alink" type="button" data-go="simple-plan" data-i18n="notify.later"></button><p role="status" id="notify-error"></p><button class="alink" id="notify-retry" type="button" data-i18n="retry"></button></form>`,onMount(){$('notify-form').onsubmit=save;$('notify-form').addEventListener('input',()=>{dirty=true;controls();});$('notify-form').addEventListener('change',()=>{dirty=true;controls();});$('notify-retry').onclick=()=>load(true);},onShow(){return load();},onLanguage(){leadOptions();controls();}});
