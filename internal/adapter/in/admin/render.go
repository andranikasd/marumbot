package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// funcs are the template helpers. They exist so templates contain no logic
// beyond choosing what to show, and so money is formatted in exactly one place.
func funcs() template.FuncMap {
	return template.FuncMap{
		"date":   formatDate,
		"stamp":  formatStamp,
		"minor":  formatMinorPtr,
		"minorv": formatMinor,
		"drift":  formatDrift,
		"pct":    formatPercent,
		"short":  shortID,
		"duration": func(seconds int64) string {
			switch {
			case seconds <= 0:
				return "—"
			case seconds < 60:
				return fmt.Sprintf("%ds", seconds)
			case seconds < 3600:
				return fmt.Sprintf("%dm", seconds/60)
			default:
				return fmt.Sprintf("%dh", seconds/3600)
			}
		},
		"reliability": func(s *string) string {
			if s == nil {
				return "t-mute"
			}
			switch *s {
			case "confirmed":
				return "t-ok"
			case "estimated", "stale":
				return "t-warn"
			default:
				return "t-stop"
			}
		},
		"trust": func(s string) string {
			if s == "bank_confirmed" || s == "imported_verified" {
				return "t-ok"
			}
			return "t-warn"
		},
		"status": func(s string) string {
			switch s {
			case "completed", "sent":
				return "t-ok"
			case "pending", "leased":
				return "t-warn"
			case "dead":
				return "t-stop"
			}
			return "t-mute"
		},
		"access": func(s string) string {
			switch s {
			case "active", "trial":
				return "t-ok"
			case "grace":
				return "t-warn"
			}
			return "t-stop"
		},
		"excess": func(s string) string {
			switch s {
			case "reduce_principal":
				return "t-ok"
			case "unknown":
				return "t-stop"
			}
			return "t-warn"
		},
		"order": func(raw []byte) string {
			var def struct {
				Order []string `json:"order"`
			}
			if err := json.Unmarshal(raw, &def); err != nil || len(def.Order) == 0 {
				return "—"
			}
			return strings.Join(def.Order, " → ")
		},
	}
}

func formatDate(t any) string {
	v, ok := asTime(t)
	if !ok {
		return "—"
	}
	return v.Format("2006-01-02")
}

func formatStamp(t any) string {
	v, ok := asTime(t)
	if !ok {
		return "—"
	}
	return v.Local().Format("2006-01-02 15:04")
}

func asTime(t any) (time.Time, bool) {
	switch v := t.(type) {
	case time.Time:
		return v, !v.IsZero()
	case *time.Time:
		if v == nil {
			return time.Time{}, false
		}
		return *v, !v.IsZero()
	}
	return time.Time{}, false
}

// formatMinor renders minor units in the loan's own currency. Amounts are
// never rendered anywhere else, so an exponent is applied exactly once.
func formatMinor(minor int64, code string) string {
	cur, err := money.Lookup(code)
	if err != nil {
		return fmt.Sprintf("%d", minor)
	}
	return money.FromMinor(minor, cur).String()
}

func formatMinorPtr(minor *int64, code string) string {
	if minor == nil {
		return "—"
	}
	return formatMinor(*minor, code)
}

// formatDrift shows a signed difference, because the direction is the whole
// point: are we over- or under-stating what the borrower owes.
func formatDrift(minor int64) string {
	switch {
	case minor == 0:
		return "0"
	case minor > 0:
		return fmt.Sprintf("+%d", minor)
	default:
		return fmt.Sprintf("%d", minor)
	}
}

func formatPercent(rate float64) string { return fmt.Sprintf("%.3f%%", rate*100) }

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func urlEscape(s string) string { return url.QueryEscape(s) }

// newUUID returns a random RFC 4122 version 4 identifier. The engine never
// interprets an ID, so a local generator avoids a dependency for it.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // a failing CSPRNG is not a condition to paper over
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
