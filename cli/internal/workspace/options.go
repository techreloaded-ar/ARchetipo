package workspace

import (
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// Options is the request to initialize a workspace. Dir is the destination;
// everything else is a choice the caller made among the ones Available()
// declares.
type Options struct {
	Dir       string                `json:"dir"`
	Connector string                `json:"connector"`
	Tools     []string              `json:"tools"`
	Paths     domain.ConfigPaths    `json:"paths"`
	Worktree  domain.WorktreeConfig `json:"worktree"`
}

// FieldError binds a refusal to the input that caused it. Code is stable and
// is what a program keys on; Message is what a person reads.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError carries every refusal of a single request, so one answer can
// highlight several inputs instead of making the caller fix them one at a time.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return "invalid workspace options — " + strings.Join(parts, "; ")
}

func (e *ValidationError) add(field, code, message string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Code: code, Message: message})
}

// Refusal codes. They are part of the contract: the viewer renders them and
// the smoke tests assert on them, so they never change silently.
const (
	CodeDirRequired         = "WORKSPACE_DIR_REQUIRED"
	CodeDirNotAbsolute      = "WORKSPACE_DIR_NOT_ABSOLUTE"
	CodeDirNotADirectory    = "WORKSPACE_DIR_NOT_A_DIRECTORY"
	CodeDirNotWritable      = "WORKSPACE_DIR_NOT_WRITABLE"
	CodeParentNotWritable   = "WORKSPACE_PARENT_NOT_WRITABLE"
	CodeAlreadyInitialized  = "WORKSPACE_ALREADY_INITIALIZED"
	CodeConnectorUnknown    = "WORKSPACE_CONNECTOR_UNKNOWN"
	CodeToolsRequired       = "WORKSPACE_TOOLS_REQUIRED"
	CodeToolUnknown         = "WORKSPACE_TOOL_UNKNOWN"
	CodePathRequired        = "WORKSPACE_PATH_REQUIRED"
	CodePathNotRelative     = "WORKSPACE_PATH_NOT_RELATIVE"
	CodeWorktreeFieldNeeded = "WORKSPACE_WORKTREE_FIELD_REQUIRED"
	CodeWorktreeDirInvalid  = "WORKSPACE_WORKTREE_DIR_INVALID"
	CodePathOccupied        = "WORKSPACE_PATH_OCCUPIED"
	CodeCancelled           = "WORKSPACE_INITIALIZATION_CANCELLED"
)

// Choice is one selectable value with the text that describes it.
type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// TemplateIdentity is the process Template a new workspace is initialized on.
// It is reported, never chosen: there is exactly one installed Archetype, and
// a choice with a single element is not a feature.
type TemplateIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Label   string `json:"label"`
}

// Choices is what an initialization actually accepts, plus the values it would
// use if the caller expressed no preference.
type Choices struct {
	Connectors []Choice              `json:"connectors"`
	Tools      []Choice              `json:"tools"`
	Paths      domain.ConfigPaths    `json:"paths"`
	Worktree   domain.WorktreeConfig `json:"worktree"`
	Template   TemplateIdentity      `json:"template"`
}

// Available reports the accepted choices and the defaults. It reads the same
// registries the initialization itself reads, which is what keeps "offered"
// and "accepted" the same list rather than two lists that agree today.
func Available() Choices {
	defaults := config.Default()
	tpl := template.Default()

	connectors := make([]Choice, 0, len(connectorLabels))
	for _, c := range connectorLabels {
		connectors = append(connectors, Choice{ID: c.ID, Label: c.Label})
	}
	toolChoices := make([]Choice, 0, len(tools))
	for _, t := range Tools() {
		toolChoices = append(toolChoices, Choice{ID: t.Key, Label: t.Name})
	}
	return Choices{
		Connectors: connectors,
		Tools:      toolChoices,
		Paths:      defaults.Paths,
		Worktree:   defaults.Worktree,
		Template: TemplateIdentity{
			ID:      tpl.ID,
			Version: tpl.Version,
			Label:   tpl.Label,
		},
	}
}
