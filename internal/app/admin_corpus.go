package app

import "context"

type AdminFixture struct {
	Name      string
	SHA256    string
	Source    string
	Profile   string
	Support   string
	Note      string
	ExactRows int
	TotalRows int
}
type AdminFixtureField struct {
	Name  string
	Value string
}
type AdminFixtureDocument struct {
	Fixture AdminFixture
	Fields  []AdminFixtureField
	Columns []string
	Rows    [][]string
}
type AdminCorpusStore interface {
	Fixtures(context.Context) ([]AdminFixture, error)
	Fixture(context.Context, string) (AdminFixtureDocument, error)
}

func (a *Admin) WithCorpus(c AdminCorpusStore) *Admin { a.corpus = c; return a }
func (a *Admin) GoldenFixtures(ctx context.Context) ([]AdminFixture, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityFixtureReview, "golden_corpus"); err != nil {
		return nil, err
	}
	if a.corpus == nil {
		return nil, ErrAdminSecurityUnavailable
	}
	return a.corpus.Fixtures(ctx)
}

func (a *Admin) GoldenFixture(ctx context.Context, name string) (AdminFixtureDocument, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityFixtureReview, name); err != nil {
		return AdminFixtureDocument{}, err
	}
	if a.corpus == nil {
		return AdminFixtureDocument{}, ErrAdminSecurityUnavailable
	}
	return a.corpus.Fixture(ctx, name)
}
