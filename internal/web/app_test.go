package web

import (
	"path/filepath"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	templates, err := parseTemplates(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	if templates.Lookup("faq") == nil {
		t.Fatal("FAQ template is not registered")
	}
}
