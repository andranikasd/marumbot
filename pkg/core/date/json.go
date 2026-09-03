package date

import "encoding/json"

// MarshalJSON encodes an ISO date, or null for an absent date.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON restores an ISO date without timezone conversion.
func (d *Date) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		*d = Date{}
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	parsed, err := Parse(text)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
