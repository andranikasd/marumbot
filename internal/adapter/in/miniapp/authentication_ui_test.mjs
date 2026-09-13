import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/api.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'').replaceAll('export ','');
function setup(initData='signed-data'){
 const fields=new Map();let requests=0;let status=200;
 const field=id=>{if(!fields.has(id))fields.set(id,{hidden:false});return fields.get(id);};
 const env={AbortController,setTimeout,clearTimeout,Map,Set,Date,Error,TypeError,Event,
  document:{getElementById:field},addStrings(){},tg:{initData,close(){}},T:x=>x,
  fetch:async(path,init)=>{requests++;assert.equal(init.headers['X-Telegram-Init-Data'],initData);return {ok:status===200,status,headers:{get:()=>status===403?'account-required':null},json:async()=>({loans:[]})};}};
 vm.createContext(env);vm.runInContext(source,env);
 return {env,field,get requests(){return requests;},set status(value){status=value;}};
}
{
 const app=setup('');app.env.watchAuthentication();
 assert.equal(app.field('auth-required').hidden,false);
 assert.equal(app.field('auth-message').textContent,'auth.missing');
 await assert.rejects(app.env.getJSON('api/loans'),/authentication required/);
 await assert.rejects(app.env.api('api/settings',{method:'POST',body:'{"locale":"en"}'}),/authentication required/);
 assert.equal(app.requests,0,'missing credentials must not generate repeated failing requests');
 assert.equal(app.field('view').hidden,true,'generic failed/empty data must not obscure launch instructions');
}
for(const [status,message] of [[401,'auth.expired'],[403,'auth.account']]){
 const app=setup();await app.env.getJSON('api/loans');app.status=status;
 await assert.rejects(app.env.getJSON('api/loans'),/authentication required/);
 assert.equal(app.field('auth-message').textContent,message);
 assert.equal(app.field('tabs').hidden,true,'stale financial content must not stay visible after auth rejection');
 assert.equal(app.env.authenticationRequired(),true);
}
{
 const app=setup();const empty=await app.env.getJSON('api/loans');
 assert.deepEqual(empty,{loans:[]});assert.equal(app.env.authenticationRequired(),false);
 app.status=503;await assert.rejects(app.env.getJSON('api/loans'),/http 503/);
 assert.equal(app.env.authenticationRequired(),false,'server failures are not missing accounts');
}
console.log('Missing, expired and unknown-account auth are distinct from empty data and server failures.');
