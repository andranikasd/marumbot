"use strict";
import { addStrings } from "../i18n.js";

addStrings({
  "bh.title": "Ինչպես է աշխատում բյուջեն",
  "bh.rule": "Պլանը վճարում է միայն այն դեպքում, երբ գումարը հասանելի է և բյուջեի սահմանը բավարար է։ Նախ՝ պարտադիր վճարումները, հետո՝ հավելյալը։",
  "bh.limit": "Բյուջեն վարկերի ամսական ծախսի սահմանն է՝ ներառյալ պարտադիր վճարումները։",
  "bh.monthly": "Ամսական գումարը վարկերի համար պարբերաբար առանձնացվող գումարն է։ Նշեք՝ ամսվա որ օրն է այն հասանելի։",
  "bh.cash": "Այսօր ձեռքի տակ եղած գումարը նշեք առանձին։ Նշեք նաև պահուստը, որը չեք ուզում ծախսել։",
  "bh.paid": "«Արդեն վճարվածը» ընթացիկ բյուջետային ժամանակահատվածում վարկերին վճարած գումարն է, ոչ թե նոր հասանելի գումար։ Գումարի մուտքի օրը այն չի զրոյանում։",
  "bh.overrides": "Առանձին ամսվա սահմանը փոխարինում է սովորական սահմանին․ այն գումար չի ավելացնում։",
  "bh.extra": "Չհաստատված լրացուցիչ գումարը նշեք որպես սպասվող։ Այն չի ներառվում հիմնական պլանում մինչև հաստատումը։",
}, {
  "bh.title": "How budgeting works",
  "bh.rule": "The plan needs both available money and room in your budget. Required payments come first; extra payments use what remains.",
  "bh.limit": "Budget is your monthly loan spending limit, including required payments.",
  "bh.monthly": "Monthly loan money is the money you regularly set aside for loans. Set payday to the day it becomes available.",
  "bh.cash": "Enter cash you hold today separately. Also set the reserve you want to keep untouched.",
  "bh.paid": "Already paid means loan payments made in the current budget period, not fresh cash. Payday does not reset it.",
  "bh.overrides": "A limit for a specific month replaces your usual limit. It does not create money.",
  "bh.extra": "Mark uncertain extra money as expected. It stays out of the base plan until confirmed.",
});

export const budgetHelpHTML = `
  <details class="fold budget-help">
    <summary data-i18n="bh.title"></summary>
    <div class="fold-body">
      <p data-i18n="bh.rule"></p>
      <p data-i18n="bh.limit"></p>
      <p data-i18n="bh.monthly"></p>
      <p data-i18n="bh.cash"></p>
      <p data-i18n="bh.paid"></p>
      <p data-i18n="bh.overrides"></p>
      <p data-i18n="bh.extra"></p>
    </div>
  </details>`;
