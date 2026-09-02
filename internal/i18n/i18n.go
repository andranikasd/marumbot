// Package i18n holds every string shown to a borrower, in every language Marum
// speaks.
//
// The catalogue is a map per locale rather than files loaded at runtime, so a
// missing translation is a test failure rather than a message that renders as a
// key in front of a user. There is no fallback chain that silently substitutes
// English either: a half-translated interface is worse than an honestly English
// one, because the user cannot tell which parts they are reading correctly.
//
// Armenian is the default. Marum is built for Armenia first, and the schema
// agrees -- users.locale defaults to 'hy'.
package i18n

import (
	"fmt"
	"sort"
	"strings"
)

// Locale is a language Marum speaks.
type Locale string

// The supported locales. The users table constrains its column to these.
const (
	HY Locale = "hy" // Armenian, the default
	EN Locale = "en" // English
)

// Default is used when a user has expressed no preference.
const Default = HY

// Supported lists the locales in the order they are offered.
func Supported() []Locale { return []Locale{HY, EN} }

// Parse maps a Telegram language_code to a locale Marum speaks.
//
// Telegram sends IETF tags like "hy-AM" or "en-GB", and anything unrecognised
// falls back to Armenian rather than English: an Armenian user with an unusual
// phone locale is far more likely here than an English speaker who set one.
func Parse(tag string) Locale {
	switch strings.ToLower(strings.SplitN(tag, "-", 2)[0]) {
	case "en":
		return EN
	case "hy":
		return HY
	default:
		return Default
	}
}

// Name is the locale's own name, for a language picker. A picker that lists
// languages in a language the reader does not speak is not a picker.
func (l Locale) Name() string {
	if l == EN {
		return "English"
	}
	return "Հայերեն"
}

// Valid reports whether the locale is one Marum speaks.
func (l Locale) Valid() bool {
	return l == HY || l == EN
}

// T returns the message for a key, formatted with args.
//
// An unknown key returns the key itself rather than panicking. A missing string
// should be visible and ugly in one message, not fatal to a conversation the
// user is halfway through.
func T(l Locale, key string, args ...any) string {
	if !l.Valid() {
		l = Default
	}
	m, ok := catalogue[l][key]
	if !ok {
		if m, ok = catalogue[Default][key]; !ok {
			return key
		}
	}
	if len(args) == 0 {
		return m
	}
	return fmt.Sprintf(m, args...)
}

// Keys lists every message key, sorted. Used by the completeness test.
func Keys() []string {
	seen := map[string]struct{}{}
	for _, m := range catalogue {
		for k := range m {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// kindAdd is the command kind behind the Mini App entry point; it recurs in
// the legacy label map below.
const kindAdd = "add"

// buttonKeys maps a command kind to the catalogue key labelling its button.
var buttonKeys = map[string]string{
	kindAdd:    "btn.dashboard",
	"loans":    "btn.loans",
	"budget":   "btn.budget",
	"language": "btn.language",
	"advice":   "btn.advice",
	"help":     "btn.help",
}

// DashboardButton labels the persistent Mini App entry point. It is not an
// "add loan" command: the app opens on the loans overview and exposes every
// workflow from there.
func DashboardButton(l Locale) string { return T(l, "btn.dashboard") }

// Button returns the label for a command in a locale.
func Button(l Locale, kind string) string {
	if k, ok := buttonKeys[kind]; ok {
		return T(l, k)
	}
	return kind
}

// MatchButton maps a tapped button back to the command it stands for.
//
// A reply-keyboard button sends its own label as an ordinary message, so the
// bot receives "Իմ վարկերը" rather than "/loans". Matching across EVERY locale,
// not just the user's current one, matters: someone who switches language
// mid-conversation still has the old keyboard on screen until they tap
// something, and the tap that arrives will carry the old label.
func MatchButton(text string) (kind string, ok bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", false
	}
	for _, l := range Supported() {
		for k := range buttonKeys {
			if T(l, buttonKeys[k]) == t {
				return k, true
			}
		}
	}
	// Labels that shipped and were later renamed. A keyboard drawn last month
	// is still on someone's screen, and its tap must keep meaning what it
	// meant when it was drawn.
	if k, ok := legacyButtons[t]; ok {
		return k, true
	}
	return "", false
}

// legacyButtons maps retired labels to their commands.
var legacyButtons = map[string]string{
	"➕ Ավելացնել վարկ":     kindAdd,
	"➕ Add a loan":         kindAdd,
	"📱 Գլխավոր":            kindAdd,
	"📱 Dashboard":          kindAdd,
	"💡 Ի՞նչ անել":          "advice",
	"💡 What to do":         "advice",
	"📋 Իմ վարկերը":         "loans",
	"📋 My loans":           "loans",
	"💰 Բյուջե":             "budget",
	"💰 Budget":             "budget",
	"🌐 Լեզուն":             "language",
	"🌐 Language":           "language",
	"❓ Ինչպե՞ս է աշխատում": "help",
	"❓ How it works":       "help",
}
