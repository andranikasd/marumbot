"use strict";
import "./reminder.js";
import "./reminder-setup.js";
import {mountPreferences,showPreferences,preferencesHTML} from "./user-preferences.js";
import {register,refreshLanguage} from '../nav.js';
import {addStrings,T} from '../i18n.js';
import {icon} from '../icons.js';
import {lang,setLanguage} from '../core.js';
import {api} from '../api.js';
addStrings({'settings.extra':'Հավելյալ վճարումներ','settings.history':'Պատմություն','settings.advanced':'Նախկին գործիքներ','tab.more':'Կարգավորումներ','more.budget':'Խմբագրել բյուջեն','more.loan':'Ավելացնել վարկ','settings.language':'Լեզու / Language'},{'settings.extra':'Extra payments','settings.history':'Activity','settings.advanced':'Previous tools','tab.more':'Settings','more.budget':'Edit budget','more.loan':'Add loan','settings.language':'Language / Լեզու'});
register({id:'more',icon:icon('wallet'),labelKey:'tab.more',html:`<div class="stack"><div class="card field"><label for="settings-language" data-i18n="settings.language"></label><select id="settings-language"><option value="hy">Հայերեն</option><option value="en">English</option></select><p id="settings-error" class="error" role="alert"></p></div><button class="card" data-go="extra" data-i18n="settings.extra"></button><button class="card" data-go="reminder-setup" data-i18n="prefs.title"></button><details class="fold"><summary data-i18n="settings.advanced"></summary><div class="fold-body"><button class="card" data-go="budget-edit" data-i18n="more.budget"></button><button class="card" data-go="activity" data-i18n="settings.history"></button></div></details><button class="card" data-go="loan-setup" data-i18n="more.loan"></button><details class="fold"><summary data-i18n="prefs.title"></summary><div class="fold-body">${preferencesHTML}</div></details></div>`,onMount(){
 mountPreferences();
 const select=document.getElementById('settings-language');select.addEventListener('change',async()=>{
  const want=select.value;select.disabled=true;document.getElementById('settings-error').textContent='';
  try{const res=await api('api/settings',{method:'POST',body:JSON.stringify({locale:want})});if(!res.ok)throw new Error('save');const saved=await res.json();if(saved.locale!=='hy'&&saved.locale!=='en')throw new Error('locale');setLanguage(saved.locale);refreshLanguage();}
  catch{select.value=lang;document.getElementById('settings-error').textContent=T('err.save');}
  finally{select.value=lang;select.disabled=false;}
 });
},onShow(){showPreferences();const select=document.getElementById('settings-language');if(!select.disabled)select.value=lang;}});
