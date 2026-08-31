package providerconfig

import (
	"fmt"
	"sort"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func ParseSeconds(raw map[string]any, field string, fallback, minimum, maximum int) (int, error) {
	value, present := raw[field]
	if !present || value == nil {
		return fallback, nil
	}
	var seconds int
	switch typed := value.(type) {
	case int:
		seconds = typed
	case int64:
		seconds = int(typed)
	case float64:
		if typed != float64(int(typed)) {
			return 0, configError(field, "must be a whole number of seconds")
		}
		seconds = int(typed)
	default:
		return 0, configError(field, "must be an integer number of seconds")
	}
	if seconds < minimum || seconds > maximum {
		return 0, configError(field, fmt.Sprintf("must be between %d and %d", minimum, maximum))
	}
	return seconds, nil
}

func String(value any, field, fallback string, allowEmpty bool) (string, error) {
	if value == nil {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configError(field, "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" && !allowEmpty {
		return "", configError(field, "must not be empty")
	}
	return text, nil
}

func RejectUnknownKeys(raw, known map[string]any, provider string) error {
	unknown := make([]string, 0, len(raw))
	for key := range raw {
		if _, ok := known[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return configError(unknown[0], "is not a recognized "+provider+" provider configuration key")
}

func configError(field, reason string) error {
	return &execution.ConfigurationError{Field: field, Reason: reason}
}
