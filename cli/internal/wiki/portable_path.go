package wiki

import (
	"errors"
	pathpkg "path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var (
	errPortablePathCollision = errors.New("portable evidence path collision")
	errNonCanonicalPath      = errors.New("evidence path spelling is not canonical")
)

// normalizePortableEvidencePath applies one lexical namespace on every host.
// Slash and backslash are accepted as input separators and slash is the only
// separator returned to callers. It deliberately does not consult the host
// filesystem.
func normalizePortableEvidencePath(sourcePath string) (string, error) {
	if sourcePath == "" || !utf8.ValidString(sourcePath) {
		return "", ErrInvalidSourcePath
	}
	portable := strings.ReplaceAll(sourcePath, `\`, "/")
	if strings.HasPrefix(portable, "/") || hasWindowsVolumePrefix(portable) {
		return "", ErrInvalidSourcePath
	}
	// Detect traversal before lexical cleaning so a/../b can never be turned
	// into an apparently safe path. Benign dot components, repeated separators,
	// and a trailing separator remain accepted for compatibility; no metadata is
	// rewritten and all internal identity uses the slash-normalized clean form.
	for _, component := range strings.Split(portable, "/") {
		if component == ".." {
			return "", ErrUnsafeSourcePath
		}
	}
	clean := pathpkg.Clean(portable)
	if clean == "." {
		return ".", nil
	}
	components := strings.Split(clean, "/")
	for _, component := range components {
		if err := validatePortableEvidenceComponent(component); err != nil {
			return "", err
		}
	}
	return strings.Join(components, "/"), nil
}

func validatePortableEvidenceComponent(component string) error {
	if component == "" || !utf8.ValidString(component) {
		return ErrInvalidSourcePath
	}
	if strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
		return ErrInvalidSourcePath
	}
	for _, r := range component {
		if unicode.IsControl(r) || strings.ContainsRune(`:<>"|?*`, r) {
			return ErrInvalidSourcePath
		}
	}
	if isDOSDeviceComponent(component) {
		return ErrInvalidSourcePath
	}
	return nil
}

func isDOSDeviceComponent(component string) bool {
	stem := component
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) {
		return stem[3] >= '1' && stem[3] <= '9'
	}
	for _, prefix := range []string{"COM", "LPT"} {
		if strings.HasPrefix(stem, prefix) {
			suffix := strings.TrimPrefix(stem, prefix)
			if suffix == "¹" || suffix == "²" || suffix == "³" {
				return true
			}
		}
	}
	return false
}

func portableComponentKey(component string) (string, error) {
	if err := validatePortableEvidenceComponent(component); err != nil {
		return "", err
	}
	return cases.Fold().String(norm.NFC.String(component)), nil
}
