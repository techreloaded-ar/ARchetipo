package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
)

// The documented JSON payloads use snake_case keys. readStructuredInput decodes
// them with yaml.v3, which ignores json tags: a key without its yaml tag was
// silently dropped, so `increment publish` always saw an empty head_sha and
// answered "the result changed after it was displayed".
func TestIncrementPayloadKeepsSnakeCaseKeys(t *testing.T) {
	raw := `{"revision":2,"head_sha":"0d63deeebfb54bc8fa73e5ed92d1911936d7d708","development_mode":"continuous","origin_conversation_id":"conv-1","story_codes":["US-1"],"title":"t","request":"r","feedback":"f","kind":"k","observation":"o","decision":"d"}`
	var got incrementPayload
	if err := readStructuredInput(strings.NewReader(raw), "-", &got); err != nil {
		t.Fatal(err)
	}
	want := incrementPayload{
		Observation:          "o",
		Decision:             "d",
		DevelopmentMode:      "continuous",
		Title:                "t",
		Request:              "r",
		OriginConversationID: "conv-1",
		Revision:             2,
		Feedback:             "f",
		Kind:                 "k",
		StoryCodes:           []string{"US-1"},
		HeadSHA:              "0d63deeebfb54bc8fa73e5ed92d1911936d7d708",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded payload = %+v, want %+v", got, want)
	}
}

func TestIncrementProposalKeepsStoryCodes(t *testing.T) {
	var got increment.Proposal
	if err := readStructuredInput(strings.NewReader(`{"intent":"# Spec","story_codes":["US-1","US-2"]}`), "-", &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.StoryCodes, []string{"US-1", "US-2"}) {
		t.Fatalf("story_codes = %v", got.StoryCodes)
	}
}
