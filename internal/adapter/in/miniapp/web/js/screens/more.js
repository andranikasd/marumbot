"use strict";
import "./reminder.js";
import {mountPreferences,showPreferences,preferencesHTML} from "./user-preferences.js";
import {register,refreshLanguage} from '../nav.js';
import {addStrings,T} from '../i18n.js';
import {icon} from '../icons.js';
import {lang,setLanguage} from '../core.js';
import {api} from '../api.js';
addStrings({'tab.more':'Ավելին','more.budget':'Խմբագրել բյուջեն','more.loan':'Ավելացնել վարկ','settings.language':'Լեզու / Language'},{'tab.more':'More','more.budget':'Edit budget','more.loan':'Add loan','settings.language':'Language / Լեզու'});
register({id:'more',icon:icon('wallet'),labelKey:'tab.more',html:`<div class="stack">${preferencesHTML}<div class="card field"><label for="settings-language" data-i18n="settings.language"></label><select id="settings-language"><option value="hy">Հայերեն</option><option value="en">English</option></select><p id="settings-error" class="error" role="alert"></p></div><button class="card" data-go="budget-edit" data-i18n="more.budget"></button><button class="card" data-go="add" data-i18n="more.loan"></button></div>`,onMount(){
 mountPreferences();
 const select=document.getElementById('settings-language');select.addEventListener('change',async()=>{
  const want=select.value;select.disabled=true;document.getElementById('settings-error').textContent='';
  try{const res=await api('api/settings',{method:'POST',body:JSON.stringify({locale:want})});if(!res.ok)throw new Error('save');const saved=await res.json();setLanguage(saved.locale);refreshLanguage();}
  catch{select.value=lang;document.getElementById('settings-error').textContent=T('err.save');}
  finally{select.disabled=false;}
 });
},onShow(){showPreferences();document.getElementById('settings-language').value=lang;}});
