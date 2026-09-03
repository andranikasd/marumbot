package miniapp

import (
	"encoding/json"
	"io"
	"net/http"
)

// decodeRequest refuses unknown financial terms instead of silently ignoring
// clauses that this engine does not implement, and accepts exactly one object.
func decodeRequest(w http.ResponseWriter, r *http.Request, out any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequest))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		if err == nil {
			return ErrInvalid
		}
		return err
	}
	return nil
}
