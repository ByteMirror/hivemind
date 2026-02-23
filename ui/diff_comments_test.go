package ui

import (
	"strings"
	"testing"
)

func TestDiffPane_AddComment(t *testing.T) {
	d := NewDiffPane()
	d.AddComment("main.go", 5, "+", "newCode()", "this should use the existing helper")

	comments := d.GetComments()
	if len(comments["main.go"]) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments["main.go"]))
	}
	c := comments["main.go"][0]
	if c.Line != 5 || c.Comment != "this should use the existing helper" {
		t.Errorf("unexpected comment: %+v", c)
	}
}

func TestDiffPane_FormatCommentsMessage(t *testing.T) {
	d := NewDiffPane()
	d.AddComment("auth/handler.go", 42, "+", `token := r.Header.Get("Auth")`, `use "Authorization"`)
	d.AddComment("utils/parse.go", 18, "-", `return nil`, "don't remove this nil check")

	msg := d.FormatCommentsMessage()
	if !strings.Contains(msg, "auth/handler.go") {
		t.Error("expected auth/handler.go in message")
	}
	if !strings.Contains(msg, `use "Authorization"`) {
		t.Error("expected comment text in message")
	}
	if !strings.Contains(msg, "Please address these") {
		t.Error("expected closing instruction in message")
	}
}

func TestDiffPane_ClearComments(t *testing.T) {
	d := NewDiffPane()
	d.AddComment("main.go", 1, "+", "code", "comment")
	d.ClearComments()

	if len(d.GetComments()) != 0 {
		t.Error("expected comments cleared")
	}
}
