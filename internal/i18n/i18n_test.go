package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The guard that matters. A key present in one language and missing in another
// renders as either a raw key or a sentence in the wrong language, mid
// conversation, in an application about money.
func TestEveryKeyIsTranslated(t *testing.T) {
	keys := Keys()
	if len(keys) == 0 {
		t.Fatal("the catalogue is empty")
	}
	for _, l := range Supported() {
		for _, k := range keys {
			v, ok := catalogue[l][k]
			if !ok {
				t.Errorf("%s is missing %q", l, k)
				continue
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s has %q empty", l, k)
			}
		}
	}
}

// A format string with a different number of verbs in one language than another
// produces %!s(MISSING) in production, in exactly one locale, which is the kind
// of bug that reaches a user rather than a reviewer.
func TestFormatVerbsAgreeAcrossLocales(t *testing.T) {
	for _, k := range Keys() {
		want := strings.Count(catalogue[Default][k], "%s") + strings.Count(catalogue[Default][k], "%d")
		for _, l := range Supported() {
			got := strings.Count(catalogue[l][k], "%s") + strings.Count(catalogue[l][k], "%d")
			if got != want {
				t.Errorf("%q: %s has %d verbs, %s has %d", k, l, got, Default, want)
			}
		}
	}
	// %% is an escaped percent sign, not a verb; make sure none crept in as one.
	for _, l := range Supported() {
		for _, k := range Keys() {
			if strings.Contains(catalogue[l][k], "% ") {
				t.Errorf("%q in %s contains a stray percent sign", k, l)
			}
		}
	}
}

func TestParseFallsBackToArmenian(t *testing.T) {
	for tag, want := range map[string]Locale{
		"hy": HY, "hy-AM": HY, "en": EN, "en-GB": EN, "EN": EN,
		"ru": Default, "fr": Default, "": Default, "nonsense": Default,
	} {
		if got := Parse(tag); got != want {
			t.Errorf("Parse(%q) = %s, want %s", tag, got, want)
		}
	}
}

func TestUnknownKeyReturnsTheKey(t *testing.T) {
	if got := T(HY, "no.such.key"); got != "no.such.key" {
		t.Errorf("got %q, want the key back", got)
	}
}

// Armenian is the default, and the default must be a language the catalogue
// actually has. A default pointing at an absent locale would make every message
// fall through to its key.
func TestDefaultLocaleIsPopulated(t *testing.T) {
	if len(catalogue[Default]) == 0 {
		t.Fatalf("default locale %s has no messages", Default)
	}
	if !Default.Valid() {
		t.Fatalf("default locale %s is not valid", Default)
	}
}

// A tapped keyboard button arrives as ordinary text, so every button label must
// map back to the command it stands for -- in EVERY locale, not just the
// current one. A user who switches language still has the old keyboard on
// screen until they tap something.
func TestEveryButtonLabelMatchesBack(t *testing.T) {
	for _, l := range Supported() {
		for kind := range buttonKeys {
			label := Button(l, kind)
			got, ok := MatchButton(label)
			if !ok {
				t.Errorf("%s button %q (%s) does not match back", l, label, kind)
				continue
			}
			if got != kind {
				t.Errorf("%s button %q matched %q, want %q", l, label, got, kind)
			}
		}
	}
}

// Labels must be unique across languages, or a tap is ambiguous.
func TestButtonLabelsAreUnambiguous(t *testing.T) {
	seen := map[string]string{}
	for _, l := range Supported() {
		for kind := range buttonKeys {
			label := Button(l, kind)
			if prev, dup := seen[label]; dup && prev != kind {
				t.Errorf("label %q means both %q and %q", label, prev, kind)
			}
			seen[label] = kind
		}
	}
}

func TestMatchButtonIgnoresOrdinaryText(t *testing.T) {
	for _, s := range []string{"", "   ", "hello", "/loans", "150000"} {
		if kind, ok := MatchButton(s); ok {
			t.Errorf("MatchButton(%q) matched %q; ordinary text must not", s, kind)
		}
	}
}

// Every key the application asks for must exist. The completeness test above
// checks that defined keys exist in every language; it cannot notice a key that
// is USED but never defined, and that is exactly what shipped: a report whose
// every line rendered as "advice.months". This walks the Go source for T(...)
// calls and refuses any key the catalogue does not know.
func TestEveryUsedKeyIsDefined(t *testing.T) {
	root := filepath.Join("..", "..")
	re := regexp.MustCompile(`i18n\.(?:T|Button)\([^,]+,\s*"([a-z][a-z0-9_.]*)"`)
	known := map[string]bool{}
	for _, k := range Keys() {
		known[k] = true
	}
	var missing []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if !known[m[1]] {
				missing = append(missing, m[1]+"  ("+filepath.Base(p)+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("used but not in the catalogue: %s", m)
	}
}
