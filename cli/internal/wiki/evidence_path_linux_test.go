//go:build linux

package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestLinuxMountInfoPathDecoding(t *testing.T) {
	decoded, err := decodeLinuxMountInfoPath(`/tmp/space\040tab\011slash\134name`)
	if err != nil || decoded != "/tmp/space tab\tslash\\name" {
		t.Fatalf("decodeLinuxMountInfoPath()=(%q, %v)", decoded, err)
	}
}

func TestLinuxMountIdentityClassificationRejectsCrossing(t *testing.T) {
	if err := classifyLinuxMountIdentity(41, 41); err != nil {
		t.Fatalf("same mount identity rejected: %v", err)
	}
	if err := classifyLinuxMountIdentity(41, 42); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("cross-mount identity error=%v, want ErrUnsafeSourcePath", err)
	}
}

func TestLinuxMountIdentityIsStableWithinProject(t *testing.T) {
	project := t.TempDir()
	child := filepath.Join(project, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	rootID, err := linuxMountID(project)
	if err != nil {
		t.Fatal(err)
	}
	childID, err := linuxMountID(child)
	if err != nil {
		t.Fatal(err)
	}
	if childID != rootID {
		t.Fatalf("ordinary child mount ID=%d, root=%d", childID, rootID)
	}
}

// TestLinuxNativeBindMountIsRejected is driven by the named Linux CI setup,
// which creates mounted/evidence.txt as a same-filesystem bind mount below the
// trusted project root. The ordinary package suite may skip only because it
// does not create privileged host fixtures; the named native gate always sets
// ARCHETIPO_LINUX_BIND_PROJECT and therefore may not skip.
func TestLinuxNativeBindMountIsRejected(t *testing.T) {
	project := os.Getenv("ARCHETIPO_LINUX_BIND_PROJECT")
	if project == "" {
		t.Skip("native bind-mount fixture is configured by the named Linux CI gate")
	}
	mounted := filepath.Join(project, "mounted")
	rootInfo, err := os.Stat(project)
	if err != nil {
		t.Fatalf("stat project fixture: %v", err)
	}
	mountedInfo, err := os.Stat(mounted)
	if err != nil {
		t.Fatalf("stat bind fixture: %v", err)
	}
	rootStat, rootOK := rootInfo.Sys().(*syscall.Stat_t)
	mountedStat, mountedOK := mountedInfo.Sys().(*syscall.Stat_t)
	if !rootOK || !mountedOK {
		t.Fatal("native device identity is unavailable")
	}
	if rootStat.Dev != mountedStat.Dev {
		t.Fatalf("fixture is not a same-filesystem bind mount: root dev=%d mounted dev=%d", rootStat.Dev, mountedStat.Dev)
	}
	rootMountID, err := linuxMountID(project)
	if err != nil {
		t.Fatalf("project mount identity: %v", err)
	}
	mountedMountID, err := linuxMountID(mounted)
	if err != nil {
		t.Fatalf("bind mount identity: %v", err)
	}
	if rootMountID == mountedMountID {
		t.Fatalf("bind mount did not receive a distinct mount ID: %d", rootMountID)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("mounted/evidence.txt"); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("bind-mounted evidence error=%v, want ErrUnsafeSourcePath", err)
	}
}
