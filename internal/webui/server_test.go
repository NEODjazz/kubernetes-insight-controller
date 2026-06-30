package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexIncludesMarkdownRenderer(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()

	server.index(response, httptest.NewRequest("GET", "/", nil))

	body := response.Body.String()
	for _, expected := range []string{
		"function renderMarkdown(value)",
		"function renderCodeBlock(code,language)",
		"function renderTable(header,alignments,rows)",
		`new RegExp("^( {0,3})\\x60{3,}([^\\s\\x60]*)\\s*$")`,
		"codeIndent=fence[1].length",
		`<div class="table-wrap"><table>`,
		`<div class="code-block">`,
		"renderMarkdown(s.recommendations",
		`/^(https?:|mailto:|#|\/)/i.test(decoded)`,
		"rel=\"noopener noreferrer\"",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("index HTML does not contain %q", expected)
		}
	}
}
