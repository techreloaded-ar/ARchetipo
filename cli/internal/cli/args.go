package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// contentSensitiveFlags are flag names whose values must never appear in
// analytics events. Only their presence (true) is recorded in the args map.
var contentSensitiveFlags = map[string]bool{
	"file":           true,
	"commit-summary": true,
}

// extractArgs builds an analytics-safe args map from the executed Cobra command.
// It returns nil when no flags were set and there are no positional args, so
// the JSON field is omitted entirely via omitempty.
//
// Rules:
//   - Only flags explicitly set by the user are included (cmd.Flags().Visit).
//   - Content-sensitive flags (file, commit-summary) record only presence (true).
//   - All other flags record their actual values.
//   - Positional (non-flag) arguments get keys _0, _1, _2, ...
func extractArgs(cmd *cobra.Command) map[string]any {
	if cmd == nil {
		return nil
	}
	args := make(map[string]any)

	// Named flags — only those explicitly set by the user.
	cmd.Flags().Visit(func(f *pflag.Flag) {
		name := f.Name
		if contentSensitiveFlags[name] {
			args[name] = true
		} else {
			args[name] = flagValue(f)
		}
	})

	// Positional args (non-flag arguments after parsing).
	posArgs := cmd.Flags().Args()
	for i, a := range posArgs {
		args[fmt.Sprintf("_%d", i)] = a
	}

	if len(args) == 0 {
		return nil
	}
	return args
}

// flagValue returns the best JSON-compatible representation of a pflag value.
// StringSlice values are returned as []string for clean JSON arrays; everything
// else uses the flag's String() representation.
func flagValue(f *pflag.Flag) any {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return sv.GetSlice()
	}
	return f.Value.String()
}
