//go:build darwin

package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinMountIdentityClassificationRejectsCrossing(t *testing.T) {
	project := t.TempDir()
	root, err := darwinMountIdentity(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := classifyDarwinMountIdentity(root, root); err != nil {
		t.Fatalf("same mount identity rejected: %v", err)
	}

	differentMount := root
	differentMount.mountOn += "/nested-mount"
	if err := classifyDarwinMountIdentity(root, differentMount); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("different mounted-on identity error=%v, want ErrUnsafeSourcePath", err)
	}

	differentFilesystem := root
	differentFilesystem.fsid.Val[0]++
	if err := classifyDarwinMountIdentity(root, differentFilesystem); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("different filesystem identity error=%v, want ErrUnsafeSourcePath", err)
	}
}

func TestDarwinMountIdentityIsStableWithinProject(t *testing.T) {
	project := t.TempDir()
	child := filepath.Join(project, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := darwinMountIdentity(project)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := darwinMountIdentity(child)
	if err != nil {
		t.Fatal(err)
	}
	if identity != root {
		t.Fatalf("ordinary child mount identity=%+v, root=%+v", identity, root)
	}
}

// TestDarwinNativeMountedImageIsRejected is driven by the named macOS CI
// setup, which attaches a real filesystem image at mounted/ below the trusted
// root. The named native gate always supplies the environment variable.
func TestDarwinNativeMountedImageIsRejected(t *testing.T) {
	project := os.Getenv("ARCHETIPO_DARWIN_MOUNT_PROJECT")
	if project == "" {
		t.Skip("native mounted-image fixture is configured by the named macOS CI gate")
	}
	mounted := filepath.Join(project, "mounted")
	rootIdentity, err := darwinMountIdentity(project)
	if err != nil {
		t.Fatalf("project mount identity: %v", err)
	}
	mountedIdentity, err := darwinMountIdentity(mounted)
	if err != nil {
		t.Fatalf("mounted-image identity: %v", err)
	}
	if mountedIdentity == rootIdentity {
		t.Fatalf("mounted image did not cross filesystem identity: %+v", mountedIdentity)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("mounted/evidence.txt"); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("mounted-image evidence error=%v, want ErrUnsafeSourcePath", err)
	}
}
