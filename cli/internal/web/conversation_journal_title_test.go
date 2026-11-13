package web

import "testing"

func TestPlainTitleRemovesInlineMarkdownBeforeTruncating(t *testing.T) {
	got := plainTitle("Rivedi **Fatturazione clienti** e `Marginalità`", 20)
	if want := "Rivedi Fatturazione " + conversationTitleEllipsis; got != want {
		t.Fatalf("plainTitle = %q, want %q", got, want)
	}
}
