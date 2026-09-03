import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=(await readFile(new URL('./web/js/screens/plan-evidence.js',import.meta.url),'utf8')).replace(/^import .*;$/gm,'').replaceAll('export ','');
const env={addStrings(){},T:k=>k,esc:v=>String(v).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;'),fmtMoney:(n,c)=>`${n} ${c}`};
vm.createContext(env);vm.runInContext(source,env);
const data={currency:'AMD',currency_exponent:2,outdated:true,certificate:{strength:'bounded_heuristic',policies:4096,truncation:'attempt cap 4096',lower_bound_minor:0,gap_minor:125,eligibility:'fixed orders'},excluded_loans:[{id:'private',name:'<script>loan</script>',reason:'required_only'}]};
let html=env.planEvidence(data);
for(const text of ['proof.bounded_heuristic','attempt cap 4096','0 AMD','1.25 AMD','proof.old','proof.required','&lt;script&gt;loan&lt;/script&gt;','<details'])assert.ok(html.includes(text),text);
assert.ok(!html.includes('<script>'));assert.ok(!html.includes('private'),'do not display internal loan IDs');
html=env.planEvidence({currency:'AMD',currency_exponent:2,summary:{strength:'named_strategies_only'},certificate:{lower_bound_minor:null,gap_minor:null},excluded_loans:[]});
assert.ok(html.includes('proof.named_strategies_only'));assert.ok(html.includes('proof.none'));assert.ok(!html.includes('0 AMD'),'unknown evidence must not become zero');
for(const screen of ['plan','plan-scenarios','plan-history']){const text=await readFile(new URL(`./web/js/screens/${screen}.js`,import.meta.url),'utf8');assert.match(text,/planEvidence\(d\)/,screen+' uses the shared disclosure');}
console.log('All three plan surfaces disclose bounded evidence, unknown values, exclusions and historical staleness.');
