// Package services runs the services a project recipe declares (a database, a
// cache) for one checkout. The project states what it needs; ARchetipo starts
// it, so the same recipe works with the person's container engine and inside
// the isolated environment, where no engine is available (ADR 0005).
package services

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Spec is one declared service, as written in the recipe.
type Spec struct {
	// Image is the container image the service comes from, for example
	// postgres:17-alpine. Inside the isolated environment it selects the
	// runtime installed there instead.
	Image string `json:"image"`
	// Port is the port the service listens on inside its image.
	Port int `json:"port"`
	// Env configures the service with the variables its image documents.
	Env map[string]string `json:"env,omitempty"`
	// Data is the directory of the image that holds the service's data. It is
	// kept across stops and removed with the checkout. Without it the data
	// lasts as long as the running service.
	Data string `json:"data,omitempty"`
	// Expose names the variables handed to preparation, checks and preview.
	// {host} and {port} are replaced with where this checkout's service answers.
	Expose map[string]string `json:"expose,omitempty"`
}

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	envPattern   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	imagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*(:[A-Za-z0-9._-]+)?(@sha256:[a-f0-9]{64})?$`)
	placeholder  = regexp.MustCompile(`\{[^}]*\}`)
)

// reservedPrefixes are variables the deterministic layer owns.
var reservedPrefixes = []string{"ARCHETIPO_"}

// Validate reports the first problem of a declaration, in terms of the recipe.
func Validate(name string, spec Spec) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("service name %q must be lowercase letters, digits and dashes", name)
	}
	if len(spec.Image) > 255 || !imagePattern.MatchString(spec.Image) {
		return fmt.Errorf("service %s: image %q is not a valid image reference", name, spec.Image)
	}
	if spec.Port < 1 || spec.Port > 65535 {
		return fmt.Errorf("service %s: port must be between 1 and 65535", name)
	}
	if spec.Data != "" && (!strings.HasPrefix(spec.Data, "/") || strings.Contains(spec.Data, "..") || strings.ContainsAny(spec.Data, ":,\x00")) {
		return fmt.Errorf("service %s: data must be an absolute directory of the image", name)
	}
	for key, value := range spec.Env {
		if !envPattern.MatchString(key) || reserved(key) || strings.ContainsRune(value, 0) {
			return fmt.Errorf("service %s: invalid env variable %q", name, key)
		}
	}
	if len(spec.Expose) == 0 {
		return fmt.Errorf("service %s: expose at least one variable, so the app knows where the service is", name)
	}
	for key, value := range spec.Expose {
		if !envPattern.MatchString(key) || reserved(key) || strings.ContainsRune(value, 0) {
			return fmt.Errorf("service %s: invalid exposed variable %q", name, key)
		}
		for _, found := range placeholder.FindAllString(value, -1) {
			if found != "{host}" && found != "{port}" {
				return fmt.Errorf("service %s: %s may use only {host} and {port}, not %s", name, key, found)
			}
		}
	}
	return nil
}

func reserved(key string) bool {
	for _, prefix := range reservedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// PortKey is the name of the recipe port assigned to a service.
func PortKey(name string) string {
	return "SERVICE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// Exposed returns the variables of a service reachable at host:port, sorted.
func Exposed(spec Spec, host string, port string) []string {
	keys := make([]string, 0, len(spec.Expose))
	for key := range spec.Expose {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	replacer := strings.NewReplacer("{host}", host, "{port}", port)
	for _, key := range keys {
		values = append(values, key+"="+replacer.Replace(spec.Expose[key]))
	}
	return values
}

// imageName splits a reference into its repository (without the default
// registry) and its tag.
func imageName(image string) (repository, tag string) {
	ref := image
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	repository = ref
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		repository, tag = ref[:colon], ref[colon+1:]
	}
	repository = strings.TrimPrefix(repository, "docker.io/")
	repository = strings.TrimPrefix(repository, "library/")
	return repository, tag
}

// majorVersion reads the leading number of a tag: "17-alpine" and "17.2" are 17.
func majorVersion(tag string) (int, bool) {
	end := 0
	for end < len(tag) && tag[end] >= '0' && tag[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(tag[:end])
	return n, err == nil
}
