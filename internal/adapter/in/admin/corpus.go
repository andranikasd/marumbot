package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/andranikasd/marumbot/internal/app"
)

//go:embed evidence/*.json
var evidenceFS embed.FS

type embeddedCorpus struct{}

func (embeddedCorpus) Fixtures(context.Context) ([]app.AdminFixture, error) {
	body, err := evidenceFS.ReadFile("evidence/MANIFEST.json")
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Fixtures map[string]struct {
			SHA256    string `json:"sha256"`
			Source    string `json:"source"`
			Profile   string `json:"profile"`
			Support   string `json:"support"`
			Note      string `json:"note"`
			ExactRows int    `json:"exact_rows"`
			TotalRows int    `json:"total_rows"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, err
	}
	var result []app.AdminFixture
	for name, f := range manifest.Fixtures {
		raw, err := evidenceFS.ReadFile("evidence/" + name)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(raw)
		if hex.EncodeToString(hash[:]) != f.SHA256 {
			return nil, app.ErrConflict
		}
		result = append(result, app.AdminFixture{Name: name, SHA256: f.SHA256, Source: f.Source, Profile: f.Profile, Support: f.Support, Note: f.Note, ExactRows: f.ExactRows, TotalRows: f.TotalRows})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (c embeddedCorpus) Fixture(ctx context.Context, name string) (app.AdminFixtureDocument, error) {
	fixtures, err := c.Fixtures(ctx)
	if err != nil {
		return app.AdminFixtureDocument{}, err
	}
	var doc app.AdminFixtureDocument
	found := false
	for _, fixture := range fixtures {
		if fixture.Name == name {
			doc.Fixture = fixture
			found = true
			break
		}
	}
	if !found {
		return doc, app.ErrNotFound
	}
	body, err := evidenceFS.ReadFile("evidence/" + name)
	if err != nil {
		return doc, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var parsed map[string]any
	if err := decoder.Decode(&parsed); err != nil {
		return doc, err
	}
	rows, _ := parsed["rows"].([]any)
	delete(parsed, "rows")
	flattenEvidence("", parsed, &doc.Fields)
	columns := map[string]bool{}
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for key := range row {
			columns[key] = true
		}
	}
	for key := range columns {
		doc.Columns = append(doc.Columns, key)
	}
	sort.Strings(doc.Columns)
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		values := make([]string, 0, len(doc.Columns))
		for _, key := range doc.Columns {
			values = append(values, evidenceValue(row[key]))
		}
		doc.Rows = append(doc.Rows, values)
	}
	return doc, nil
}

func flattenEvidence(prefix string, value any, fields *[]app.AdminFixtureField) {
	if object, ok := value.(map[string]any); ok {
		names := make([]string, 0, len(object))
		for key := range object {
			names = append(names, key)
		}
		sort.Strings(names)
		for _, key := range names {
			label := key
			if prefix != "" {
				label = prefix + " / " + key
			}
			flattenEvidence(label, object[key], fields)
		}
		return
	}
	*fields = append(*fields, app.AdminFixtureField{Name: strings.ReplaceAll(prefix, "_", " "), Value: evidenceValue(value)})
}

func evidenceValue(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprint(value)
}

func (s *Server) corpusPage(w http.ResponseWriter, r *http.Request) {
	if name := r.PathValue("name"); name != "" {
		doc, err := s.admin.GoldenFixture(r.Context(), name)
		if err != nil {
			s.managementError(w, r, err)
			return
		}
		s.managementPage(w, r, "corpus.html", "Golden fixture", "corpus", map[string]any{"Document": doc})
		return
	}
	rows, err := s.admin.GoldenFixtures(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	s.managementPage(w, r, "corpus.html", "Golden corpus", "corpus", map[string]any{"Fixtures": rows})
}
