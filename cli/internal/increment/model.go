// Package increment records product work in a dedicated workspace.
// It owns durable facts; agent judgement remains in the skills.
package increment

import (
	"context"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

const (
	Concluded  = "CONCLUDED"
	Discovery  = "DISCOVERY"
	Developing = "DEVELOPING"
	Review     = "REVIEW"
	Publishing = "PUBLISHING"
	Proposed   = "PROPOSED"
	Delivered  = "DELIVERED"
)

// Continuous development authorizes local iterations from the request, never publication.
const Continuous = "continuous"

// Stories is decoded only to reject retired delivery payloads explicitly.
type Proposal struct {
	StoryCodes []string      `json:"story_codes,omitempty"`
	Intent     string        `json:"intent"`
	Stories    []domain.Spec `json:"stories,omitempty"`
}

// FeedbackRequest classifies agent judgement explicitly. The CLI validates the
// declared story identities; it never classifies the person's natural language.
type FeedbackRequest struct {
	Feedback   string   `json:"feedback" yaml:"feedback"`
	Kind       string   `json:"kind,omitempty" yaml:"kind,omitempty"`
	StoryCodes []string `json:"story_codes,omitempty" yaml:"story_codes,omitempty"`
}
type StoryLink struct {
	Body  string `json:"body,omitempty"`
	Code  string `json:"code"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
}
type Revision struct {
	LinkedStories []StoryLink `json:"linked_stories,omitempty"`
	Number        int         `json:"number"`
	Proposal
	ApprovalSource string           `json:"approval_source,omitempty"`
	Approved       bool             `json:"approved,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	Feedback       string           `json:"feedback,omitempty"`
	Result         *Result          `json:"result,omitempty"`
	HeadSHA        string           `json:"head_sha,omitempty"`
	Change         *FeedbackRequest `json:"change,omitempty"`
}
type Result struct {
	Evidence   []verification.Result `json:"evidence,omitempty"`
	Summary    string                `json:"summary"`
	PreviewURL string                `json:"preview_url,omitempty"`
	Checks     []string              `json:"checks"`
}
type Learning struct {
	Observation string    `json:"observation"`
	Decision    string    `json:"decision,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Record struct {
	ApprovedAt           *time.Time       `json:"approved_at,omitempty"`
	LinkedStories        []StoryLink      `json:"linked_stories,omitempty"`
	Learning             []Learning       `json:"learning,omitempty"`
	Conclusion           string           `json:"conclusion,omitempty"`
	IntegrationSHA       string           `json:"integration_sha,omitempty"`
	ID                   string           `json:"id"`
	DevelopmentMode      string           `json:"development_mode,omitempty"`
	Title                string           `json:"title"`
	Request              string           `json:"request"`
	Status               string           `json:"status"`
	CreatedAt            time.Time        `json:"created_at"`
	BaseBranch           string           `json:"base_branch"`
	BaseSHA              string           `json:"base_sha"`
	Branch               string           `json:"branch"`
	Workdir              string           `json:"workdir"`
	ConversationID       string           `json:"conversation_id,omitempty"`
	OriginConversationID string           `json:"origin_conversation_id,omitempty"`
	Revisions            []Revision       `json:"revisions"`
	Feedback             []string         `json:"feedback"`
	PendingChange        *FeedbackRequest `json:"pending_change,omitempty"`
	Result               *Result          `json:"result,omitempty"`
	ReadySHA             string           `json:"ready_sha,omitempty"`
	ReadyScope           string           `json:"ready_scope,omitempty"`
	// Historical hash is retained only when reading old delivery records.
	ReadyStories  string            `json:"ready_stories,omitempty"`
	ApprovedSHA   string            `json:"approved_sha,omitempty"`
	PR            *PullRequest      `json:"pull_request,omitempty"`
	Published     map[string]string `json:"published_stories,omitempty"`
	PublishedEpic string            `json:"published_epic,omitempty"`
}
type PullRequest struct {
	URL     string `json:"url"`
	Number  int    `json:"number"`
	State   string `json:"state"`
	HeadSHA string `json:"headRefOid"`
}

// Hosting is the only external side effect besides Git and the connector.
// Tests replace it without pretending to have opened a real pull request.
type Hosting interface {
	Find(context.Context, string, string, string) (*PullRequest, error)
	Create(context.Context, string, string, string, string, string) (*PullRequest, error)
	Read(context.Context, string, int) (*PullRequest, error)
}

type Service struct {
	Config    config.Config
	Connector connector.Connector
	Hosting   Hosting
}

func New(cfg config.Config, conn connector.Connector) *Service {
	return &Service{Config: cfg, Connector: conn, Hosting: GitHub{}}
}
