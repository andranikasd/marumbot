package corpus_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The manifest is the ratchet's memory. Every fixture is hashed into it, so
// a fixture cannot be rewritten to match a regression without the manifest
// changing in the same reviewed commit; the fixture count and each exact
// row count may only go up; and every fixture carries a support state that
// the product may quote — provisional until the boundary matrix is covered,
// verified after, experimental while rows still disagree.

type manifest struct {
	Fixtures map[string]struct {
		SHA256    string `json:"sha256"`
		TotalRows int    `json:"total_rows"`
		ExactRows int    `json:"exact_rows"`
		Support   string `json:"support"`
		Profile   string `json:"profile"`
	} `json:"fixtures"`
}

func TestCorpusProvenance(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "golden")
	raw, err := os.ReadFile(filepath.Join(dir, "MANIFEST.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, p := range paths {
		name := filepath.Base(p)
		if name == "MANIFEST.json" {
			continue
		}
		seen++
		entry, ok := m.Fixtures[name]
		if !ok {
			t.Errorf("%s is not in the manifest; add it with its hash and support state", name)
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
			t.Errorf("%s changed (sha256 %s, manifest %s): a fixture is evidence; update the manifest in the same reviewed change", name, got[:12], entry.SHA256[:12])
		}
		f := load(t, p)
		if len(f.Rows) < entry.TotalRows {
			t.Errorf("%s lost rows: %d, manifest %d", name, len(f.Rows), entry.TotalRows)
		}
		if f.Fidelity.ExactRows < entry.ExactRows {
			t.Errorf("%s exact_rows fell: %d, manifest %d", name, f.Fidelity.ExactRows, entry.ExactRows)
		}
		switch entry.Support {
		case "verified", "provisional", "experimental", "regulatory_example":
		default:
			t.Errorf("%s has unknown support state %q", name, entry.Support)
		}
		if entry.Support == "verified" && f.Fidelity.ExactRows != len(f.Rows) {
			t.Errorf("%s is marked verified with %d/%d exact rows", name, f.Fidelity.ExactRows, len(f.Rows))
		}
	}
	if seen < len(m.Fixtures) {
		t.Errorf("the corpus has %d fixtures, the manifest records %d: fixtures may not be removed", seen, len(m.Fixtures))
	}
	if seen == 0 {
		t.Fatal("empty corpus")
	}
}
