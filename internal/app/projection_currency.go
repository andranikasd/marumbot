package app

import "github.com/andranikasd/marumbot/pkg/core/money"

func projectionCurrencyValid(code string) bool {
	c, err := money.Lookup(code)
	return err == nil && c.Code == code
}
