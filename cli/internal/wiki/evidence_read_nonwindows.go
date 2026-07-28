//go:build !windows

package wiki

import "os"

func openEvidenceRegular(path string) (*os.File, func() error, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, func() error { return nil }, nil
}
