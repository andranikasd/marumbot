package money

import "encoding/json"

// MarshalJSON preserves minor units and currency metadata for source manifests.
func (a Amount) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Minor    int64    `json:"minor"`
		Currency Currency `json:"currency"`
	}{a.minor, a.cur})
}

// UnmarshalJSON restores exact units and refuses changed currency metadata.
func (a *Amount) UnmarshalJSON(raw []byte) error {
	var v struct {
		Minor    int64    `json:"minor"`
		Currency Currency `json:"currency"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	if v.Currency.Code == "" && v.Minor == 0 && v.Currency == (Currency{}) {
		*a = Amount{}
		return nil
	}
	cur, err := Lookup(v.Currency.Code)
	if err != nil {
		return err
	}
	// A registry change requires the historical engine; never reinterpret units.
	if cur != v.Currency {
		return ErrUnknownCurrency
	}
	*a = FromMinor(v.Minor, cur)
	return nil
}
