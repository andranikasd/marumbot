package i18n

// The message catalogue.
//
// Terminology matters more than fluency here. A borrower reading about their
// own debt needs the words their lender used, so these follow standard Armenian
// banking usage: վարկ for a loan, մայր գումար for principal, տոկոսադրույք for
// the interest rate, մարման գրաֆիկ for a repayment schedule, and վճարում for
// the monthly instalment -- the word on the bank's own schedule. "Marum"
// itself is մարում, the repayment of a debt.
//
// English uses "instalment" for the contractual monthly figure and "payment"
// for any money handed over, so an extra payment is never an instalment.
//
// Messages go out in Telegram's HTML mode, so a string may carry <b>, <i> and
// <code>; anything a borrower typed is escaped before it is formatted in.
// Numbers sit in <code> so they line up and stand out from the prose.
//
// Every key must exist in every locale. TestEveryKeyIsTranslated enforces it,
// because a message that falls back to another language mid-conversation is how
// a user stops trusting the numbers around it.
var catalogue = map[Locale]map[string]string{
	HY: {
		// --- greeting ---
		"start.greeting": "Բարև, ես Մարումն եմ 👋\n\n" +
			"1️⃣ Ավելացնում եք վարկերը\n" +
			"2️⃣ Նշում եք ամսական բյուջեն\n" +
			"3️⃣ Ես ասում եմ՝ ում, երբ և որքան վճարել, որ տոկոսն ամենաքիչը լինի",
		"start.next":     "👇 Սկսենք՝ սեղմեք «➕ Ավելացնել վարկ»։",
		"start.language": "🌐 Prefer English? Tap «🌐 Լեզուն»։",
		"start.no_ai":    "Ոչ մի կռահում․ ամեն թիվ հաշվարկվում է հրապարակված բանաձևով՝ ձեր տվյալներից։",

		// --- how it works ---
		"help.title": "❓ Ինչպե՞ս է աշխատում Մարումը",
		"help.intro": "Դուք ավելացնում եք վարկերն ու բյուջեն։ Ես ստուգում եմ վճարման բոլոր " +
			"հերթականություններն ու ժամկետները և ասում՝ որն է լավագույնը ձեր նպատակի համար։",
		"help.goals":         "Երեք նպատակ՝",
		"help.goal.cheapest": "💸 <b>Ամենաքիչ տոկոս</b> — բանկին ընդհանուր առմամբ ամենաքիչը վճարել։",
		"help.goal.soonest":  "🏁 <b>Ամենաշուտ ավարտ</b> — բոլոր վարկերը փակել հնարավորինս շուտ, և տեսնել՝ ինչ կտա ավելի մեծ բյուջեն։",
		"help.goal.relief":   "🌬 <b>Ամսական թեթևացում</b> — առաջին վարկը փակելուց հետո ամսական վճարն իջեցնել։",
		"help.commands":      "Հրամաններ՝",
		"help.add":           "/add — ավելացնել վարկ",
		"help.advice":        "/advice — ի՞նչ անել այս ամիս",
		"help.loans":         "/loans — իմ վարկերը",
		"help.budget":        "/budget — ամսական բյուջե",
		"help.language":      "/language — լեզուն",
		"help.help":          "/help — այս բացատրությունը",

		// Command descriptions, shown by Telegram in the "/" menu. Kept short:
		// the client truncates them, and a description that needs a comma is
		// describing too much.
		"menu.add":      "Ավելացնել վարկ",
		"menu.advice":   "Ի՞նչ անել այս ամիս",
		"menu.loans":    "Իմ վարկերը",
		"menu.budget":   "Ամսական բյուջե",
		"menu.language": "Լեզուն / Language",
		"menu.help":     "Ինչպես է աշխատում",

		// Keyboard buttons. Short enough to sit two to a row on a phone.
		"btn.add":        "➕ Ավելացնել վարկ",
		"btn.advice":     "💡 Ի՞նչ անել",
		"btn.loans":      "📋 Իմ վարկերը",
		"btn.budget":     "💰 Բյուջե",
		"btn.language":   "🌐 Լեզուն",
		"btn.help":       "❓ Ինչպե՞ս է աշխատում",
		"btn.plan":       "💡 Տեսնել պլանը",
		"btn.manage":     "✏️ Կառավարել վարկերը",
		"kb.placeholder": "Ընտրեք գործողություն",

		// --- the advice report ---
		"advice.title":    "💡 Ձեր պլանը",
		"advice.currency": "Բոլոր գումարները՝ %s",
		"advice.owed":     "Ընդհանուր պարտք",
		"advice.required": "Այս ամիս պարտադիր",
		"advice.budget":   "Բյուջե",
		"advice.payday":   "Աշխատավարձ՝ ամսի %d-ին",

		"goal.cheapest": "💸 Ամենաքիչ տոկոս",
		"goal.soonest":  "🏁 Ամենաշուտ ավարտ",
		"goal.relief":   "🌬 Ամսական թեթևացում",
		"goal.first":    "🎯 Առաջին հաղթանակ",

		"advice.this_month":  "📌 Այս ամիս",
		"advice.step_due":    "☐ %s — <code>%s</code> → «%s»",
		"advice.step_extra":  "☐ %s — <code>%s</code> → «%s» ➕ լրացուցիչ",
		"advice.step_early":  "☐ %s — <code>%s</code> → «%s» ⚡ լրացուցիչ, մինչև վճարման օրը (խնայում է %s)",
		"advice.no_surplus":  "Ավելցուկ չկա՝ միայն պարտադիր վճարումները։",
		"advice.remaining":   "Ամսվա վերջում կմնա՝ <code>%s</code>",
		"advice.first_clear": "Առաջինը կփակվի «%s»-ը՝ %s-ին։",

		"advice.result":              "📊 Արդյունք",
		"advice.months_interest":     "Ավարտ՝ <code>%s</code> (%d ամիս) · ընդհանուր տոկոս՝ <code>%s</code>",
		"advice.vs_minimum":          "Միայն պարտադիրը վճարելու համեմատ՝ խնայում եք <code>%s</code> և ավարտում %d ամիս շուտ։",
		"advice.fees":                "Դրանից վաղաժամկետ մարման վճարներ՝ <code>%s</code>։",
		"advice.step_fee":            "վճար՝ %s",
		"advice.assumed":             "Հաշվարկը ենթադրում է, որ մինչև այսօր %d վճարում կատարվել է ժամանակին։",
		"advice.strength.proven":     "Ապացուցված լավագույնն է նշված ենթադրությունների դեպքում. ստուգվել է %d տարբերակ։",
		"advice.strength.exhaustive": "Լավագույնն է բոլոր հերթականությունների և ժամկետների մեջ. ստուգվել է %d տարբերակ։ Ամսից ամիս փոխվող դասավորություններ չեն դիտարկվել։",
		"advice.strength.bounded":    "Լավագույն գտնվածն է սահմանափակ որոնման մեջ (%d տարբերակ). վճարների պատճառով ապացույց չկա։",
		"advice.strength.named":      "Հայտնի ռազմավարությունների համեմատություն է (%d)՝ առանց լավագույնի պնդման։",
		"advice.effect.mixed":        "Ենթադրություն՝ վարկերի մի մասի համար վճարը մնում է նույնը, մյուսների համար՝ վերահաշվարկվում է։",
		"advice.trust_caveat":        "Թվերը հիմնված են ձեր մուտքագրածի վրա. ստուգեք բանկի քաղվածքով, և ես կուղղեմ պլանը։",
		"advice.refuse.too_many":     "Մեկ պլանում առավելագույնը %d վարկ է։ Արխիվացրեք ավելորդները, և կվերադառնամ պլանով։",
		"advice.refuse.infeasible":   "%s-ին պահանջվում է %s, և բյուջեն չի բավականացնում %s-ով։ Բարձրացրեք բյուջեն կամ նշեք աշխատավարձի օրը։",
		"advice.refuse.unsupported":  "Այս պայմանագրի մի մասը դեռ չեմ կարող հաշվել՝ %s։ Ես չեմ գուշակում։",
		"advice.refuse.horizon":      "Վարկերը չեն փակվում 50 տարվա ընթացքում այս բյուջեով։ Ստուգեք տվյալները։",
		"advice.refuse.calculation":  "Հաշվարկը ձախողվեց, և ես չեմ ցուցադրի կասկածելի թիվ։ Խնդիրն արձանագրված է։",
		"relief.prompt":              "Հիմա պարտադիր ամսական վճարը %s %s է։ Ո՞ր գումարից ցածր եք ուզում իջեցնել այն։ Գրեք թիվը։",
		"relief.not_a_number":        "Գրեք գումարը թվով, օրինակ՝ 90000։",
		"advice.ladder_intro":        "Ավելի մեծ բյուջեով՝",
		"advice.ladder":              "• <code>%s</code>/ամիս → ավարտ %s, տոկոս՝ <code>%s</code>",
		"advice.budget_for":          "Մինչև %s ավարտելու համար պետք է <code>%s</code>/ամիս։",
		"advice.relief.head":         "Պարտադիր ամսական վճարը <code>%s</code>-ից կիջնի <code>%s</code>-ի՝ %d-րդ ամսից։",
		"advice.relief.none":         "Այս բյուջեով շուտով թեթևացում չի լինի․ մինչև առաջին վարկի փակումը վճարը նույնն է։",
		"advice.relief.vs_cheapest":  "Դա <code>%s</code>-ով թանկ է ամենաէժան տարբերակից, որը կավարտվեր %s-ին։",

		"advice.how":            "🔍 Ինչու",
		"advice.vs_avalanche":   "Ամենաբարձր տոկոսից սկսելուց էժան է <code>%s</code>-ով։",
		"advice.vs_snowball":    "Ամենափոքր մնացորդից սկսելուց էժան է <code>%s</code>-ով։",
		"advice.timing":         "Դրանից <code>%s</code>-ը միայն վճարման օրվա ընտրությունից է։",
		"advice.effect.shorten": "Ենթադրություն՝ վաղաժամկետ մարումից հետո վճարը մնում է նույնը, ժամկետը կրճատվում է։",
		"advice.effect.reduce":  "Ենթադրություն՝ վաղաժամկետ մարումից հետո բանկը վերահաշվարկում (նվազեցնում) է վճարը։",
		"advice.tie.one_loan":   "Մեկ վարկ է՝ հերթականությունը նշանակություն չունի։",
		"advice.tie.no_surplus": "Ավելցուկ չկա՝ բոլոր տարբերակները վճարում են միայն պարտադիրը։",
		"advice.tie.same_order": "Ամենաբարձր տոկոսով վարկը նաև ամենափոքրն է՝ երկու հայտնի ռազմավարությունները համընկնում են։",
		"advice.set_payday":     "Բյուջեում նշեք աշխատավարձի օրը՝ տեսնելու, թե վաղ վճարելը որքան կխնայի։",
		"advice.rule_unknown":   "Ձեր բանկի վաղաժամկետ մարման կանոնը հաստատված չէ, ուստի վաղ վճարման խնայողությունը հաշվված չէ։",
		"advice.pick":           "Այլ նպատա՞կ — ընտրեք ներքևում։",

		"advice.compare":       "⚖️ Համեմատել",
		"advice.compare_title": "⚖️ Համեմատություն",
		"advice.row":           "<code>🏁 %s · 💸 %s</code>",
		"advice.row_flat":      "<code>📆 մինչև %s/ամիս պարտադիր</code>",
		"advice.row_relief":    "<code>📆 %s → %s՝ %d-րդ ամսից</code>",
		"advice.minimum":       "🐢 Միայն պարտադիրը",
		"advice.ties_intro":    "Ինչու են տարբերակները համընկնում",
		"advice.compare_pick":  "Ընտրեք նպատակը՝ ամսվա քայլերը տեսնելու համար։",

		// --- empty states: what is missing and where to fix it ---
		"loans.none":               "Դեռ վարկ չկա։ Ավելացրեք առաջինը, և ես կասեմ՝ ինչ անել։",
		"advice.set_budget":        "Բյուջե դեռ նշված չէ։ Նշեք՝ որքան կարող եք ամսական հատկացնել բոլոր վարկերին, և ես կկազմեմ պլանը։",
		"advice.currency_mismatch": "Բյուջեն %s-ով է, իսկ վարկերը՝ %s-ով։ Նշեք բյուջեն վարկերի արժույթով։",
		"budget.too_low": "Բյուջեն <code>%s</code> է, իսկ պարտադիր վճարումները՝ <code>%s</code>։ " +
			"Պլան չեմ կարող առաջարկել՝ այն կհանգեցնի ժամկետանց պարտքի։ Մեծացրեք բյուջեն կամ ստուգեք վարկերը։",

		// --- loans ---
		"loans.title":       "📋 Ձեր վարկերը",
		"loan.balance":      "Մնացորդ՝ <code>%s</code> · %s%%",
		"loan.next":         "Հաջորդ վճարումը՝ %s — <code>%s</code>",
		"loan.remaining":    "Մնացած վճարումներ՝ %d",
		"loan.no_schedule":  "Մարման գրաֆիկը հասանելի չէ։",
		"working.intro":     "Ահա հաշվարկը՝ քայլ առ քայլ։",
		"working.check":     "Համեմատեք ձեր բանկի գրաֆիկի հետ։ Եթե տարբերվում է՝ գրեք մեզ, դա մեր սխալն է։",
		"working.button":    "🔍 Հաշվարկը՝ %s",
		"reliability.yours": "Ձեր նշած մնացորդը՝ %s դրությամբ։ Բանկի հաստատումից հետո ավելի ճշգրիտ կլինի։",

		// --- adding a loan ---
		"add.open":        "Բացեք ձևը՝ վարկն ավելացնելու համար։",
		"add.button":      "➕ Ավելացնել վարկ",
		"add.saved":       "✅ Վարկն ավելացված է։",
		"add.invalid":     "Տվյալներն ամբողջական չեն։ Ստուգեք և փորձեք նորից։",
		"add.unavailable": "Ձևն այս պահին հասանելի չէ։ Դա կարգավորման խնդիր է, ոչ թե ձեր։",

		// --- budget ---
		"budget.button": "💰 Նշել բյուջեն",
		"budget.prompt_or_type": "Որքա՞ն կարող եք ամսական հատկացնել բոլոր վարկերին՝ պարտադիր վճարումները ներառյալ։\n\n" +
			"Գրեք գումարը (օրինակ՝ 100000) կամ բացեք ձևը։",
		"budget.required_hint": "Այս ամիս պարտադիրը՝ <code>%s</code>։",
		"budget.not_a_number":  "Չհասկացա գումարը։ Գրեք միայն թիվ, օրինակ՝ 100000։",
		"budget.set":           "✅ Բյուջեն նշված է՝ <code>%s</code>։",
		"budget.set_surplus":   "Ավելցուկ՝ <code>%s</code>/ամիս։",
		"budget.set_low": "⚠️ Բյուջեն նշված է՝ <code>%s</code>, բայց պարտադիր վճարումները <code>%s</code> են։ " +
			"Պլանի համար պետք է առնվազն այդքան։",

		// --- reminders: date, amount, loan -- nothing else ---
		"reminder.due_soon":        "🔔 %s — <code>%s</code> «%s»-ի համար։",
		"reminder.due_today":       "🔔 Այսօր, %s — <code>%s</code> «%s»-ի համար։",
		"reminder.due_soon_plain":  "🔔 %s — վճարում «%s»-ի համար։",
		"reminder.due_today_plain": "🔔 Այսօր, %s — վճարում «%s»-ի համար։",

		// --- language ---
		"language.prompt": "Ընտրեք լեզուն։",
		"language.set":    "Լեզուն փոխված է հայերենի։",

		// --- errors ---
		"error.generic": "Ինչ-որ բան սխալ գնաց։ Փորձեք նորից մի փոքր ուշ։",
	},

	EN: {
		"start.greeting": "Hi, I am Marum 👋\n\n" +
			"1️⃣ You add your loans\n" +
			"2️⃣ You set a monthly budget\n" +
			"3️⃣ I tell you whom, when and how much to pay so the interest is lowest",
		"start.next":     "👇 Start by tapping “➕ Add a loan”.",
		"start.language": "🌐 Հայերե՞ն — սեղմեք «🌐 Language»։",
		"start.no_ai":    "No guessing: every figure comes from a published formula over your own data.",

		"help.title": "❓ How Marum works",
		"help.intro": "You add your loans and a budget. I check every order and timing of paying " +
			"them and say which is best for your goal.",
		"help.goals":         "Three goals:",
		"help.goal.cheapest": "💸 <b>Least interest</b> — pay the bank the least overall.",
		"help.goal.soonest":  "🏁 <b>Finish soonest</b> — clear every loan as early as possible, and see what a bigger budget would buy.",
		"help.goal.relief":   "🌬 <b>Ease each month</b> — once the first loan clears, pay less every month.",
		"help.commands":      "Commands:",
		"help.add":           "/add — add a loan",
		"help.advice":        "/advice — what to do this month",
		"help.loans":         "/loans — my loans",
		"help.budget":        "/budget — monthly budget",
		"help.language":      "/language — change language",
		"help.help":          "/help — this explanation",

		"menu.add":      "Add a loan",
		"menu.advice":   "What to do this month",
		"menu.loans":    "My loans",
		"menu.budget":   "Monthly budget",
		"menu.language": "Language / Լեզուն",
		"menu.help":     "How it works",

		"btn.add":        "➕ Add a loan",
		"btn.advice":     "💡 What to do",
		"btn.loans":      "📋 My loans",
		"btn.budget":     "💰 Budget",
		"btn.language":   "🌐 Language",
		"btn.help":       "❓ How it works",
		"btn.plan":       "💡 See my plan",
		"btn.manage":     "✏️ Manage loans",
		"kb.placeholder": "Choose an action",

		"advice.title":    "💡 Your plan",
		"advice.currency": "All amounts in %s",
		"advice.owed":     "Total owed",
		"advice.required": "Required this month",
		"advice.budget":   "Budget",
		"advice.payday":   "Payday: the %dth",

		"goal.cheapest": "💸 Least interest",
		"goal.soonest":  "🏁 Finish soonest",
		"goal.relief":   "🌬 Ease each month",
		"goal.first":    "🎯 First win",

		"advice.this_month":  "📌 This month",
		"advice.step_due":    "☐ %s — <code>%s</code> → “%s”",
		"advice.step_extra":  "☐ %s — <code>%s</code> → “%s” ➕ extra",
		"advice.step_early":  "☐ %s — <code>%s</code> → “%s” ⚡ extra, before the due date (saves %s)",
		"advice.no_surplus":  "No surplus: only the required instalments.",
		"advice.remaining":   "Owed at the end of the month: <code>%s</code>",
		"advice.first_clear": "“%s” clears first, on %s.",

		"advice.result":              "📊 Result",
		"advice.months_interest":     "Done on <code>%s</code> (%d months) · total interest <code>%s</code>",
		"advice.vs_minimum":          "Against paying only the minimum you save <code>%s</code> and finish %d months sooner.",
		"advice.fees":                "Of which early-repayment fees: <code>%s</code>.",
		"advice.step_fee":            "fee %s",
		"advice.assumed":             "Assumes %d instalment(s) were paid on time up to today.",
		"advice.strength.proven":     "Proven best under the stated assumptions; %d ways of paying were checked.",
		"advice.strength.exhaustive": "Best of every priority order and timing (%d checked). Switching loans month to month was not explored.",
		"advice.strength.bounded":    "Best found in a bounded search (%d candidates); fees prevent a proof.",
		"advice.strength.named":      "A comparison of well-known strategies (%d); no claim of the best.",
		"advice.effect.mixed":        "Assumes the instalment stays the same on some loans and is re-solved on others.",
		"advice.trust_caveat":        "Figures rest on what you entered; check a bank statement and I will correct the plan.",
		"advice.refuse.too_many":     "A plan covers at most %d loans. Archive the ones that no longer matter and ask again.",
		"advice.refuse.infeasible":   "On %s a payment of %s falls due and the budget is short by %s. Raise the budget or set your payday.",
		"advice.refuse.unsupported":  "Part of this contract is not something I can calculate yet: %s. I will not guess.",
		"advice.refuse.horizon":      "The loans do not clear within 50 years on this budget. Please check the figures.",
		"advice.refuse.calculation":  "The calculation failed and I will not show a doubtful number. The problem has been recorded.",
		"relief.prompt":              "Your required monthly payment is %s %s today. What amount do you want to get under? Type the number.",
		"relief.not_a_number":        "Type the amount as a number, e.g. 90000.",
		"advice.ladder_intro":        "With a bigger budget:",
		"advice.ladder":              "• <code>%s</code>/month → done %s, interest <code>%s</code>",
		"advice.budget_for":          "To be done by %s you would need <code>%s</code>/month.",
		"advice.relief.head":         "Your required monthly payment drops from <code>%s</code> to <code>%s</code> from month %d.",
		"advice.relief.none":         "This budget brings no relief soon: the payment stays the same until the first loan clears.",
		"advice.relief.vs_cheapest":  "That costs <code>%s</code> more than the cheapest plan, which would finish on %s.",

		"advice.how":            "🔍 Why",
		"advice.vs_avalanche":   "Cheaper than highest-rate-first by <code>%s</code>.",
		"advice.vs_snowball":    "Cheaper than smallest-balance-first by <code>%s</code>.",
		"advice.timing":         "Of that, <code>%s</code> comes only from when you pay.",
		"advice.effect.shorten": "Assumes the instalment stays the same after an early payment and the term shortens.",
		"advice.effect.reduce":  "Assumes the bank re-solves (lowers) the instalment after an early payment.",
		"advice.tie.one_loan":   "One loan: the order cannot matter.",
		"advice.tie.no_surplus": "No surplus: every plan pays only what is required.",
		"advice.tie.same_order": "The highest-rate loan is also the smallest, so both well-known strategies coincide.",
		"advice.set_payday":     "Set your payday in the budget to see what paying early saves.",
		"advice.rule_unknown":   "Your bank’s early-repayment rule is not confirmed, so no early-payment saving is counted.",
		"advice.pick":           "Another goal? Pick one below.",

		"advice.compare":       "⚖️ Compare",
		"advice.compare_title": "⚖️ Comparison",
		"advice.row":           "<code>🏁 %s · 💸 %s</code>",
		"advice.row_flat":      "<code>📆 up to %s/month required</code>",
		"advice.row_relief":    "<code>📆 %s → %s from month %d</code>",
		"advice.minimum":       "🐢 Minimum only",
		"advice.ties_intro":    "Why the options coincide",
		"advice.compare_pick":  "Pick a goal to see this month’s steps.",

		"loans.none":               "No loans yet. Add the first one and I will tell you what to do.",
		"advice.set_budget":        "No budget yet. Tell me how much you can put towards all your loans each month and I will build the plan.",
		"advice.currency_mismatch": "Your budget is in %s but your loans are in %s. Set the budget in the loans’ currency.",
		"budget.too_low": "Your budget is <code>%s</code> but the required instalments come to <code>%s</code>. " +
			"I cannot propose a plan that puts you into arrears. Raise the budget or check the loans.",

		"loans.title":       "📋 Your loans",
		"loan.balance":      "Balance <code>%s</code> · %s%%",
		"loan.next":         "Next instalment: %s — <code>%s</code>",
		"loan.remaining":    "Instalments remaining: %d",
		"loan.no_schedule":  "The repayment schedule is not available.",
		"working.intro":     "Here is the calculation, step by step.",
		"working.check":     "Compare it with your bank’s schedule. If it differs, tell us: that is our mistake to fix.",
		"working.button":    "🔍 Calculation: %s",
		"reliability.yours": "Your own figure, as of %s. It will be firmer once your bank confirms it.",

		"add.open":        "Open the form to add a loan.",
		"add.button":      "➕ Add a loan",
		"add.saved":       "✅ Loan added.",
		"add.invalid":     "Those details are incomplete. Check them and try again.",
		"add.unavailable": "The form is not available right now. That is a configuration problem, not yours.",

		"budget.button": "💰 Set a budget",
		"budget.prompt_or_type": "How much can you put towards all your loans each month, required instalments included?\n\n" +
			"Type the amount (for example 100000) or open the form.",
		"budget.required_hint": "Required this month: <code>%s</code>.",
		"budget.not_a_number":  "I could not read that amount. Type just a number, for example 100000.",
		"budget.set":           "✅ Budget set: <code>%s</code>.",
		"budget.set_surplus":   "Surplus: <code>%s</code>/month.",
		"budget.set_low": "⚠️ Budget set to <code>%s</code>, but the required instalments come to <code>%s</code>. " +
			"A plan needs at least that much.",

		"reminder.due_soon":        "🔔 %s — <code>%s</code> for “%s”.",
		"reminder.due_today":       "🔔 Today, %s — <code>%s</code> for “%s”.",
		"reminder.due_soon_plain":  "🔔 %s — instalment for “%s”.",
		"reminder.due_today_plain": "🔔 Today, %s — instalment for “%s”.",

		"language.prompt": "Choose a language.",
		"language.set":    "Language changed to English.",

		"error.generic": "Something went wrong. Please try again shortly.",
	},
}
