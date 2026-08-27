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
		"add.open":    "Բացեք ձևը՝ վարկն ավելացնելու համար։",
		"add.button":  "Ավելացնել վարկ",
		"add.saved":   "Վարկն ավելացված է։",
		"add.invalid": "Այս տվյալները ամբողջական չեն։ Ստուգեք և փորձեք նորից։",

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

		"add.open":    "Open the form to add a loan.",
		"add.button":  "Add a loan",
		"add.saved":   "Loan added.",
		"add.invalid": "Those details are incomplete. Check them and try again.",

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
