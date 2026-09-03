"use strict";
import {api} from './api.js';
import {addStrings,T} from './i18n.js';

addStrings({
 'loan.write.check':'Ստուգել նախորդ պահպանումը',
 'loan.write.retry':'Պահպանումը հաստատված չէ։ Կրկին ուղարկեք նույն տվյալները՝ առանց նոր գրառում ստեղծելու։',
 'loan.write.conflict':'Վարկի տվյալները փոխվել են։ Վերաբացեք էջը, ստուգեք փոփոխությունները և կրկին պահպանեք։'
},{
 'loan.write.check':'Check previous save',
 'loan.write.retry':'Save is unconfirmed. Retry the same details to check the original request.',
 'loan.write.conflict':'Loan details changed. Reopen this page, review the changes, then save again.'
});

// Financial drafts stay in memory. Navigation and transport failures retain
// the original command identity; editing an uncertain request cannot create a second fact.
const pending=new Map();
export async function loanMutation(path,method,body,version){
 const payload=body===undefined?undefined:JSON.stringify(body);
 let request=pending.get(path);
 if(request&&(request.method!==method||request.body!==payload)){
  throw new Error('loan_retry_pending');
 }
 if(!request){request={method,body:payload,version,key:crypto.randomUUID()};pending.set(path,request);}
 const headers={'Idempotency-Key':request.key};
 if(request.version!==undefined)headers['If-Match']=String(request.version);
 const response=await api(path,{method:request.method,body:request.body,headers});
 if(response.ok||(response.status>=400&&response.status<500))pending.delete(path);
 return response;
}
export function loanWriteError(err){
 if(err?.message==='loan_conflict')return T('loan.write.conflict');
 if(err instanceof TypeError||err?.message==='loan_retry_pending'||/^5\d\d$/.test(err?.message||''))return T('loan.write.retry');
 return T('err.save');
}

// A recovery action can resend the exact original command even if navigation
// refreshed the displayed loan, or an archive hid it after a lost response.
export function showLoanRetry(button,path,onDone){
 button.hidden=!pending.has(path);
 button.textContent=T('loan.write.check');
 button.onclick=async()=>{
  const request=pending.get(path);if(!request)return;
  button.disabled=true;
  try{
   const response=await loanMutation(path,request.method,request.body===undefined?undefined:JSON.parse(request.body),request.version);
   if(response.ok){button.hidden=true;await onDone();}
   else {button.textContent=T(response.status===409?'loan.write.conflict':'loan.write.retry');}
  }catch{button.textContent=T('loan.write.retry');}
  finally{button.disabled=false;}
 };
}
