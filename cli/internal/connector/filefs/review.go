package filefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

const reviewSchema = "archetipo/review/v1"

// reviewDoc is the on-disk representation of a spec's review artifact: the
// inline comments, the dossier a provider prepared and the verdict a person
// reached. Dossier and verdict are additive and optional, so a file written
// before they existed stays readable and simply yields nil for both — the
// schema version is unchanged for exactly that reason.
type reviewDoc struct {
	Schema   string                 `yaml:"schema"`
	SpecCode string                 `yaml:"spec_code"`
	Comments []domain.ReviewComment `yaml:"comments"`
	Dossier  *domain.ReviewDossier  `yaml:"dossier,omitempty"`
	Verdict  *domain.ReviewVerdict  `yaml:"verdict,omitempty"`
}

// reviewsDir is the directory holding per-spec review files, a sibling of the
// specs/ directory under .archetipo/.
func (c *Connector) reviewsDir() string {
	return filepath.Join(filepath.Dir(c.backlogPath()), "reviews")
}

func (c *Connector) reviewPath(specCode string) string {
	return filepath.Join(c.reviewsDir(), specCode+".yaml")
}

// ReadReview returns the inline comments saved for a spec. A missing file is
// not an error: it returns an empty Review. The web viewer discovers this
// method at runtime via a type assertion (reviewStore capability).
func (c *Connector) ReadReview(ctx context.Context, code string) (domain.Review, error) {
	raw, err := os.ReadFile(c.reviewPath(code))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return domain.Review{Comments: []domain.ReviewComment{}}, nil
		}
		return domain.Review{}, fmt.Errorf("reading review file: %w", err)
	}
	var doc reviewDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return domain.Review{}, iox.NewInvalidInput(fmt.Sprintf("invalid review YAML at %s", c.reviewPath(code)), "", err)
	}
	if doc.Comments == nil {
		doc.Comments = []domain.ReviewComment{}
	}
	return domain.Review{Comments: doc.Comments, Dossier: doc.Dossier, Verdict: doc.Verdict}, nil
}

// SaveReview persists the review artifact for a spec. Saving a Review with no
// comments clears them (used after "request changes" converts comments to
// tasks); a nil dossier or verdict is written as absent, not as empty.
func (c *Connector) SaveReview(ctx context.Context, code string, r domain.Review) error {
	doc := reviewDoc{
		Schema:   reviewSchema,
		SpecCode: code,
		Comments: r.Comments,
		Dossier:  r.Dossier,
		Verdict:  r.Verdict,
	}
	if doc.Comments == nil {
		doc.Comments = []domain.ReviewComment{}
	}
	return writeYAML(c.reviewPath(code), doc)
}
