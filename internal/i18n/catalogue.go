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
		"start.next":     "👇 Սկսենք՝ բացեք «📱 Գլխավոր»-ը և ավելացրեք առաջին վարկը։",
		"start.language": "🌐 Prefer English? Tap «🌐 Լեզուն»։",
		"start.no_ai":    "Ոչ մի կռահում․ ամեն թիվ հաշվարկվում է հրապարակված բանաձևով՝ ձեր տվյալներից։",

		// --- how it works ---
		"help.title": "❓ Ինչպե՞ս է աշխատում Մարումը",
		"help.intro": "Դուք ավելացնում եք վարկերն ու բյուջեն։ Ես ստուգում եմ վճարման բոլոր " +
			"հերթականություններն ու ժամկետները և ասում՝ որն է լավագույնը ձեր նպատակի համար։",
		"help.goals":         "Չորս նպատակ՝",
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
		"btn.dashboard":  "📱 Գլխավոր",
		"btn.advice":     "💡 Ի՞նչ անել",
		"btn.loans":      "📋 Իմ վարկերը",
		"btn.budget":     "💰 Բյուջե",
		"btn.language":   "🌐 Լեզուն",
		"btn.help":       "❓ Ինչպե՞ս է աշխատում",
		"btn.plan":       "💡 Տեսնել պլանը",
		"btn.manage":     "✏️ Կառավարել վարկերը",
		"kb.placeholder": "Ընտրեք գործողություն",
		// --- the journey: one tip per message, the next thing worth doing ---
		"tip.add":                "Ավելացրեք առաջին վարկը («📱 Գլխավոր»)․ ես կարող եմ պլանավորել միայն այն, ինչ տեսնում եմ։",
		"tip.budget":             "Նշեք ամսական բյուջեն («💰 Բյուջե»)․ առանց դրա պլան չկա։",
		"tip.payday":             "Բյուջեում ավելացրեք աշխատավարձի օրը․ վաղ վճարելը տոկոս է խնայում, և ես ցույց կտամ՝ որքան։",
		"tip.confirm":            "Համեմատեք մնացորդը բանկի քաղվածքի հետ («🔍 Հաշվարկը»)․ ճշգրտված թվերով պլանն ավելի վստահելի է։",
		"tip.pro.early":          "Ավելցուկը վճարեք աշխատավարձի օրը, ոչ թե վճարման օրը․ ամեն օրը տոկոս է։",
		"tip.pro.compare":        "«⚖️ Համեմատել»-ը ցույց է տալիս բոլոր նպատակները կողք կողքի՝ գնով ու ժամկետով։",
		"tip.pro.first_win":      "«🎯 Առաջին հաղթանակ»-ը ամենաարագ ճանապարհն է մեկ վարկից ազատվելու։",
		"tip.pro.working":        "Կասկածու՞մ եք թվին․ «🔍 Հաշվարկը» ցույց է տալիս ամեն տողի բանաձևը։",
		"tip.pro.update_balance": "Վաղաժամկետ մարումից հետո թարմացրեք մնացորդը («✏️ Կառավարել վարկերը»)․ պլանը կհետևի։",
		"tip.pro.relief":         "Եթե ամիսը ծանր է՝ «🌬 Ամսական թեթևացում»-ը ցույց կտա, երբ վճարը կիջնի ձեր նշած սահմանից։",
		"start.reminders":        "Ամեն վճարումից 3 օր առաջ և վճարման օրը կհիշեցնեմ։",
		"help.goal.first":        "🎯 <b>Առաջին հաղթանակ</b> — որևէ մեկ վարկ փակել հնարավորինս շուտ։",
		"help.compare":           "⚖️ <b>Համեմատել</b> — բոլոր նպատակները կողք կողքի, գնով ու ժամկետով։",
		"help.reminders":         "🔔 Հիշեցումներ՝ ամեն վճարումից 3 օր առաջ և վճարման օրը, ժամը 10:00-ին։",
		"add.saved_chat":         "✅ Վարկը պահպանված է․ հիշեցումները միացված են։",

		// --- the advice report ---
		"advice.title": "💡 Ձեր պլանը",
		// Short month names, the way a date is said aloud.
		"month.1": "հնվ", "month.2": "փտվ", "month.3": "մրտ", "month.4": "ապր",
		"month.5": "մյս", "month.6": "հնս", "month.7": "հլս", "month.8": "օգս",
		"month.9": "սեպ", "month.10": "հոկ", "month.11": "նոյ", "month.12": "դեկ",
		"advice.header":       "Պարտք՝ <code>%s</code> · Բյուջե՝ <code>%s</code>/ամիս · %s",
		"advice.promise":      "🏁 Պարտքից ազատ՝ <b>%s</b> — %d ամսում։",
		"advice.footer":       "Պարտք՝ <code>%s</code> · վճարվելիք տոկոս՝ <code>%s</code> · %s",
		"advice.cost_fees":    "(+ վճարներ՝ <code>%s</code>)",
		"advice.ladder_hint":  "⚡ <code>%s</code>/ամիս դեպքում կավարտեիք %s-ին — մանրամասները՝ «🔍 Ինչու»-ում։",
		"advice.why_button":   "🔍 Ինչու՞",
		"plan.approve_button": "✅ Հաստատել այս պլանը",
		"plan.sheet_button":   "📄 Ամբողջ գրաֆիկը",
		"plan.approved":       "✅ Պլանը հաստատված է՝ %s։\nԱվարտ՝ %s · ընդհանուր տոկոս՝ <code>%s</code>։\nԱմեն ամիս կհիշեցնեմ քայլերը․ վարկերը փոխվելուց պլանը կթարմացվի։",
		"plan.badge":          "✅ Այս պլանը հաստատել եք․ ավարտը՝ %s։",
		"advice.owed":         "Ընդհանուր պարտք",
		"advice.required":     "Այս ամիս պարտադիր",

		"goal.cheapest": "💸 Ամենաքիչ տոկոս",
		"goal.soonest":  "🏁 Ամենաշուտ ավարտ",
		"goal.relief":   "🌬 Ամսական թեթևացում",
		"goal.first":    "🎯 Առաջին հաղթանակ",

		"advice.this_month":       "Այս ամիս — <code>%s</code>",
		"advice.extra_note":       "⚡ լրացուցիչ վճարում՝ վաղ — խնայում է <code>%s</code>",
		"advice.extra_note_plain": "⚡ լրացուցիչ վճարում պարտադիրից բացի",
		"advice.no_surplus":       "Ավելցուկ չկա՝ միայն պարտադիր վճարումները։",
		"advice.first_clear":      "Առաջինը կփակվի «%s»-ը՝ %s-ին։",

		"advice.vs_minimum":          "<b>Խնայում եք <code>%s</code> · %d ամիս շուտ</b>, քան միայն պարտադիրը վճարելիս։",
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
		"advice.refuse.stale":        "Մնացորդը նշված է %s-ին, և այդ պահից շատ վճարումներ են անցել։ Թարմացրեք մնացորդը («📋 Իմ վարկերը»), և ես կհաշվեմ իրական թվերով։",
		"advice.refuse.calculation":  "Հաշվարկը ձախողվեց, և ես չեմ ցուցադրի կասկածելի թիվ։ Խնդիրն արձանագրված է։",
		"relief.prompt":              "Հիմա պարտադիր ամսական վճարը %s %s է։ Ո՞ր գումարից ցածր եք ուզում իջեցնել այն։ Գրեք թիվը։",
		"relief.not_a_number":        "Գրեք գումարը թվով, օրինակ՝ 90000։",
		"reminder.also":              "Այս օրը նաև՝ <code>%s</code> → «%s»։",
		"advice.ladder_intro":        "Ավելի մեծ բյուջեով՝",
		"advice.ladder":              "• <code>%s</code>/ամիս → ավարտ %s, տոկոս՝ <code>%s</code>",
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
		"advice.tie.no_payday":  "Աշխատավարձի օրը նշված չէ՝ վաղ վճարումը չի հաշվարկվել։",
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
		"reminder.paid_button":     "✓ Վճարել եմ",

		// --- the paid flow: confirmed balances are what reset drift ---
		"paid.ask_balance": "Գրանցե՞նք։ Գրեք, թե բանկի հավելվածն ինչ մնացորդ է ցույց տալիս «%s»-ի համար։ " +
			"Կարող եք նաև բաց թողնել։",
		"paid.skip_button":    "Բաց թողնել",
		"paid.skipped":        "👍 Լավ է, որ վճարել եք։",
		"paid.not_a_number":   "Չհասկացա գումարը։ Գրեք միայն թիվ, օրինակ՝ 950000, կամ սեղմեք «Բաց թողնել»։",
		"paid.wrong_currency": "Այս վարկը %s-ով է։ Գրեք մնացորդը նույն արժույթով։",
		"paid.saved":          "✅ «%s»-ի մնացորդը թարմացված է՝ <code>%s</code>։",
		"paid.cleared":        "🎉 «%s»-ը մարված է։ Շնորհավո՛ր։",

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
		"start.next":     "👇 Start by opening the 📱 Dashboard and adding your first loan.",
		"start.language": "🌐 Հայերե՞ն — սեղմեք «🌐 Language»։",
		"start.no_ai":    "No guessing: every figure comes from a published formula over your own data.",

		"help.title": "❓ How Marum works",
		"help.intro": "You add your loans and a budget. I check every order and timing of paying " +
			"them and say which is best for your goal.",
		"help.goals":         "Four goals:",
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

		"btn.dashboard":  "📱 Dashboard",
		"btn.advice":     "💡 What to do",
		"btn.loans":      "📋 My loans",
		"btn.budget":     "💰 Budget",
		"btn.language":   "🌐 Language",
		"btn.help":       "❓ How it works",
		"btn.plan":       "💡 See my plan",
		"btn.manage":     "✏️ Manage loans",
		"kb.placeholder": "Choose an action",
		// --- the journey: one tip per message, the next thing worth doing ---
		"tip.add":                "Add your first loan (📱 Dashboard): I can only plan what I can see.",
		"tip.budget":             "Set a monthly budget (💰 Budget): without one there is no plan.",
		"tip.payday":             "Add your payday in the budget: paying early saves interest, and I will show how much.",
		"tip.confirm":            "Check the balance against a bank statement (🔍 The maths): confirmed figures make the plan firmer.",
		"tip.pro.early":          "Pay the surplus on payday, not on the due date: every day costs interest.",
		"tip.pro.compare":        "⚖️ Compare shows every goal side by side, with its cost and finish date.",
		"tip.pro.first_win":      "🎯 First win is the fastest way to be rid of one loan entirely.",
		"tip.pro.working":        "Doubt a number? 🔍 The maths shows the formula behind every row.",
		"tip.pro.update_balance": "After an early repayment, update the balance (✏️ Manage loans): the plan will follow.",
		"tip.pro.relief":         "Heavy month? 🌬 Ease each month shows when the payment drops below a cap you choose.",
		"start.reminders":        "I will remind you 3 days before every instalment and on the day.",
		"help.goal.first":        "🎯 <b>First win</b> — close any one loan as soon as possible.",
		"help.compare":           "⚖️ <b>Compare</b> — every goal side by side, with its cost and finish date.",
		"help.reminders":         "🔔 Reminders: 3 days before every instalment and on the day, at 10:00.",
		"add.saved_chat":         "✅ Loan saved; reminders are on.",

		"advice.title": "💡 Your plan",
		// Short month names, the way a date is said aloud.
		"month.1": "Jan", "month.2": "Feb", "month.3": "Mar", "month.4": "Apr",
		"month.5": "May", "month.6": "Jun", "month.7": "Jul", "month.8": "Aug",
		"month.9": "Sep", "month.10": "Oct", "month.11": "Nov", "month.12": "Dec",
		"advice.header":       "Owed <code>%s</code> · Budget <code>%s</code>/month · %s",
		"advice.promise":      "🏁 Debt-free <b>%s</b> — in %d months.",
		"advice.footer":       "Owed <code>%s</code> · interest to pay <code>%s</code> · %s",
		"advice.cost_fees":    "(+ fees <code>%s</code>)",
		"advice.ladder_hint":  "⚡ At <code>%s</code>/month you would finish %s — details under 🔍 Why.",
		"advice.why_button":   "🔍 Why",
		"plan.approve_button": "✅ Approve this plan",
		"plan.sheet_button":   "📄 Full schedule",
		"plan.approved":       "✅ Plan approved: %s.\nDone %s · total interest <code>%s</code>.\nI will remind you of each month's steps; the plan follows your loans as they change.",
		"plan.badge":          "✅ You approved this plan; it finishes %s.",
		"advice.owed":         "Total owed",
		"advice.required":     "Required this month",

		"goal.cheapest": "💸 Least interest",
		"goal.soonest":  "🏁 Finish soonest",
		"goal.relief":   "🌬 Ease each month",
		"goal.first":    "🎯 First win",

		"advice.this_month":       "This month — <code>%s</code>",
		"advice.extra_note":       "⚡ extra, paid early — saves <code>%s</code>",
		"advice.extra_note_plain": "⚡ extra payment beyond the instalment",
		"advice.no_surplus":       "No surplus: only the required instalments.",
		"advice.first_clear":      "“%s” clears first, on %s.",

		"advice.vs_minimum":          "<b>Saves <code>%s</code> · %d months sooner</b> than paying only the minimum.",
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
		"advice.refuse.stale":        "The balance was stated on %s, and too many instalments have passed since. Update the balance (“📋 My loans”) and I will plan on real figures.",
		"advice.refuse.calculation":  "The calculation failed and I will not show a doubtful number. The problem has been recorded.",
		"relief.prompt":              "Your required monthly payment is %s %s today. What amount do you want to get under? Type the number.",
		"relief.not_a_number":        "Type the amount as a number, e.g. 90000.",
		"reminder.also":              "Also due this day: <code>%s</code> → “%s”.",
		"advice.ladder_intro":        "With a bigger budget:",
		"advice.ladder":              "• <code>%s</code>/month → done %s, interest <code>%s</code>",
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
		"advice.tie.no_payday":  "No payday given, so early payment was not simulated.",
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
		"reminder.paid_button":     "✓ I paid",

		"paid.ask_balance": "Record it? Type the balance your bank app now shows for “%s”. " +
			"You can also skip this.",
		"paid.skip_button":    "Skip",
		"paid.skipped":        "👍 Good — the payment is what matters.",
		"paid.not_a_number":   "I could not read that as an amount. Type just the number, like 950000, or tap Skip.",
		"paid.wrong_currency": "This loan is in %s. Type the balance in the same currency.",
		"paid.saved":          "✅ “%s” updated: <code>%s</code> owed.",
		"paid.cleared":        "🎉 “%s” is paid off. Congratulations!",

		"language.prompt": "Choose a language.",
		"language.set":    "Language changed to English.",

		"error.generic": "Something went wrong. Please try again shortly.",
	},
}
