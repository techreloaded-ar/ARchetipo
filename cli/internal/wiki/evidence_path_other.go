//go:build !windows && !linux && !darwin

package wiki

import "errors"

type platformEvidenceRoot struct{}

func newPlatformEvidenceRoot(_ string) (platformEvidenceRoot, error) {
	return platformEvidenceRoot{}, errors.Join(ErrEvidenceUnreadable, errors.New("platform mount identity is unsupported"))
}

func validatePlatformEvidenceComponent(_ platformEvidenceRoot, _, _ string, _ bool) error {
	return errors.Join(ErrEvidenceUnreadable, errors.New("platform mount identity is unsupported"))
}
