package increment

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// snapshotWorktree includes tracked, untracked and ignored local files. Links
// are retained as links, never followed into other checkouts. The Git history is
// retained separately by the deletion receipt's backup ref. A changing checkout
// or a write failure prevents removal; an incomplete archive is never published.
func snapshotWorktree(ctx context.Context, root, destination string) error {
	file, err := os.CreateTemp(filepath.Dir(destination), ".snapshot-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(temporary) }()
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	before := sha256.New()
	err = walkSnapshot(ctx, root, func(path, rel string, info fs.FileInfo) error {
		fingerprintEntry(before, rel, info)
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			var linkErr error
			link, linkErr = os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
		}
		header, headerErr := tar.FileInfoHeader(info, link)
		if headerErr != nil {
			return headerErr
		}
		header.Name = filepath.ToSlash(rel)
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()
		opened, err := source.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			return fmt.Errorf("file changed during snapshot: %s", rel)
		}
		if _, err = io.Copy(archive, source); err != nil {
			return err
		}
		after, err := source.Stat()
		if err != nil {
			return err
		}
		if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return fmt.Errorf("file changed during snapshot: %s", rel)
		}
		return source.Close()
	})
	tarErr := archive.Close()
	zipErr := compressed.Close()
	if err != nil {
		return err
	}
	if tarErr != nil {
		return tarErr
	}
	if zipErr != nil {
		return zipErr
	}
	after := sha256.New()
	if err = walkSnapshot(ctx, root, func(_ string, rel string, info fs.FileInfo) error { fingerprintEntry(after, rel, info); return nil }); err != nil {
		return err
	}
	if string(before.Sum(nil)) != string(after.Sum(nil)) {
		return iox.NewConflict("worktree changed during recovery snapshot", "stop its writers and retry; no working files were removed", nil)
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, destination)
}

func fingerprintEntry(out io.Writer, rel string, info fs.FileInfo) {
	_, _ = fmt.Fprintf(out, "%s\x00%d\x00%d\x00%d\n", rel, info.Mode(), info.Size(), info.ModTime().UnixNano())
}

func walkSnapshot(ctx context.Context, root string, visit func(string, string, fs.FileInfo) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		// Socket endpoints and devices are runtime resources, not recoverable files.
		if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		return visit(path, rel, info)
	})
}
