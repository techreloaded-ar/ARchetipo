package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// NextSpecCode returns the progressive code that follows the highest US-NNN
// already present. There is no counter anywhere: the persisted backlog is the
// only memory, so a spec that was never written never consumes a code.
func NextSpecCode(specs []Spec) string {
	highest := 0
	for _, spec := range specs {
		suffix, ok := strings.CutPrefix(strings.TrimSpace(spec.Code), "US-")
		if !ok {
			continue
		}
		number, err := strconv.Atoi(suffix)
		if err != nil || number <= 0 {
			continue
		}
		if number > highest {
			highest = number
		}
	}
	return fmt.Sprintf("US-%03d", highest+1)
}
