//go:build liveprobe

package codex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// TestLiveCodexNativeSessionProvider exercises the production SessionProvider,
// not only the protocol helper: the same thread writes through a real skill,
// is released, resumes through a fresh app-server process and retains context.
func TestLiveCodexNativeSessionProvider(t *testing.T) {
	if os.Getenv("LIVE_CODEX") == "" {
		t.Skip("set LIVE_CODEX=1 to run the live Codex SessionProvider probe")
	}
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "native-provider-probe")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillBody := "---\nname: native-provider-probe\ndescription: Production adapter fixture.\n---\nWrite exactly NATIVE_PROVIDER_OK followed by a newline to provider-marker.txt, then answer only NATIVE_PROVIDER_OK.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	provider := New(Options{WorkingDir: func() (string, error) { return root, nil }})
	config := map[string]any{"sandbox": "workspace-write"}
	discovery, err := provider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{ProviderConfig: config, WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Models) == 0 {
		t.Fatal("Codex model/list returned no model")
	}
	var skill execution.SessionSkill
	for _, candidate := range discovery.Skills {
		if candidate.Name == "native-provider-probe" {
			skill = candidate
		}
	}
	if skill.Name == "" {
		t.Fatalf("Codex skills/list omitted the fixture: %#v", discovery.Skills)
	}
	model := discovery.Models[0]
	for _, candidate := range discovery.Models {
		if candidate.Default {
			model = candidate
			break
		}
	}
	effort := ""
	if len(model.Options) > 0 && len(model.Options[0].Choices) > 0 {
		effort = model.Options[0].Choices[0].Value
	}
	created, err := provider.CreateSession(ctx, execution.CreateSessionRequest{ConversationID: "live-codex-native", Environment: discovery.Environment})
	if err != nil {
		t.Fatal(err)
	}
	randomWord := "ZAFFIRO-" + strings.ToUpper(liveCodexRandomHex(t, 4))
	first, err := provider.StartTurn(ctx, execution.StartTurnRequest{Session: created.Session, TurnID: "turn-live-1", SubmissionID: "submission-live-1", Message: "Invoca la skill indicata. Memorizza anche " + randomWord + " senza scriverla nel file.", Model: model.ID, Options: mapIfLiveCodexEffort(effort), Skills: []execution.SessionSkill{skill}})
	if err != nil || first.Delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("first turn = %#v, %v", first, err)
	}
	waitLiveCodexNativeState(t, ctx, provider, created.Session, execution.TurnCompleted)
	marker, err := os.ReadFile(filepath.Join(root, "provider-marker.txt"))
	if err != nil || string(marker) != "NATIVE_PROVIDER_OK\n" {
		t.Fatalf("native skill did not write through the production adapter: %q, %v", marker, err)
	}
	if err := provider.ReleaseSession(ctx, execution.SessionRequest{Session: created.Session}); err != nil {
		t.Fatal(err)
	}
	resumed, err := provider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: created.Session})
	if err != nil || resumed.Session.Native.ID != created.Session.Native.ID {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
	second, err := provider.StartTurn(ctx, execution.StartTurnRequest{Session: resumed.Session, TurnID: "turn-live-2", SubmissionID: "submission-live-2", Message: "Qual era la parola casuale? Rispondi soltanto con quella.", Model: model.ID, Options: mapIfLiveCodexEffort(effort)})
	if err != nil || second.Delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("second turn = %#v, %v", second, err)
	}
	waitLiveCodexNativeState(t, ctx, provider, resumed.Session, execution.TurnCompleted)
	if answer := strings.TrimSpace(provider.nativeSession(created.Session.Native.ID).client.FinalMessage()); answer != randomWord {
		t.Fatalf("native resume lost context: got %q, want %q", answer, randomWord)
	}
	t.Logf("production SessionProvider wrote a file and resumed thread %s with model %s effort %s", created.Session.Native.ID, model.ID, effort)
}

func mapIfLiveCodexEffort(effort string) map[string]string {
	if effort == "" {
		return nil
	}
	return map[string]string{"effort": effort}
}

func waitLiveCodexNativeState(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata, state execution.TurnState) {
	t.Helper()
	for {
		snapshot, err := provider.ReadSession(ctx, execution.SessionRequest{Session: session})
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.CurrentTurn != nil && snapshot.CurrentTurn.State == state {
			return
		}
		// Un turno finito male non diventa mai lo stato atteso: senza questa
		// uscita la prova resta appesa fino al timeout e non dice cosa è
		// successo davvero.
		if snapshot.CurrentTurn != nil && snapshot.CurrentTurn.State != execution.TurnActive &&
			snapshot.CurrentTurn.State != execution.TurnWaitingInput && snapshot.CurrentTurn.State != execution.TurnWaitingApproval {
			t.Fatalf("Codex native turn reached %s instead of %s: %s", snapshot.CurrentTurn.State, state, snapshot.CurrentTurn.Error)
		}
		for _, approval := range snapshot.PendingApprovals {
			if _, err := provider.RespondSessionApproval(ctx, execution.SessionCommandRequest{
				Session: session, TurnID: snapshot.CurrentTurn.ID, SubmissionID: "live-approval-" + approval.ID,
				InteractionID: approval.ID, OptionID: localrun.ApprovalAllow,
			}); err != nil {
				t.Fatalf("answering live Codex approval %s: %v", approval.ID, err)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Codex native turn did not reach %s", state)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestLiveCodexResumeWithModelEffortAndSkill drives the persisted app-server
// contract that View needs: discover the runtime's real model and skill
// catalogs, run one turn, release the app-server process, resume the same
// thread in a fresh process, and change model and effort on the next turn.
// The random word is deliberately absent from the resumed prompt.
//
// LIVE_CODEX=1 go test -tags liveprobe -run TestLiveCodexResumeWithModelEffortAndSkill -timeout 10m ./internal/execution/codex/
func TestLiveCodexResumeWithModelEffortAndSkill(t *testing.T) {
	if os.Getenv("LIVE_CODEX") == "" {
		t.Skip("set LIVE_CODEX=1 to run the live Codex resume probe")
	}
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "native-protocol-probe")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: native-protocol-probe\ndescription: Fixture for the native session protocol probe.\n---\nWrite exactly SKILL_FIXTURE_OK followed by a newline to native-skill-marker.txt, then answer only SKILL_FIXTURE_OK.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	firstProcess, firstClient := startLiveCodexProtocol(t, ctx, root)
	models := listLiveCodexModels(t, ctx, firstClient)
	firstModel, secondModel, firstEffort, secondEffort := selectLiveCodexModels(t, models)
	skillsResult := callLiveCodex(t, ctx, firstClient, "skills/list", map[string]any{"cwds": []string{root}, "forceReload": true})
	if !strings.Contains(string(skillsResult), "native-protocol-probe") {
		t.Fatalf("skills/list omitted the fixture: %s", skillsResult)
	}

	threadResult := callLiveCodex(t, ctx, firstClient, methodThreadStart, map[string]any{
		"cwd": root, "sandbox": "workspace-write", "approvalPolicy": "never", "ephemeral": false,
		"model": firstModel,
	})
	threadID, reportedFirstModel, _ := decodeLiveCodexThread(t, threadResult)
	firstClient.mu.Lock()
	firstClient.threadID = threadID
	firstClient.mu.Unlock()
	randomWord := "AMBRA-" + strings.ToUpper(liveCodexRandomHex(t, 4))
	firstFinal := startLiveCodexTurn(t, ctx, firstClient, threadID,
		"Invoca $native-protocol-probe. Memorizza anche la parola "+randomWord+" senza scriverla in file.", firstModel, firstEffort)
	if strings.TrimSpace(firstFinal) != "SKILL_FIXTURE_OK" {
		t.Fatalf("fixture skill returned %q", firstFinal)
	}
	marker, err := os.ReadFile(filepath.Join(root, "native-skill-marker.txt"))
	if err != nil || string(marker) != "SKILL_FIXTURE_OK\n" {
		t.Fatalf("the fixture skill did not perform its write: %q, %v", marker, err)
	}
	_ = firstProcess.Close()
	select {
	case <-firstClient.Gone():
	case <-ctx.Done():
		t.Fatal("the first app-server process did not close")
	}

	secondProcess, secondClient := startLiveCodexProtocol(t, ctx, root)
	t.Cleanup(func() { _ = secondProcess.Close() })
	resumeResult := callLiveCodex(t, ctx, secondClient, "thread/resume", map[string]any{
		"threadId": threadID, "cwd": root, "sandbox": "workspace-write", "approvalPolicy": "never", "model": secondModel,
	})
	resumedID, reportedSecondModel, _ := decodeLiveCodexThread(t, resumeResult)
	if resumedID != threadID {
		t.Fatalf("thread/resume returned %q, want %q", resumedID, threadID)
	}
	secondClient.mu.Lock()
	secondClient.threadID = threadID
	secondClient.mu.Unlock()
	secondFinal := startLiveCodexTurn(t, ctx, secondClient, threadID,
		"Qual era la parola casuale? Rispondi soltanto con quella.", secondModel, secondEffort)
	if strings.TrimSpace(secondFinal) != randomWord {
		t.Fatalf("resume lost native context: got %q, want %q", secondFinal, randomWord)
	}

	observed := callLiveCodex(t, ctx, secondClient, "thread/resume", map[string]any{"threadId": threadID})
	observedID, observedModel, observedEffort := decodeLiveCodexThread(t, observed)
	if observedID != threadID || observedModel != secondModel || observedEffort != secondEffort {
		t.Fatalf("sticky turn overrides were not reported: id=%q model=%q effort=%q", observedID, observedModel, observedEffort)
	}
	t.Logf("resumed %s across app-server processes; models %s (%s) -> %s (%s); initial response model=%s, resumed response model=%s", threadID, firstModel, firstEffort, secondModel, secondEffort, reportedFirstModel, reportedSecondModel)
}

// TestLiveCodexApprovalAndUserInput proves both kinds of server-initiated
// request against the real app-server. The test-only process wrapper routes
// those requests to the probe; production still owns no behavior from this
// experiment.
//
// LIVE_CODEX=1 go test -tags liveprobe -run TestLiveCodexApprovalAndUserInput -timeout 10m ./internal/execution/codex/
func TestLiveCodexApprovalAndUserInput(t *testing.T) {
	if os.Getenv("LIVE_CODEX") == "" {
		t.Skip("set LIVE_CODEX=1 to run the live Codex server-request probe")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	process, client := startLiveCodexProtocol(t, ctx, root)
	t.Cleanup(func() { _ = process.Close() })
	models := listLiveCodexModels(t, ctx, client)
	model, _, effort, _ := selectLiveCodexModels(t, models)

	threadResult := callLiveCodex(t, ctx, client, methodThreadStart, map[string]any{
		"cwd": root, "sandbox": "workspace-write", "approvalPolicy": "untrusted", "ephemeral": true, "model": model,
	})
	threadID, _, _ := decodeLiveCodexThread(t, threadResult)
	client.mu.Lock()
	client.threadID = threadID
	client.mu.Unlock()

	beginLiveCodexTurn(t, ctx, client, threadID,
		"Esegui con il tool shell il comando `printf 'APPROVED\\n' > approved.txt`, poi rispondi soltanto APPROVAL_DONE.", nil)
	approvedRequest := waitLiveCodexServerRequest(t, ctx, process, "item/commandExecution/requestApproval", client)
	respondLiveCodexServerRequest(t, process, approvedRequest, map[string]any{"decision": "accept"})
	approvedFinal := waitLiveCodexTurn(t, ctx, client)
	if strings.TrimSpace(approvedFinal) != "APPROVAL_DONE" {
		t.Fatalf("accepted command returned %q", approvedFinal)
	}
	approvedBody, err := os.ReadFile(filepath.Join(root, "approved.txt"))
	if err != nil || string(approvedBody) != "APPROVED\n" {
		t.Fatalf("accepted command did not run: %q, %v", approvedBody, err)
	}

	beginLiveCodexTurn(t, ctx, client, threadID,
		"Esegui con il tool shell il comando `printf 'DENIED\\n' > denied.txt`. Se viene negato, rispondi soltanto APPROVAL_DENIED.", nil)
	deniedRequest := waitLiveCodexServerRequest(t, ctx, process, "item/commandExecution/requestApproval", client)
	respondLiveCodexServerRequest(t, process, deniedRequest, map[string]any{"decision": "decline"})
	deniedFinal := waitLiveCodexTurn(t, ctx, client)
	if strings.TrimSpace(deniedFinal) != "APPROVAL_DENIED" {
		t.Fatalf("declined command returned %q", deniedFinal)
	}
	if _, err := os.Stat(filepath.Join(root, "denied.txt")); !os.IsNotExist(err) {
		t.Fatalf("declined command created denied.txt: %v", err)
	}

	beginLiveCodexTurn(t, ctx, client, threadID,
		"Usa obbligatoriamente request_user_input per chiedere una scelta tra ROSSO e BLU. Dopo la risposta, rispondi soltanto con la scelta ricevuta.",
		map[string]any{"collaborationMode": map[string]any{"mode": "plan", "settings": map[string]any{"model": model, "reasoning_effort": effort}}})
	inputRequest := waitLiveCodexServerRequest(t, ctx, process, "item/tool/requestUserInput", client)
	if !strings.Contains(string(inputRequest.Params), "ROSSO") || !strings.Contains(string(inputRequest.Params), "BLU") {
		t.Fatalf("request_user_input did not carry the choices: %s", inputRequest.Params)
	}
	respondLiveCodexServerRequest(t, process, inputRequest, map[string]any{
		"answers": map[string]any{"colore": map[string]any{"answers": []string{"BLU"}}},
	})
	inputFinal := waitLiveCodexTurn(t, ctx, client)
	if !strings.Contains(strings.ToUpper(inputFinal), "BLU") {
		t.Fatalf("the user-input answer did not return to the turn: %q", inputFinal)
	}
	t.Log("accepted and declined real command approvals; answered a real item/tool/requestUserInput request")
}

// TestLiveCodexHTTPServerLifecycle starts real development servers through the
// real harness and observes them at the boundaries View has to keep apart: the
// end of the turn that started one, the interrupt of a turn holding one, and
// the release of the app-server runtime that owns the session. It is the twin
// of the Claude probe of the same name, at the same production boundary, so the
// two answers can be read side by side.
//
// Nothing here claims a service survives a restart of View. What is
// demonstrated is what a person actually needs: the session comes back, and
// from inside it the service can be checked and stopped.
//
// Whatever is still listening when the test ends is stopped by pid.
//
//	LIVE_CODEX=1 go test -tags liveprobe -run TestLiveCodexHTTPServerLifecycle -timeout 15m ./internal/execution/codex/
func TestLiveCodexHTTPServerLifecycle(t *testing.T) {
	if os.Getenv("LIVE_CODEX") == "" {
		t.Skip("set LIVE_CODEX=1 to run the live Codex HTTP lifecycle probe")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "http-probe.txt"), []byte("HTTP_CODEX_OK\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detachedPort := reserveLiveCodexPort(t)
	foregroundPort := reserveLiveCodexPort(t)
	detachedURL := fmt.Sprintf("http://127.0.0.1:%d/http-probe.txt", detachedPort)
	foregroundURL := fmt.Sprintf("http://127.0.0.1:%d/http-probe.txt", foregroundPort)
	detachedPID := filepath.Join(root, "detached.pid")
	foregroundPID := filepath.Join(root, "foreground.pid")
	t.Cleanup(func() { stopLiveCodexPID(detachedPID); stopLiveCodexPID(foregroundPID) })

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	provider := New(Options{WorkingDir: func() (string, error) { return root, nil }})
	config := map[string]any{"sandbox": "danger-full-access"}
	discovery, err := provider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{ProviderConfig: config, WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	created, err := provider.CreateSession(ctx, execution.CreateSessionRequest{ConversationID: "live-http-codex", Environment: discovery.Environment})
	if err != nil {
		t.Fatal(err)
	}

	// One: a development server started in the background, and a turn that ends
	// while it keeps serving.
	detachedPrompt := fmt.Sprintf(
		"Sei dentro una sandbox usa e getta di un test automatico: la directory di lavoro contiene solo il file di fixture http-probe.txt. "+
			"Il test verifica che un server di sviluppo avviato da te sopravviva alla fine del turno. "+
			"Avvia con il tool shell un server statico sulla porta %d di 127.0.0.1 in background, scrivendone il PID nel file detached.pid della directory di lavoro, "+
			"con `nohup python3 -m http.server %d --bind 127.0.0.1 </dev/null >/dev/null 2>&1 & echo $! > detached.pid; disown` — i tre redirect e il disown servono perché il comando torni subito. "+
			"Il test lo fermerà da sé alla fine. Quando il PID è scritto rispondi soltanto DETACHED_STARTED.",
		detachedPort, detachedPort)
	if started, err := provider.StartTurn(ctx, execution.StartTurnRequest{
		Session: created.Session, TurnID: "turn-http-1", SubmissionID: "submission-http-1", Message: detachedPrompt,
	}); err != nil || started.Delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("starting the background server: %#v, %v", started, err)
	}
	defer dumpLiveCodexSession(t, provider, created.Session)
	detachedFileCtx, cancelDetachedFile := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelDetachedFile()
	waitForLiveCodexFileAnsweringApprovals(t, detachedFileCtx, provider, created.Session, detachedPID, "the background server PID")
	waitLiveCodexNativeState(t, ctx, provider, created.Session, execution.TurnCompleted)
	// Measured and not asserted: whether a process the harness detaches outlives
	// the tool call that started it is the harness's answer, not ARchetipo's,
	// and the two harnesses do not give the same one.
	httpCtx, cancelHTTP := context.WithTimeout(ctx, 20*time.Second)
	defer cancelHTTP()
	afterTurn := pollLiveCodexHTTP(httpCtx, detachedURL, "HTTP_CODEX_OK\n")

	// Two: a server held in the foreground of a turn, and that turn interrupted.
	foregroundPrompt := fmt.Sprintf(
		"Sempre nella stessa sandbox del test: adesso serve un secondo server statico, questa volta tenuto in primo piano dal tuo comando, "+
			"così il test può interrompere il turno mentre gira e misurare che ne è del processo. "+
			"Esegui con il tool shell `sh -c 'echo $$ > foreground.pid; exec python3 -m http.server %d --bind 127.0.0.1'` e resta in attesa mentre il comando è attivo.",
		foregroundPort)
	if started, err := provider.StartTurn(ctx, execution.StartTurnRequest{
		Session: created.Session, TurnID: "turn-http-2", SubmissionID: "submission-http-2", Message: foregroundPrompt,
	}); err != nil || started.Delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("starting the foreground server: %#v, %v", started, err)
	}
	foregroundFileCtx, cancelForegroundFile := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelForegroundFile()
	waitForLiveCodexFileAnsweringApprovals(t, foregroundFileCtx, provider, created.Session, foregroundPID, "the foreground server PID")
	foregroundCtx, cancelForeground := context.WithTimeout(ctx, 90*time.Second)
	defer cancelForeground()
	waitForLiveCodexHTTPAnsweringApprovals(t, foregroundCtx, provider, created.Session, foregroundURL, "HTTP_CODEX_OK\n")
	if _, err := provider.InterruptTurn(ctx, execution.SessionCommandRequest{
		Session: created.Session, TurnID: "turn-http-2", SubmissionID: "submission-http-2-interrupt",
	}); err != nil {
		t.Fatalf("interrupting the foreground turn: %v", err)
	}
	waitLiveCodexTurnEnd(t, ctx, provider, created.Session)
	foregroundAfterInterrupt := liveCodexHTTPAvailable(foregroundURL, "HTTP_CODEX_OK\n")
	detachedAfterInterrupt := liveCodexHTTPAvailable(detachedURL, "HTTP_CODEX_OK\n")

	// Three: the runtime released, which is what View does when it leaves a
	// workspace or is shut down.
	if err := provider.ReleaseSession(ctx, execution.SessionRequest{Session: created.Session}); err != nil {
		t.Fatal(err)
	}
	detachedAfterRelease := liveCodexHTTPAvailable(detachedURL, "HTTP_CODEX_OK\n")
	foregroundAfterRelease := liveCodexHTTPAvailable(foregroundURL, "HTTP_CODEX_OK\n")

	// Four: the session resumed by a new provider — a new View process — and
	// used to check and stop the service from inside the conversation.
	restarted := New(Options{WorkingDir: func() (string, error) { return root, nil }})
	resumed, err := restarted.ResumeSession(ctx, execution.ResumeSessionRequest{Session: created.Session})
	if err != nil {
		t.Fatalf("resuming the session that owns the servers: %v", err)
	}
	restartPrompt := fmt.Sprintf(
		"Siamo sempre nella sandbox del test, ed è la stessa conversazione in cui avevi avviato i server: la sessione è stata ripresa dopo che il runtime era stato rilasciato. "+
			"Con il tool shell verifica prima se qualcosa è ancora in ascolto sulle porte %d e %d di 127.0.0.1, poi riavvia il servizio sulla porta %d con "+
			"`sh -c 'echo $$ > foreground.pid; exec python3 -m http.server %d --bind 127.0.0.1'` e resta in attesa mentre il comando è attivo.",
		detachedPort, foregroundPort, foregroundPort, foregroundPort)
	if started, err := restarted.StartTurn(ctx, execution.StartTurnRequest{
		Session: resumed.Session, TurnID: "turn-http-3", SubmissionID: "submission-http-3", Message: restartPrompt,
	}); err != nil || started.Delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("asking the resumed session to restart the service: %#v, %v", started, err)
	}
	defer dumpLiveCodexSession(t, restarted, resumed.Session)
	restartCtx, cancelRestart := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelRestart()
	// The whole point of the resume: the service can be brought back from
	// inside the conversation that was holding it.
	waitForLiveCodexHTTPAnsweringApprovals(t, restartCtx, restarted, resumed.Session, foregroundURL, "HTTP_CODEX_OK\n")
	if _, err := restarted.InterruptTurn(ctx, execution.SessionCommandRequest{
		Session: resumed.Session, TurnID: "turn-http-3", SubmissionID: "submission-http-3-interrupt",
	}); err != nil {
		t.Fatalf("interrupting the restarted service turn: %v", err)
	}
	waitLiveCodexTurnEnd(t, ctx, restarted, resumed.Session)
	stopLiveCodexPID(detachedPID)
	stopLiveCodexPID(foregroundPID)
	waitForLiveCodexHTTPDown(t, ctx, detachedURL)
	waitForLiveCodexHTTPDown(t, ctx, foregroundURL)
	if err := restarted.ReleaseSession(ctx, execution.SessionRequest{Session: resumed.Session}); err != nil {
		t.Fatal(err)
	}
	t.Logf("background server reachable: after its turn=%v, after an interrupt of another turn=%v, after the runtime was released=%v", afterTurn, detachedAfterInterrupt, detachedAfterRelease)
	t.Logf("foreground server reachable: after the interrupt of its own turn=%v, after the runtime was released=%v", foregroundAfterInterrupt, foregroundAfterRelease)
	t.Log("the session was resumed by a second provider and the service was restarted and reached from inside it")
	t.Log("nothing was left listening when the probe returned")
}

// pollLiveCodexHTTP answers whether the fixture ever became reachable within
// the budget, instead of failing when it does not. It is for the measurements
// this probe records rather than requires.
func pollLiveCodexHTTP(ctx context.Context, url, wanted string) bool {
	for {
		if liveCodexHTTPAvailable(url, wanted) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitForLiveCodexHTTPAnsweringApprovals waits for the fixture to answer while
// granting the permissions the harness asks for on the way, which a turn that
// holds a server in the foreground needs.
func waitForLiveCodexHTTPAnsweringApprovals(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata, url, wanted string) {
	t.Helper()
	for {
		if liveCodexHTTPAvailable(url, wanted) {
			return
		}
		snapshot, err := provider.ReadSession(ctx, execution.SessionRequest{Session: session})
		if err != nil {
			t.Fatal(err)
		}
		answerLiveCodexApprovals(t, ctx, provider, session, snapshot)
		select {
		case <-ctx.Done():
			t.Fatalf("the HTTP fixture at %s never served %q", url, wanted)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// answerLiveCodexApprovals grants whatever the harness is asking permission for.
//
// The native session deliberately opens its thread with the "untrusted"
// approval policy, because in View there is a person on the other end of the
// permission bridge. In this probe the test is that person: without it the
// first shell command of every turn waits for an answer nobody gives, which is
// exactly how this probe failed before — the tool started and never returned.
func answerLiveCodexApprovals(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata, snapshot execution.SessionSnapshot) {
	t.Helper()
	if snapshot.CurrentTurn == nil {
		return
	}
	for _, approval := range snapshot.PendingApprovals {
		if _, err := provider.RespondSessionApproval(ctx, execution.SessionCommandRequest{
			Session: session, TurnID: snapshot.CurrentTurn.ID, SubmissionID: "live-approval-" + approval.ID,
			InteractionID: approval.ID, OptionID: localrun.ApprovalAllow,
		}); err != nil {
			t.Fatalf("answering live Codex approval %s: %v", approval.ID, err)
		}
	}
}

// waitForLiveCodexFileAnsweringApprovals waits for a file the harness has been
// asked to write, granting the permissions it asks for while it waits. A turn
// that holds a server in the foreground never ends, so waiting for the turn
// first is not an option here.
func waitForLiveCodexFileAnsweringApprovals(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata, name, what string) {
	t.Helper()
	for {
		if info, err := os.Stat(name); err == nil && info.Size() > 0 {
			return
		} else if err != nil && !os.IsNotExist(err) {
			t.Fatalf("checking %s: %v", what, err)
		}
		snapshot, err := provider.ReadSession(ctx, execution.SessionRequest{Session: session})
		if err != nil {
			t.Fatal(err)
		}
		answerLiveCodexApprovals(t, ctx, provider, session, snapshot)
		select {
		case <-ctx.Done():
			t.Fatalf("%s did not appear", what)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitLiveCodexTurnEndAnsweringApprovals is waitLiveCodexTurnEnd for a turn
// that still has permissions to ask for.
func waitLiveCodexTurnEndAnsweringApprovals(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata) execution.TurnState {
	t.Helper()
	for {
		snapshot, err := provider.ReadSession(ctx, execution.SessionRequest{Session: session})
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.CurrentTurn != nil {
			switch snapshot.CurrentTurn.State {
			case execution.TurnCompleted, execution.TurnInterrupted, execution.TurnFailed:
				return snapshot.CurrentTurn.State
			}
		}
		answerLiveCodexApprovals(t, ctx, provider, session, snapshot)
		select {
		case <-ctx.Done():
			t.Fatal("the Codex turn never ended")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitLiveCodexTurnEnd waits for the turn to be over, whichever way it ended.
// It is deliberately not waitLiveCodexNativeState: an interrupted turn never
// becomes COMPLETED, and asking for that state after an interrupt is asking for
// a failure.
func waitLiveCodexTurnEnd(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata) execution.TurnState {
	t.Helper()
	for {
		snapshot, err := provider.ReadSession(ctx, execution.SessionRequest{Session: session})
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.CurrentTurn != nil {
			switch snapshot.CurrentTurn.State {
			case execution.TurnCompleted, execution.TurnInterrupted, execution.TurnFailed:
				return snapshot.CurrentTurn.State
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("the Codex turn never ended")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// dumpLiveCodexSession writes what the harness actually said into the test log,
// so a live probe that fails on a missing file fails with its evidence.
func dumpLiveCodexSession(t *testing.T, provider *Provider, metadata execution.SessionMetadata) {
	t.Helper()
	session := provider.nativeSession(metadata.Native.ID)
	if session == nil {
		t.Log("the session is no longer held by this provider")
		return
	}
	session.mu.Lock()
	events := append([]execution.RunEvent(nil), session.events...)
	session.mu.Unlock()
	for _, event := range events {
		t.Logf("[%d] %s %s", event.ID, event.Kind, strings.TrimSpace(event.Text))
	}
}

type liveCodexModel struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Default bool   `json:"isDefault"`
	Efforts []struct {
		Value string `json:"reasoningEffort"`
	} `json:"supportedReasoningEfforts"`
}

type liveCodexProcess struct {
	inner    localrun.Process
	lines    chan []byte
	requests chan rpcMessage
}

func interceptLiveCodexProcess(inner localrun.Process) *liveCodexProcess {
	process := &liveCodexProcess{inner: inner, lines: make(chan []byte, 256), requests: make(chan rpcMessage, 16)}
	go func() {
		defer close(process.lines)
		defer close(process.requests)
		for line := range inner.Lines() {
			var message rpcMessage
			if json.Unmarshal(line, &message) == nil && len(message.ID) > 0 && message.Method != "" {
				process.requests <- message
				continue
			}
			process.lines <- line
		}
	}()
	return process
}

func (p *liveCodexProcess) Send(line []byte) error     { return p.inner.Send(line) }
func (p *liveCodexProcess) Lines() <-chan []byte       { return p.lines }
func (p *liveCodexProcess) Signal() error              { return p.inner.Signal() }
func (p *liveCodexProcess) Wait() (int, string, error) { return p.inner.Wait() }
func (p *liveCodexProcess) Close() error               { return p.inner.Close() }

func startLiveCodexProtocol(t *testing.T, ctx context.Context, root string) (*liveCodexProcess, *appServer) {
	t.Helper()
	inner, err := localrun.ExecStarter{}.Start(ctx, root, "codex", buildArgs())
	if err != nil {
		t.Fatalf("starting codex app-server: %v", err)
	}
	process := interceptLiveCodexProcess(inner)
	client := newAppServer(process, localrun.NewSession("live-native-probe", nil))
	go client.consume()
	callLiveCodex(t, ctx, client, methodInitialize, map[string]any{
		"clientInfo":   map[string]any{"name": "archetipo-live-probe", "version": "1"},
		"capabilities": map[string]any{"experimentalApi": true},
	})
	if err := client.notify(methodInitialized, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	return process, client
}

func callLiveCodex(t *testing.T, ctx context.Context, client *appServer, method string, params any) json.RawMessage {
	t.Helper()
	result, err := client.call(ctx, method, params)
	if err != nil {
		t.Fatalf("%s failed: %v", method, err)
	}
	return result
}

func listLiveCodexModels(t *testing.T, ctx context.Context, client *appServer) []liveCodexModel {
	t.Helper()
	result := callLiveCodex(t, ctx, client, "model/list", map[string]any{"includeHidden": false})
	var response struct {
		Data []liveCodexModel `json:"data"`
	}
	if err := json.Unmarshal(result, &response); err != nil || len(response.Data) < 2 {
		t.Fatalf("model/list did not return two usable models: %s, %v", result, err)
	}
	return response.Data
}

func selectLiveCodexModels(t *testing.T, models []liveCodexModel) (string, string, string, string) {
	t.Helper()
	first := models[0]
	for _, model := range models {
		if model.Default {
			first = model
			break
		}
	}
	second := models[0]
	if second.Model == first.Model {
		second = models[1]
	}
	if len(first.Efforts) == 0 || len(second.Efforts) < 2 {
		t.Fatalf("models do not advertise enough effort choices: %#v %#v", first, second)
	}
	return first.Model, second.Model, first.Efforts[0].Value, second.Efforts[len(second.Efforts)-1].Value
}

func decodeLiveCodexThread(t *testing.T, result json.RawMessage) (string, string, string) {
	t.Helper()
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
	}
	if err := json.Unmarshal(result, &response); err != nil || response.Thread.ID == "" {
		t.Fatalf("thread response has no identity: %s, %v", result, err)
	}
	return response.Thread.ID, response.Model, response.ReasoningEffort
}

func startLiveCodexTurn(t *testing.T, ctx context.Context, client *appServer, threadID, prompt, model, effort string) string {
	t.Helper()
	extra := map[string]any{}
	if model != "" {
		extra["model"] = model
	}
	if effort != "" {
		extra["effort"] = effort
	}
	beginLiveCodexTurn(t, ctx, client, threadID, prompt, extra)
	return waitLiveCodexTurn(t, ctx, client)
}

func beginLiveCodexTurn(t *testing.T, ctx context.Context, client *appServer, threadID, prompt string, extra map[string]any) {
	t.Helper()
	client.mu.Lock()
	client.turnOnce = sync.Once{}
	client.turnDone = make(chan struct{})
	client.completed = false
	client.agent.Reset()
	client.lastFull = ""
	client.mu.Unlock()
	params := map[string]any{
		"threadId": threadID,
		"input":    []any{map[string]any{"type": "text", "text": prompt}},
	}
	for name, value := range extra {
		params[name] = value
	}
	result := callLiveCodex(t, ctx, client, methodTurnStart, params)
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(result, &response); err != nil || response.Turn.ID == "" {
		t.Fatalf("turn/start returned no identity: %s, %v", result, err)
	}
	client.mu.Lock()
	client.turnID = response.Turn.ID
	client.mu.Unlock()
}

func waitLiveCodexTurn(t *testing.T, ctx context.Context, client *appServer) string {
	t.Helper()
	select {
	case <-client.TurnDone():
	case <-client.Gone():
		t.Fatal("app-server ended before turn/completed")
	case <-ctx.Done():
		t.Fatal("turn did not complete before the technical timeout")
	}
	if !client.Completed() {
		t.Fatal("turn ended without turn/completed")
	}
	return client.FinalMessage()
}

func waitLiveCodexServerRequest(t *testing.T, ctx context.Context, process *liveCodexProcess, method string, client *appServer) rpcMessage {
	t.Helper()
	for {
		select {
		case request, ok := <-process.requests:
			if !ok {
				t.Fatalf("app-server ended before %s", method)
			}
			if request.Method == method {
				return request
			}
			t.Fatalf("app-server requested unexpected method %s while waiting for %s", request.Method, method)
		case <-client.TurnDone():
			t.Fatalf("turn completed before %s; final=%q", method, client.FinalMessage())
		case <-ctx.Done():
			t.Fatalf("%s did not arrive before the technical timeout", method)
		}
	}
}

func respondLiveCodexServerRequest(t *testing.T, process *liveCodexProcess, request rpcMessage, result any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": json.RawMessage(request.ID), "result": result})
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Send(payload); err != nil {
		t.Fatalf("answering %s: %v", request.Method, err)
	}
}

func reserveLiveCodexPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func liveCodexHTTPAvailable(url, wanted string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get(url)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return err == nil && response.StatusCode == http.StatusOK && string(body) == wanted
}

func stopLiveCodexPID(name string) {
	body, err := os.ReadFile(name)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil || pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Kill()
	}
}

func waitForLiveCodexHTTPDown(t *testing.T, ctx context.Context, url string) {
	t.Helper()
	for liveCodexHTTPAvailable(url, "HTTP_CODEX_OK\n") {
		select {
		case <-ctx.Done():
			t.Fatal("HTTP fixture remained reachable after its PID was stopped")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func liveCodexRandomHex(t *testing.T, size int) string {
	t.Helper()
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw)
}

// TestLiveCodexPlansASpec dispatches one real spec.plan action to the real
// Codex binary in a real workspace. It is the only check that exercises the
// flags in defaultExecArgs against the CLI that has to accept them — every
// other test goes through the Runner seam and would happily agree on a flag
// Codex rejects, which is exactly how `--full-auto` survived a full review.
//
// It is behind a build tag because it costs an agent run and mutates the
// backlog: the spec it names really is planned. Run it by hand after a Codex
// upgrade, or when defaultExecArgs changes:
//
//	LIVE_WORKSPACE=/path/to/workspace LIVE_SPEC=US-0XX \
//	  go test -tags liveprobe -run TestLiveCodexPlansASpec -timeout 40m ./internal/execution/codex/
func TestLiveCodexPlansASpec(t *testing.T) {
	root := os.Getenv("LIVE_WORKSPACE")
	spec := os.Getenv("LIVE_SPEC")
	if root == "" || spec == "" {
		t.Skip("set LIVE_WORKSPACE and LIVE_SPEC to run the live Codex probe")
	}

	p := New(Options{WorkingDir: func() (string, error) { return root, nil }})
	res, err := p.Execute(context.Background(), execution.Request{
		ExecutionID:    "live-probe",
		SpecCode:       spec,
		Action:         execution.ActionPlan,
		Capability:     execution.CapabilitySpecPlan,
		ProviderConfig: map[string]any{"timeout_seconds": 1800},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	t.Logf("payload: %s", string(res.Payload))
}

// TestLiveCodexDialogue drives the real Codex binary through the real app
// server protocol: handshake, thread, turn, and — while the turn is still
// alive — a steer and an interrupt.
//
// It exists for the same reason as the probe above, and it is the only check
// that can catch the mistake that matters here: every other test in this
// package goes through the process seam and would happily agree on a method
// name, a parameter or a refusal that Codex does not recognize. That is exactly
// how `--full-auto` survived a full review.
//
// It costs a few seconds of agent time, touches no backlog and writes nothing:
// the prompt asks for one word, in a temporary directory.
//
//	LIVE_CODEX=1 go test -tags liveprobe -run TestLiveCodexDialogue -timeout 5m ./internal/execution/codex/
func TestLiveCodexDialogue(t *testing.T) {
	if os.Getenv("LIVE_CODEX") == "" {
		t.Skip("set LIVE_CODEX=1 to run the live Codex dialogue probe")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	process, err := localrun.ExecStarter{}.Start(ctx, dir, "codex", buildArgs())
	if err != nil {
		t.Fatalf("starting the codex app server: %v", err)
	}
	session := localrun.NewSession("live-probe", nil)
	client := newAppServer(process, session)
	go client.consume()

	const prompt = "Conta lentamente da 1 a 40, un numero per riga, senza usare strumenti."
	const steered = "Fermati e rispondi solo CIAO."

	cfg := settings{Command: "codex", Sandbox: defaultSandbox, Timeout: 3 * time.Minute}
	if err := client.start(ctx, cfg, dir, prompt); err != nil {
		t.Fatalf("the handshake the production client speaks was refused: %v", err)
	}
	session.AttachDialogue(client)

	// The first event is the prompt itself, re-emitted by Codex as a user
	// message: this is the mechanism the whole dialogue rests on, and it is
	// verified here against the real binary and not against a double.
	waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
		return event.Kind == localrun.KindUserMessage && event.Text == prompt
	}, "the prompt re-emitted as a user message")

	// A steer issued in the instant between `turn/start` returning and the turn
	// actually starting is refused with `no active turn to steer` — observed
	// here — which is why the wait above comes first.
	if err := client.Send(ctx, steered); err != nil {
		assertDeliveredOrRefused(t, "turn/steer", err)
	} else {
		waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
			return event.Kind == localrun.KindUserMessage && event.Text == steered
		}, "the steered message re-emitted by the process")
	}

	assertDeliveredOrRefused(t, "turn/interrupt", client.Interrupt(ctx))

	select {
	case <-client.TurnDone():
	case <-ctx.Done():
		t.Fatal("the turn never ended")
	}
	restarted := startLiveCodexTurn(t, ctx, client, client.threadID, "Dopo l'interrupt rispondi soltanto RIPRESO.", "", "")
	if strings.TrimSpace(restarted) != "RIPRESO" {
		t.Fatalf("the turn after interrupt returned %q", restarted)
	}
	_ = process.Close()

	for _, event := range session.Events(0) {
		t.Logf("event %d %s %q", event.ID, event.Kind, event.Text)
	}
	if snapshot := session.Snapshot(); snapshot.State != execution.RunActive {
		t.Fatalf("the session closed itself: %#v — only the observed end of the process may do that", snapshot)
	}
}

// waitForLiveEvent polls the history for an event instead of sleeping for an
// arbitrary time.
func waitForLiveEvent(t *testing.T, ctx context.Context, session *localrun.Session, matches func(execution.RunEvent) bool, what string) {
	t.Helper()
	for {
		for _, event := range session.Events(0) {
			if matches(event) {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s never arrived", what)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func assertDeliveredOrRefused(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	reason, refused := execution.RefusalOf(err)
	if refused && reason == execution.RunRefusedNotActive {
		t.Logf("%s was refused because the turn had already ended, which is a valid outcome", what)
		return
	}
	t.Fatalf("%s failed against the real binary: %v", what, err)
}
