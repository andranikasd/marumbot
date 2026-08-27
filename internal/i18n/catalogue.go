package i18n

// The message catalogue.
//
// Terminology matters more than fluency here. A borrower reading about their
// own debt needs the words their lender used, so these follow standard Armenian
// banking usage: վարկ for a loan, մայր գումար for principal, տոկոսադրույք for
// the interest rate, մարման գրաֆիկ for a repayment schedule. "Marum" itself is
// մարում, the repayment of a debt.
//
// Every key must exist in every locale. TestEveryKeyIsTranslated enforces it,
// because a message that falls back to another language mid-conversation is how
// a user stops trusting the numbers around it.
var catalogue = map[Locale]map[string]string{
	HY: {
		// --- greeting and help ---
		"start.greeting": "Բարի գալուստ Մարում 👋\n\n" +
			"Ես կօգնեմ ձեզ հետևել ձեր վարկերին, հիշեցնեմ վճարումների մասին և հաշվարկեմ, " +
			"թե ինչպես մարել դրանք ավելի քիչ գումարով։",
		"start.no_ai": "Ոչ մի կանխատեսում։ Յուրաքանչյուր թիվ հաշվարկվում է " +
			"հրապարակված բանաձևով՝ ձեր մուտքագրած տվյալների հիման վրա։",
		"help.title":    "Հրամաններ",
		"help.start":    "/start — սկսել",
		"help.add":      "/add — ավելացնել վարկ",
		"help.loans":    "/loans — իմ վարկերը",
		"help.budget":   "/budget — ամսական բյուջե",
		"help.language": "/language — լեզուն",
		"help.help":     "/help — այս ցանկը",

		// Command descriptions, shown by Telegram in the "/" menu. Kept short:
		// the client truncates them, and a description that needs a comma is
		// describing too much.
		"menu.add":      "Ավելացնել վարկ",
		"menu.advice":   "Ի՞նչ անել հիմա",
		"menu.loans":    "Իմ վարկերը",
		"menu.budget":   "Ամսական բյուջե",
		"menu.language": "Լեզուն / Language",
		"menu.help":     "Օգնություն",

		// Keyboard buttons. Short enough to sit two to a row on a phone.
		"btn.add":                "➕ Ավելացնել վարկ",
		"btn.loans":              "📋 Իմ վարկերը",
		"btn.budget":             "💰 Բյուջե",
		"btn.language":           "🌐 Լեզուն",
		"btn.help":               "❓ Օգնություն",
		"kb.placeholder":         "Ընտրեք կամ գրեք հրաման",
		"btn.advice":             "💡 Ի՞նչ անել",
		"advice.title":           "Ձեր վիճակը",
		"advice.owed":            "Ընդհանուր պարտք՝ %s",
		"advice.required":        "Այս ամիս պարտադիր՝ %s",
		"advice.budget":          "Ձեր բյուջեն՝ %s",
		"advice.why":             "Մեկ վարկի վրա կենտրոնանալը ավելի էժան է, քան գումարը բաշխելը։",
		"advice.no_surplus":      "Բյուջեն ծածկում է պարտադիր վճարումները առանց ավելցուկի։",
		"advice.set_budget":      "Նշեք ամսական բյուջեն՝ պլան ստանալու համար։",
		"advice.nothing":         "Ակտիվ վարկ չկա։",
		"goal.cheapest":          "💸 Ամենաքիչ տոկոս",
		"goal.soonest":           "🏁 Ամենաշուտ ավարտ",
		"goal.relief":            "🌬 Ամսական թեթևացում",
		"advice.compare":         "⚖️ Համեմատել բոլորը",
		"advice.do":              "Այս ամիս ավելցուկը՝ %s, ուղղեք «%s» վարկին։",
		"advice.remaining":       "Դրանից հետո կմնա՝ %s։",
		"advice.months":          "Բոլոր վարկերը կփակվեն %d ամսում։",
		"advice.interest":        "Ընդհանուր տոկոս մինչև վերջ՝ %s։",
		"advice.first_clear":     "Առաջինը կփակվի «%s»-ը՝ %d-րդ ամսում։",
		"advice.frees":           "Դա կազատի ամսական %s։",
		"advice.payday":          "Աշխատավարձի օր՝ ամսի %d-ին",
		"advice.step_due":        "📅 %s — %s պարտադիր վճար «%s»-ի համար։",
		"advice.step_extra":      "➕ %s — %s լրացուցիչ «%s»-ին։",
		"advice.step_early":      "⚡ %s — %s լրացուցիչ «%s»-ին՝ մինչև վճարման օրը. այս ամիս խնայում է %s։",
		"advice.evaluated":       "Ստուգվել է վճարման %d տարբերակ՝ բոլոր հերթականություններն ու ժամկետները։",
		"advice.evaluated_named": "Ստուգվել է վճարման %d հայտնի ռազմավարություն։",
		"advice.vs_avalanche":    "Ամենաբարձր տոկոսից սկսելու սովորական ձևից էժան է %s-ով։",
		"advice.vs_snowball":     "Ամենափոքր մնացորդից սկսելուց էժան է %s-ով։",
		"advice.timing":          "Դրանից %s-ը միայն վճարման օրվա ընտրությունից է։",
		"advice.set_payday":      "Բյուջեում նշեք աշխատավարձի օրը՝ տեսնելու, թե վաղ վճարումը որքան կխնայի։",
		"advice.rule_unknown":    "Ձեր բանկի վաղաժամկետ մարման կանոնը հաստատված չէ, ուստի վաղ վճարման խնայողությունը չի հաշվվել։",
		"loan.remaining":         "Մնացած վճարումներ՝ %d",
		"loan.no_schedule":       "Գրաֆիկը հասանելի չէ։",
		"working.intro":          "Ահա հաշվարկը՝ քայլ առ քայլ։",
		"working.check":          "Համեմատեք ձեր բանկի գրաֆիկի հետ։ Եթե տարբերվում է, գրեք մեզ՝ դա մեր սխալն է։",
		"working.button":         "🔍 Ինչպե՞ս է հաշվարկվել՝ %s",
		"reliability.yours":      "Ձեր նշած մնացորդը՝ %s դրությամբ։ Բանկի հաստատումից հետո ավելի ճշգրիտ կլինի։",
		"budget.open":            "Բացեք ձևը՝ բյուջեն նշելու համար։",
		"budget.button":          "Նշել բյուջեն",
		"budget.prompt_or_type":  "Որքա՞ն կարող եք ամսական հատկացնել բոլոր վարկերին։\n\nԳրեք գումարը (օրինակ՝ 100000) կամ բացեք ձևը։",
		"budget.not_a_number":    "Չհասկացա գումարը։ Գրեք միայն թիվ, օրինակ՝ 100000։",

		// --- language ---
		"language.prompt": "Ընտրեք լեզուն։",
		"language.set":    "Լեզուն փոխված է հայերենի։",

		// --- loans ---
		"loans.none":     "Դուք դեռ վարկ չեք ավելացրել։ Սեղմեք /add՝ սկսելու համար։",
		"loans.title":    "Ձեր վարկերը",
		"loan.balance":   "Մնացորդ՝ %s",
		"loan.next":      "Հաջորդ վճարումը՝ %s՝ %s",
		"loan.rate":      "Տոկոսադրույք՝ %s%%",
		"loan.principal": "Մայր գումար",
		"loan.interest":  "Տոկոս",
		"loan.total":     "Ընդամենը",
		"loan.schedule":  "Մարման գրաֆիկ",

		// --- adding a loan ---
		"add.open":        "Բացեք ձևը՝ վարկն ավելացնելու համար։",
		"add.button":      "Ավելացնել վարկ",
		"add.saved":       "Վարկն ավելացված է։",
		"add.invalid":     "Այս տվյալները ամբողջական չեն։ Ստուգեք և փորձեք նորից։",
		"add.unavailable": "Ձևը հասանելի չէ այս պահին։ Դա կարգավորման խնդիր է, ոչ թե ձեր։",

		// --- budget ---
		"budget.prompt": "Որքա՞ն կարող եք ամսական հատկացնել բոլոր վարկերին։",
		"budget.set":    "Ամսական բյուջեն սահմանված է՝ %s։",
		"budget.too_low": "Այս գումարը պակաս է պարտադիր վճարումներից (%s)։ " +
			"Մարումը չի կարող առաջարկել պլան, որը հանգեցնում է ժամկետանց պարտքի։",

		// --- reliability, the honest answer ---
		"reliability.blocked": "Ես չեմ կարող վստահորեն հաշվարկել այս մնացորդը։ " +
			"Խնդրում եմ նշեք բանկի հաստատած մնացորդը։",
		"reliability.stale": "Այս թիվը հիմնված է %s-ի տվյալների վրա և կարող է հնացած լինել։",

		// --- errors ---
		"error.generic":  "Ինչ-որ բան սխալ գնաց։ Փորձեք նորից մի փոքր ուշ։",
		"error.not_text": "Ես հասկանում եմ միայն տեքստ և հրամաններ։",
	},

	EN: {
		"start.greeting": "Welcome to Marum 👋\n\n" +
			"I keep track of your loans, remind you before each payment is due, " +
			"and work out how to clear them for the least money.",
		"start.no_ai": "No guessing. Every figure comes from a published formula " +
			"over the data you entered.",
		"help.title":    "Commands",
		"help.start":    "/start — get started",
		"help.add":      "/add — add a loan",
		"help.loans":    "/loans — my loans",
		"help.budget":   "/budget — monthly budget",
		"help.language": "/language — change language",
		"help.help":     "/help — this list",

		"menu.add":      "Add a loan",
		"menu.advice":   "What should I do now",
		"menu.loans":    "My loans",
		"menu.budget":   "Monthly budget",
		"menu.language": "Language / Լեզուն",
		"menu.help":     "Help",

		"btn.add":                "➕ Add a loan",
		"btn.loans":              "📋 My loans",
		"btn.budget":             "💰 Budget",
		"btn.language":           "🌐 Language",
		"btn.help":               "❓ Help",
		"kb.placeholder":         "Choose an option or type a command",
		"btn.advice":             "💡 What should I do",
		"advice.title":           "Where you stand",
		"advice.owed":            "Total owed: %s",
		"advice.required":        "Required this month: %s",
		"advice.budget":          "Your budget: %s",
		"advice.why":             "Concentrating on one loan costs less than spreading the money.",
		"advice.no_surplus":      "Your budget covers the required payments with nothing spare.",
		"advice.set_budget":      "Set a monthly budget to get a plan.",
		"advice.nothing":         "No active loans.",
		"goal.cheapest":          "💸 Least interest",
		"goal.soonest":           "🏁 Finish soonest",
		"goal.relief":            "🌬 Ease each month",
		"advice.compare":         "⚖️ Compare all",
		"advice.do":              "This month, put the surplus of %s towards “%s”.",
		"advice.remaining":       "After that you will owe %s.",
		"advice.months":          "Every loan is cleared in %d months.",
		"advice.interest":        "Total interest to the end: %s.",
		"advice.first_clear":     "“%s” is the first to clear, in month %d.",
		"advice.frees":           "That frees %s every month.",
		"advice.payday":          "Payday: the %dth of the month",
		"advice.step_due":        "📅 %s — %s instalment on “%s”.",
		"advice.step_extra":      "➕ %s — %s extra to “%s”.",
		"advice.step_early":      "⚡ %s — %s extra to “%s”, before the due date: saves %s this month.",
		"advice.evaluated":       "Checked %d ways of paying: every priority order and payment timing.",
		"advice.evaluated_named": "Checked %d well-known strategies.",
		"advice.vs_avalanche":    "Cheaper than highest-rate-first by %s.",
		"advice.vs_snowball":     "Cheaper than smallest-balance-first by %s.",
		"advice.timing":          "Of that, %s comes only from when you pay.",
		"advice.set_payday":      "Set your payday in the budget to see what paying early saves.",
		"advice.rule_unknown":    "Your bank’s early-repayment rule is not confirmed, so no early-payment saving is counted.",
		"loan.remaining":         "Payments remaining: %d",
		"loan.no_schedule":       "A schedule is not available.",
		"working.intro":          "Here is the calculation, step by step.",
		"working.check":          "Compare it with your bank's schedule. If it differs, tell us — that is our mistake to fix.",
		"working.button":         "🔍 How was this calculated: %s",
		"reliability.yours":      "Your own figure, as of %s. It will be firmer once your bank confirms it.",
		"budget.open":            "Open the form to set your budget.",
		"budget.button":          "Set a budget",
		"budget.prompt_or_type":  "How much can you put towards all your loans each month?\n\nType the amount (for example 100000) or open the form.",
		"budget.not_a_number":    "I could not read that amount. Type just a number, for example 100000.",

		"language.prompt": "Choose a language.",
		"language.set":    "Language changed to English.",

		"loans.none":     "You have not added a loan yet. Press /add to start.",
		"loans.title":    "Your loans",
		"loan.balance":   "Balance: %s",
		"loan.next":      "Next payment: %s on %s",
		"loan.rate":      "Rate: %s%%",
		"loan.principal": "Principal",
		"loan.interest":  "Interest",
		"loan.total":     "Total",
		"loan.schedule":  "Repayment schedule",

		"add.open":        "Open the form to add a loan.",
		"add.button":      "Add a loan",
		"add.saved":       "Loan added.",
		"add.invalid":     "Those details are incomplete. Check them and try again.",
		"add.unavailable": "The form is not available right now. That is a configuration problem, not yours.",

		"budget.prompt": "How much can you put towards all your loans each month?",
		"budget.set":    "Monthly budget set to %s.",
		"budget.too_low": "That is less than your required payments (%s). " +
			"Marum will not propose a plan that puts you into arrears.",

		"reliability.blocked": "I cannot reconstruct this balance with confidence. " +
			"Please tell me the balance your bank reports.",
		"reliability.stale": "This figure is based on data from %s and may be out of date.",

		"error.generic":  "Something went wrong. Please try again shortly.",
		"error.not_text": "I only understand text and commands.",
	},
}
