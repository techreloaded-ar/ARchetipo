package codex

import (
	"context"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

type actionOpener struct {
	provider *Provider
	cfg      settings
	dir      string
}

var _ localrun.ActionOpener = actionOpener{}

func (opener actionOpener) OpenAction(ctx context.Context, req execution.Request, prompt string) (*localrun.ActionRun, error) {
	p := opener.provider
	live, err := p.openSession(ctx, req, opener.cfg, opener.dir, prompt)
	if err != nil {
		return nil, err
	}
	return &localrun.ActionRun{Source: live.client, Process: live.process, Session: live.session, StartedAt: live.startedAt}, nil
}
