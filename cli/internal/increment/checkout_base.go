package increment

import (
	"regexp"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// CheckoutBase returns the increment that owns the checkout described by cfg
// and the commit it forked from, read from the sandbox marker and the
// increment's own record. found is false for an ordinary checkout and for a
// record without a usable base: no base is ever guessed from Git.
func CheckoutBase(cfg config.Config) (base verification.IncrementBase, found bool, err error) {
	reference := cfg.Increment
	if reference == nil {
		return base, false, nil
	}
	parent, err := config.LoadExactReadOnly(reference.ParentRoot)
	if err != nil {
		return base, false, err
	}
	record, err := New(parent, nil).Read(reference.ID)
	if err != nil {
		return base, false, err
	}
	if !fullCommitSHA.MatchString(record.BaseSHA) {
		return base, false, nil
	}
	return verification.IncrementBase{IncrementID: record.ID, BaseSHA: record.BaseSHA, BaseBranch: record.BaseBranch}, true, nil
}
