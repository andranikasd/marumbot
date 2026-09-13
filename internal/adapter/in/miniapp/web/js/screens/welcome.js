"use strict";
import {register} from '../nav.js';
import {addStrings} from '../i18n.js';
addStrings({
 'welcome.title':'Մարեք վարկերը պարզ պլանով','welcome.intro':'Բանկի հավելվածը պահեք ձեռքի տակ։',
 'welcome.loans':'Ավելացրեք ձեր վարկերը','welcome.loans.hint':'Մենք կհավաքենք պարտադիր վճարումները մեկ տեղում։',
 'welcome.extra':'Ընտրեք հավելյալ գումարը','welcome.extra.hint':'Որքան կարող եք ավելացնել ամեն ամիս։ Զրոն էլ է ընդունելի։',
 'welcome.plan':'Տեսեք ձեր պլանը','welcome.plan.hint':'Ինչքան և որ վարկին վճարել։ Փոխեք պլանը, երբ պետք է։',
 'welcome.start':'Ավելացնել առաջին վարկը'
},{
 'welcome.title':'A simpler way to repay','welcome.intro':'Keep your bank app handy.',
 'welcome.loans':'Add your loans','welcome.loans.hint':'See your required payments in one place.',
 'welcome.extra':'Choose your extra','welcome.extra.hint':'What you can add each month. Zero is fine.',
 'welcome.plan':'Get your plan','welcome.plan.hint':'See what to pay on each loan. Adjust whenever you need.',
 'welcome.start':'Add my first loan'
});
register({id:'welcome',parent:'simple-plan',titleKey:'welcome.title',html:`<div class="stack">
 <p class="hint" data-i18n="welcome.intro"></p>
 <section class="card stack"><div><b data-i18n="welcome.loans"></b><p class="hint" data-i18n="welcome.loans.hint"></p></div><div><b data-i18n="welcome.extra"></b><p class="hint" data-i18n="welcome.extra.hint"></p></div><div><b data-i18n="welcome.plan"></b><p class="hint" data-i18n="welcome.plan.hint"></p></div></section>
 <button class="cta" data-go="loan-setup" data-i18n="welcome.start"></button></div>`});
