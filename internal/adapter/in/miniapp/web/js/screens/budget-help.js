"use strict";
import { addStrings } from "../i18n.js";

addStrings({
  "bh.title": "Ո՞ր գումարը որտեղ նշել",
  "bh.rule": "Նշեք մեկ ամսական գումար՝ բոլոր վարկերի համար։ Այն նաև ծախսի սահմանն է, եթե առանձին սահման չեք ընտրում։ Նախ՝ պարտադիր վճարումները։",
  "bh.limit": "Բյուջեն վարկերի ամսական ծախսի սահմանն է՝ ներառյալ պարտադիր վճարումները։",
  "bh.monthly": "Ամսական գումարը վարկերի համար պարբերաբար առանձնացվող գումարն է։ Նշեք՝ ամսվա որ օրն է այն հասանելի։",
  "bh.cash": "Այս պահին հասանելի գումարը վարկերի համար ձեր հաշվում դեռ մնացած գումարն է։ Մի ներառեք ապագա մուտքերն ու արդեն վճարվածը։ Եթե չկա՝ նշեք 0, ամսական գումարը կրկին մի ավելացրեք։",
  "bh.paid": "«Արդեն վճարվածը» ընթացիկ բյուջետային ժամանակահատվածում վարկերին վճարած գումարն է, ոչ թե նոր հասանելի գումար։ Գումարի մուտքի օրը այն չի զրոյանում։",
  "bh.overrides": "Առանձին ամսվա սահմանը փոխարինում է սովորական սահմանին․ այն գումար չի ավելացնում։",
  "bh.extra": "Չհաստատված լրացուցիչ գումարը նշեք որպես սպասվող։ Այն չի ներառվում հիմնական պլանում մինչև հաստատումը։",
}, {
  "bh.title": "Which money goes where?",
  "bh.rule": "Start with one monthly amount for all your loans. We use it as your spending limit too, unless you choose a different limit. Required payments come first.",
  "bh.limit": "Budget is your monthly loan spending limit, including required payments.",
  "bh.monthly": "Monthly loan money is the money you regularly set aside for loans. Set payday to the day it becomes available.",
  "bh.cash": "Available now is money still in your account for loans. Leave out future income and money already paid. Enter 0 if there is none; do not add the monthly amount again.",
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
