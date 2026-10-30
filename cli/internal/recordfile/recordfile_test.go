package recordfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

func TestAtomicReplacementWithConcurrentReaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	if err := WriteAtomic(dir, path, ".record-", map[string]int{"revision": 0}); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	failures := make(chan error, 4)
	workers.Add(1)
	go func() {
		defer workers.Done()
		for revision := 1; revision <= 50; revision++ {
			if err := WriteAtomic(dir, path, ".record-", map[string]int{"revision": revision}); err != nil {
				failures <- err
				return
			}
		}
	}()
	for reader := 0; reader < 3; reader++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for sample := 0; sample < 100; sample++ {
				raw, err := ReadFile(path)
				var record map[string]int
				if err == nil {
					err = json.Unmarshal(raw, &record)
				}
				if err != nil || len(record) != 1 || record["revision"] < 0 || record["revision"] > 50 {
					failures <- fmt.Errorf("incomplete atomic record %q: %v", raw, err)
					return
				}
			}
		}()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}
