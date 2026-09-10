package miniapp

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// JSON consumers must be able to represent minor units exactly.
const maxSafeMinor int64 = 9_007_199_254_740_991

// Keep the original JSON decimal digits all the way to integer minor units.
// Numbers and quoted decimals are accepted for old and new clients respectively.
var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func scaledDecimal(value json.Number, exponent uint8, max int64, allowZero bool) (int64, error) {
	raw := string(value)
	if raw == "" {
		raw = "0"
	}
	if len(raw) > 64 || !decimalPattern.MatchString(raw) {
		return 0, fmt.Errorf("%w: decimal", ErrInvalid)
	}
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		e, err := strconv.Atoi(raw[i+1:])
		if err != nil || e < -18 || e > 18 {
			return 0, fmt.Errorf("%w: decimal exponent", ErrInvalid)
		}
	}
	n, ok := new(big.Rat).SetString(raw)
	if !ok {
		return 0, fmt.Errorf("%w: decimal", ErrInvalid)
	}
	scale := int64(1)
	for range exponent {
		scale *= 10
	}
	n.Mul(n, new(big.Rat).SetInt64(scale))
	if !n.IsInt() || !n.Num().IsInt64() {
		return 0, fmt.Errorf("%w: amount precision or range", ErrInvalid)
	}
	minor := n.Num().Int64()
	if minor < 0 || minor > max || (!allowZero && minor == 0) {
		return 0, fmt.Errorf("%w: amount range", ErrInvalid)
	}
	return minor, nil
}

func decimalNumber(minor int64, exponent uint8) json.Number {
	raw := strconv.FormatInt(minor, 10)
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = raw[1:]
	}
	for len(raw) <= int(exponent) {
		raw = "0" + raw
	}
	if exponent > 0 {
		at := len(raw) - int(exponent)
		raw = raw[:at] + "." + raw[at:]
	}
	if exponent > 0 {
		raw = strings.TrimRight(strings.TrimRight(raw, "0"), ".")
	}
	return json.Number(sign + raw)
}
