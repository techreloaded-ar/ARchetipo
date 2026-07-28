package wiki

import (
	"errors"
	"io"
)

func readRegularEvidence(path string) ([]byte, error) {
	file, verify, err := openEvidenceRegular(path)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(file)
	verifyErr := verify()
	closeErr := file.Close()
	if readErr != nil || verifyErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, verifyErr, closeErr)
	}
	return raw, nil
}
