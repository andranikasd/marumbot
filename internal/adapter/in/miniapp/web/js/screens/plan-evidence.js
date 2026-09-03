"use strict";
import {addStrings,T} from '../i18n.js';
import {esc,fmtMoney} from '../core.js';
addStrings({
 'proof.title':'Հաշվարկի հիմքը','proof.proven_optimal':'Ծախսի նվազագույնը հաստատված է','proof.exhaustive_static_order':'Համեմատվել են բոլոր հաստատուն հերթականությունները','proof.bounded_heuristic':'Լավագույնը ստուգված տարբերակներից','proof.named_strategies_only':'Ընտրված մեթոդի հաշվարկ','proof.unknown':'Հաստատված չէ','proof.attempts':'Ստուգված տարբերակներ','proof.limit':'Որոնման սահմանափակում','proof.lower':'Ծախսի ստորին սահման','proof.gap':'Տարբերությունը ստորին սահմանից','proof.excluded':'Լրացուցիչ վճարումներից բացառված վարկեր','proof.none':'Չկան','proof.required':'Պարտադիր վճարումները մնում են պլանում։','proof.old':'Տվյալները փոխվել են․ ցուցադրված է սկզբնական հաշվարկը։','proof.scope':'Հաշվարկի կիրառելիությունը'
},{
 'proof.title':'Calculation evidence','proof.proven_optimal':'Minimum cost proven','proof.exhaustive_static_order':'All fixed orders compared','proof.bounded_heuristic':'Best of the tested options','proof.named_strategies_only':'Selected method calculation','proof.unknown':'Not established','proof.attempts':'Options tested','proof.limit':'Search limit','proof.lower':'Cost lower bound','proof.gap':'Gap from lower bound','proof.excluded':'Excluded from extra payments','proof.none':'None','proof.required':'Required payments remain in the plan.','proof.old':'Inputs have changed. This is the original calculation.','proof.scope':'Calculation scope'
});
// One compact disclosure for current plans, immutable scenarios and history.
export function planEvidence(d){
 const c=d.certificate||{};
 const strength=c.strength||d.summary?.strength;
 const known=['proven_optimal','exhaustive_static_order','bounded_heuristic','named_strategies_only'];
 const label=T(known.includes(strength)?'proof.'+strength:'proof.unknown');
 const amount=n=>n==null?T('proof.unknown'):fmtMoney(n/10**d.currency_exponent,d.currency);
 const row=(name,value)=>`<div><span>${esc(T(name))}</span><b>${esc(value)}</b></div>`;
 const excluded=d.excluded_loans;
 const names=Array.isArray(excluded)?(excluded.length?excluded.map(l=>l.name).join(', '):T('proof.none')):T('proof.unknown');
 return `${d.outdated?`<p class="hint" role="status">${esc(T('proof.old'))}</p>`:''}<p class="hint">${esc(label)}</p><details class="card"><summary>${esc(T('proof.title'))}</summary><div class="kv">${row('proof.attempts',c.policies??T('proof.unknown'))}${row('proof.limit',c.truncation||T('proof.unknown'))}${row('proof.lower',amount(c.lower_bound_minor))}${row('proof.gap',amount(c.gap_minor))}${c.eligibility?row('proof.scope',c.eligibility):''}${row('proof.excluded',names)}</div>${excluded?.length?`<p class="hint">${esc(T('proof.required'))}</p>`:''}</details>`;
}
