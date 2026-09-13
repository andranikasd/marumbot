import assert from 'node:assert/strict';
import {readFile,readdir} from 'node:fs/promises';
const base=new URL('./web/js/',import.meta.url);
// Import actual modules without mounting screens; cold boot must not depend on
// whether a user has already opened a lazy destination in this session.
globalThis.window={};globalThis.location={search:''};
globalThis.document={documentElement:{dataset:{},style:{}},body:{classList:{toggle(){}}},createElement(){return {textContent:'',get innerHTML(){return this.textContent;}};}};
const {STRINGS}=await import(new URL('i18n.js',base));
const main=await readFile(new URL('main.js',base),'utf8');
const eager=[];
for(const match of main.matchAll(/^import "(.+screens\/.+)";/gm)){await import(new URL(match[1],base));eager.push(match[1]);}
function keys(source){return [...source.matchAll(/(?:T|sub)\(\s*["']([^"']+)["']/g),...source.matchAll(/data-i18n(?:-aria-label)?=["']([^"']+)["']/g)].map(m=>m[1]).filter(k=>!k.endsWith('.')&&!k.includes('${'));}
for(const file of eager){const source=await readFile(new URL(file,base),'utf8');for(const key of keys(source))for(const lang of ['hy','en'])assert.ok(STRINGS[lang][key],`cold boot ${file}: ${lang} ${key}`);}
const all=await readdir(new URL('screens/',base));
for(const file of all)await import(new URL('screens/'+file,base));
for(const file of all){const source=await readFile(new URL('screens/'+file,base),'utf8');for(const key of keys(source))for(const lang of ['hy','en'])assert.ok(STRINGS[lang][key],`${file}: ${lang} ${key}`);}
assert.deepEqual(Object.keys(STRINGS.hy).sort(),Object.keys(STRINGS.en).sort(),'catalog keys match in Armenian and English');
for(const key of Object.keys(STRINGS.hy))assert.deepEqual([...STRINGS.hy[key].matchAll(/\{(\w+)\}/g)].map(m=>m[1]).sort(),[...STRINGS.en[key].matchAll(/\{(\w+)\}/g)].map(m=>m[1]).sort(),`${key}: translated substitutions match`);
console.log('Cold-boot links, all literal screen keys, both-language catalog parity and interpolation fields are covered.');
// Reason keys are selected dynamically from API values, so literal-key scans
// alone cannot detect a missing translation here.
for(const reason of ['bank_payment_mismatch','non_amortizing_payment','payment_reconciliation_required','unknown_terms','overdue','accrued_interest_needed','interest_needed','bank_payment_needed','early_payment_rules_needed']){
 for(const language of ['hy','en'])assert.ok(STRINGS[language]['sp.reason.'+reason],`${language} projection reason ${reason}`);
}
