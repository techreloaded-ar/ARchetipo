package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/inmemory"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
)

func TestConversationPreviewUsesItsWorkingDirectory(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	server, err := NewServer(inmemory.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.session().previews.Close()
	workdir := t.TempDir()
	if _, err = preview.Configure(workdir, []byte(`{"version":1,"directory":".","url":"http://127.0.0.1:41987/checkout"}`)); err != nil {
		t.Fatal(err)
	}
	record := conversationlog.Record{ID: "conv-preview", WorkingDir: workdir}
	if err = server.session().conversationStore().Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/workspace/conversations/conv-preview/preview", nil)
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	var view preview.View
	if err = json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Configured || view.Recipe.URL != "http://127.0.0.1:41987/checkout" {
		t.Fatalf("wrong working directory: %+v", view)
	}
}
