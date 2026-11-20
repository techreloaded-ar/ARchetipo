package increment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
)

func TestCheckoutBaseComesFromTheIncrementRecord(t *testing.T) {
	s := fixture(t)
	record, err := s.Create(context.Background(), "Customer export", "Prepare the sales review", "")
	if err != nil {
		t.Fatal(err)
	}
	worktreeConfig, err := config.LoadExact(record.Workdir)
	if err != nil {
		t.Fatal(err)
	}
	base, found, err := CheckoutBase(worktreeConfig)
	if err != nil || !found {
		t.Fatalf("CheckoutBase() found=%t err=%v", found, err)
	}
	if base.IncrementID != record.ID || base.BaseSHA != record.BaseSHA || base.BaseBranch != record.BaseBranch || base.BaseSHA != runGit(t, s.Config.ProjectRoot, "rev-parse", "main") {
		t.Fatalf("base = %+v, want the record's %s on %s", base, record.BaseSHA, record.BaseBranch)
	}
	// Commits added by the increment never move the base it started from.
	if err = os.WriteFile(filepath.Join(record.Workdir, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, record.Workdir, "commit", "-am", "product change")
	afterCommit, found, err := CheckoutBase(worktreeConfig)
	if err != nil || !found || afterCommit != base {
		t.Fatalf("base moved with the increment's own commit: %+v found=%t err=%v", afterCommit, found, err)
	}
}

func TestOrdinaryCheckoutHasNoBase(t *testing.T) {
	s := fixture(t)
	base, found, err := CheckoutBase(s.Config)
	if err != nil || found || base.BaseSHA != "" {
		t.Fatalf("an ordinary checkout got a base: %+v found=%t err=%v", base, found, err)
	}
}

func TestRecordWithoutAUsableBaseIsNotGuessed(t *testing.T) {
	s := fixture(t)
	record, err := s.Create(context.Background(), "Customer export", "Prepare the sales review", "")
	if err != nil {
		t.Fatal(err)
	}
	record.BaseSHA = "main"
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(record.Workdir, ".archetipo", "increment-local", "increment.json")
	if err = os.WriteFile(local, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	worktreeConfig, err := config.LoadExact(record.Workdir)
	if err != nil {
		t.Fatal(err)
	}
	if base, found, baseErr := CheckoutBase(worktreeConfig); baseErr != nil || found {
		t.Fatalf("a record without a commit base produced %+v found=%t err=%v", base, found, baseErr)
	}
}
