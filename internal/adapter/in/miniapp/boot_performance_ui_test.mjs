import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

// Execute the production boot, navigator, catalogue and kernel together. Only
// screen registration and the network/DOM boundary are fakes.
const sources = await Promise.all(['core', 'i18n', 'nav', 'main'].map(async name =>
  (await readFile(new URL(`./web/js/${name}.js`, import.meta.url), 'utf8'))
    .replace(/^import .*;$/gm, '').replaceAll('export ', '')
    .replaceAll('import.meta.url', '"https://example.test/app/a/test/js/main.js"')));
class Element {
  constructor(tag='div') {
    this.tagName=tag;this.nodeType=1;this.childNodes=[];this.dataset={};this.attrs=new Map();
    this.hidden=false;this.disabled=false;this.value='';this.checked=false;this.listeners=new Map();
    this.style={};this.classList={toggle(){},add(){},remove(){}};
  }
  appendChild(el){el.parentElement=this;this.childNodes.push(el);return el;}
  append(...els){els.forEach(el=>this.appendChild(el));}
  get textContent(){return this.childNodes.map(n=>n.nodeValue??n.textContent).join('');}
  set textContent(text){this.childNodes=text===''?[]:[{nodeType:3,nodeValue:String(text)}];}
  set innerHTML(html){this.childNodes=[];if(html.includes('<span>')){const span=new Element('span');span.textContent=html.match(/<span>(.*?)<\/span>/s)[1];this.append(span);}}
  setAttribute(k,v){this.attrs.set(k,String(v));}
  getAttribute(k){return this.attrs.get(k)??null;}
  hasAttribute(k){return this.attrs.has(k);}
  removeAttribute(k){this.attrs.delete(k);}
  addEventListener(name,fn){this.listeners.set(name,fn);}
  matches(selector){return selector.split(',').some(raw=>{
    const s=raw.trim();
    if(s.startsWith('.'))return (this.className||'').split(' ').includes(s.slice(1));
    const attr=s.match(/^\[([^=\]]+)(?:="([^"]*)")?\]$/);
    if(attr){const value=attr[1].startsWith('data-')?this.dataset[attr[1].slice(5).replace(/-([a-z])/g,(_,c)=>c.toUpperCase())]:this.getAttribute(attr[1]);return value!=null&&(attr[2]===undefined||value===attr[2]);}
    if(s.startsWith('#'))return this.id===s.slice(1);
    const cls=s.split('.');return cls.length===2?this.tagName===cls[0]&&(this.className||'').split(' ').includes(cls[1]):s===this.tagName;
  });}
  querySelectorAll(selector){const out=[];for(const node of this.childNodes){if(node.nodeType!==1)continue;if(node.matches(selector))out.push(node);out.push(...node.querySelectorAll(selector));}return out;}
  querySelector(selector){return this.querySelectorAll(selector)[0]??null;}
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
async function boot(search='?screen=payment&id=loan-7',initialLanguage='hy') {
  const fields=new Map(),events=new Map(),settings=[];
  const field=id=>{if(!fields.has(id))fields.set(id,new Element());return fields.get(id);};
  const appbar=new Element();appbar.append(field('appbar-action'),field('appbar-title'));
  const document={documentElement:new Element(),body:new Element(),visibilityState:'visible',
    getElementById:id=>id==='settings-language'?fields.get(id)??null:field(id),
    createElement:tag=>new Element(tag),querySelectorAll:()=>field('tabs').childNodes,
    addEventListener(name,fn){if(!events.has(name))events.set(name,[]);events.get(name).push(fn);}};
  let viewChanges=0,reads=0;
  const env={document,window:{scrollTo(){},addEventListener(){}},URL,URLSearchParams,Intl,Date,
    setTimeout,clearTimeout,localStorage:{getItem(){return null;}},sessionStorage:{getItem(){return null;}},
    location:{search,href:'https://example.test/app/'+search},fetch:async()=>({ok:false}),
    api:()=>new Promise((resolve,reject)=>settings.push({resolve,reject})),
    prefetch(){},watchOffline(){},beginView(){viewChanges++;},
    read(){reads++;},makeElement:tag=>new Element(tag),field};
  vm.createContext(env);
  vm.runInContext(sources[0],env);
  vm.runInContext(`setLanguage('${initialLanguage}')`,env);
  for(const source of sources.slice(1,3))vm.runInContext(source,env);
  vm.runInContext(`
    addStrings({'test.payment':'Վճարում','test.home':'Գլխավոր','test.more':'Ավելին','test.dynamic':'Հեռացնել'},
      {'test.payment':'Payment','test.home':'Home','test.more':'More','test.dynamic':'Remove'});
    register({id:'home',labelKey:'test.home',html:'',onMount(root){
      const caption=field('home-caption');caption.tagName='span';root.append(caption);
      root.append(field('home-loan-name'));
    },onShow(){read();field('home-caption').textContent=T('test.dynamic');field('home-loan-name').textContent='Home';}});
    register({id:'loans',labelKey:'test.home',html:'',onShow(){read();}});
    register({id:'payment',parent:'home',titleKey:'test.payment',html:'',onMount(root){
      const label=makeElement('label');label.dataset.i18n='payment.amount';root.append(label);
      const amount=field('amount');amount.tagName='input';amount.value='100';amount.setAttribute('aria-label',T('test.dynamic'));root.append(amount);
      const tabs=field('form-tabs');tabs.setAttribute('role','tablist');tabs.dataset.i18nAriaLabel='test.payment';tabs.setAttribute('aria-label',T('test.payment'));root.append(tabs);
      const button=field('dynamic');button.tagName='button';button.className='alink';button.textContent=T('test.dynamic');button.onclick=()=>{};root.append(button);
      const option=field('option');option.tagName='option';option.value='keep';option.textContent='Home';root.append(option);const hint=field('hint');hint.className='hint';hint.textContent='Budget';root.append(hint);
    },onShow(){read();field('amount').value='100';}});
    register({id:'more',labelKey:'test.more',html:'',onMount(root){root.append(field('prefs'));},onShow(){read();field('prefs').value='saved';}});
  `,env);
  vm.runInContext(sources[3],env);
  return {env,field,settings,events,get reads(){return reads;},get viewChanges(){return viewChanges;},run:code=>vm.runInContext(code,env)};
}

{
  const app=await boot();
  assert.equal(app.run('currentScreen()'),'payment','deep link must mount while settings is unresolved');
  assert.equal(app.run('currentParams().id'),'loan-7');
  assert.equal(app.reads,1,'requested screen starts loading without account settings');
  app.field('amount').value='123.45';app.field('amount').disabled=true;
  const button=app.field('dynamic'),handler=button.onclick;
  const views=app.viewChanges;
  app.settings.shift().resolve({ok:true,json:async()=>({locale:'en'})});await flush();
  assert.equal(app.run('lang'),'en');assert.equal(button.textContent,'Remove','generated caption changes language');
  assert.equal(app.field('option').textContent,'Home','a loan name matching the catalogue must not be translated');assert.equal(app.field('hint').textContent,'Budget','user data in hints must not be translated');assert.equal(app.field('option').value,'keep');
  assert.equal(app.field('amount').value,'123.45','locale sync must retain edits');
  assert.equal(app.field('amount').disabled,true,'uncertain-save locks must survive');
  assert.equal(app.field('form-tabs').getAttribute('aria-label'),'Payment','explicit accessible names translate in place');
  assert.equal(app.field('amount').getAttribute('aria-label'),'Հեռացնել','unannotated attributes must never be guessed');
  assert.equal(button.onclick,handler,'locale sync must not replace controls or handlers');
  assert.equal(app.reads,1,'locale sync must not refetch/reset the form');
  assert.equal(app.viewChanges,views,'locale sync must not reset stale-resource tracking');
  assert.equal(app.field('appbar-title').textContent,'Payment');
  app.run("setLanguage('hy');refreshLanguage()");assert.equal(button.textContent,'Հեռացնել');
  assert.equal(app.field('form-tabs').getAttribute('aria-label'),'Վճարում');
  assert.equal(app.field('amount').value,'123.45');assert.equal(app.reads,1);
  app.run("go('more');setLanguage('en');refreshLanguage();go('payment',{id:'loan-7'})");
  assert.equal(app.field('form-tabs').getAttribute('aria-label'),'Payment','reopening a mounted form keeps its translated accessible name');
}
{
  const app=await boot('?screen=home','en');
  assert.equal(app.field('home-caption').textContent,'Remove','read-only content renders before settings');
  app.settings.shift().resolve({ok:true,json:async()=>({locale:'hy'})});await flush();
  assert.equal(app.field('home-caption').textContent,'Հեռացնել','unannotated generated cards finish in account language');
  assert.equal(app.field('home-loan-name').textContent,'Home','read-only refresh preserves catalogue-like user names');
  assert.equal(app.reads,2,'read-only roots use one fresh reload for generated labels');
  assert.equal(app.viewChanges,1,'locale completion does not navigate or reset resource tracking');
}
{
  const app=await boot();app.run("go('more')");app.field('prefs').value='unsaved';
  app.settings.shift().resolve({ok:true,json:async()=>({locale:'en'})});await flush();
  assert.equal(app.run('currentScreen()'),'more','late boot settings must not return to original deep link');
  assert.equal(app.field('prefs').value,'unsaved','root forms must not reset either');assert.equal(app.reads,2);
}
{
  const app=await boot();app.run("setLanguage('en');setLanguage('hy')");
  app.settings.shift().resolve({ok:true,json:async()=>({locale:'en'})});await flush();
  assert.equal(app.run('lang'),'hy','a newer local language revision wins over a stale settings read');
}
{
  const app=await boot();
  app.events.get('change').forEach(fn=>fn({target:{id:'settings-language'}}));
  app.settings.shift().resolve({ok:true,json:async()=>({locale:'en'})});await flush();
  assert.equal(app.run('lang'),'hy','a pending explicit language save also supersedes boot sync');
}
for(const failure of ['reject','http','invalid','body']){
  const app=await boot();const pending=app.settings.shift();
  if(failure==='reject')pending.reject(new Error('timeout'));
  else pending.resolve({ok:failure!=='http',json:()=>failure==='body'?Promise.reject(new Error('body timeout')):Promise.resolve({locale:'xx'})});
  await flush();assert.equal(app.run('currentScreen()'),'payment');assert.equal(app.reads,1);assert.equal(app.run('lang'),'hy');
}
console.log('Deferred settings boot: immediate deep links, safe locale refresh, preserved edits/navigation and no duplicate form loads.');

{
 const app=await boot();
 app.run(`let resolveLazy; let lazyLoads=0;
 registerLazy({id:'test-lazy',parent:'loans',load:()=>{lazyLoads++;return new Promise(resolve=>{resolveLazy=()=>{register({id:'test-lazy',parent:'loans',titleKey:'loans.title',html:''});resolve();};});}});
 go('test-lazy',{id:'keep-me'});go('test-lazy',{id:'keep-me'});`);
 assert.equal(app.run('lazyLoads'),1,'concurrent clicks share one module download');
 app.run("go('more');resolveLazy()");await flush();
 assert.equal(app.run('currentScreen()'),'more','late module loading must not steal navigation');
 app.run("go('test-lazy',{id:'keep-me'})");
 assert.equal(app.run('currentParams().id'),'keep-me','loaded screen receives original parameters');
}
{
 const app=await boot();
 app.run(`registerLazy({id:'test-failed',parent:'loans',load:()=>Promise.reject(new Error('offline'))});go('test-failed');`);
 await flush();
 assert.equal(app.run('currentScreen()'),'payment','failed module download preserves current form');
 assert.equal(app.field('view').hasAttribute('aria-busy'),false,'failed load clears busy indicator');
 assert.equal(app.field('appbar-action').hidden,false,'failed load offers a retry');
}
