package recordfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidIDAndWriteAtomic(t *testing.T) {
	if !ValidID("record-1") || ValidID("../record-1") {
		t.Fatal("invalid ID validation")
	}
	dir := filepath.Join(t.TempDir(), "records")
	path := filepath.Join(dir, "record-1.json")
	if err := WriteAtomic(dir, path, ".record-*.tmp", map[string]string{"id": "record-1"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "{\n  \"id\": \"record-1\"\n}\n" {
		t.Fatalf("record = %q, %v", body, err)
	}
}
