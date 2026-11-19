package web

import (
	"errors"
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/signin"
)

// signinListView answers GET /api/signin. Where sign-in is not offered the
// accounts list is empty and no account command is run.
type signinListView struct {
	Available bool             `json:"available"`
	Accounts  []signin.Account `json:"accounts"`
}

func (s *Server) handleListSignins(w http.ResponseWriter, r *http.Request) {
	if !signin.Offered() {
		writeJSON(w, http.StatusOK, signinListView{Accounts: []signin.Account{}})
		return
	}
	writeJSON(w, http.StatusOK, signinListView{Available: true, Accounts: s.signins.Accounts(r.Context())})
}

func signinOffered() error {
	if signin.Offered() {
		return nil
	}
	return iox.NewConflict("sign-in is offered only in the isolated environment", "sign in from your own terminal with the agent's or GitHub's command", nil)
}

func writeSigninError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, signin.ErrUnknownKind):
		writeError(w, iox.NewNotFound("unknown account", "choose claude, codex or github", err))
	case errors.Is(err, signin.ErrNoCodeExpected):
		writeError(w, iox.NewConflict("this sign-in is not waiting for a code", "start the sign-in again", err))
	default:
		writeError(w, iox.NewConflict("the sign-in could not be started", "check that the command is installed in the environment", err))
	}
}

func (s *Server) handleStartSignin(w http.ResponseWriter, r *http.Request) {
	if err := signinOffered(); err != nil {
		writeError(w, err)
		return
	}
	flow, err := s.signins.Start(r.PathValue("kind"))
	if err != nil {
		writeSigninError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, flow)
}

func (s *Server) handleSigninCode(w http.ResponseWriter, r *http.Request) {
	if err := signinOffered(); err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	flow, err := s.signins.SubmitCode(r.PathValue("kind"), req.Code)
	if err != nil {
		writeSigninError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, flow)
}

func (s *Server) handleCancelSignin(w http.ResponseWriter, r *http.Request) {
	if err := signinOffered(); err != nil {
		writeError(w, err)
		return
	}
	if err := s.signins.Cancel(r.PathValue("kind")); err != nil {
		writeSigninError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}
