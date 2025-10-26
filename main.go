package main

import (
	"os"
	"regexp"

	tree_sitter_make "github.com/make-language-server/tree-sitter-make/bindings/go"
	"github.com/tliron/glsp"
	lsp "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var handler lsp.Handler
var fileProtocolRegexp *regexp.Regexp

func captureCompletions(text []byte) ([]string, error) {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	language := tree_sitter.NewLanguage(tree_sitter_make.Language())
	parser.SetLanguage(language)
	tree := parser.Parse(text, nil)
	defer tree.Close()
	root := tree.RootNode()
	completionQuery, err := tree_sitter.NewQuery(language, `
		(define_directive name: (word) @define_directive_name)
		(variable_assignment name: (word) @variable_assignment_name)
	`)
	if err != nil {
		return nil, err
	}
	defer completionQuery.Close()
	completionsQueryCursor := tree_sitter.NewQueryCursor()
	defer completionsQueryCursor.Close()
	completionCaptures := completionsQueryCursor.Captures(completionQuery, root, text)
	var completions []string
	for {
		match, _ := completionCaptures.Next()
		if match == nil {
			break
		}
		for _, match := range match.Captures {
			begin, end := match.Node.ByteRange()
			completions = append(completions, string(text[begin:end]))
		}
	}
	return completions, nil
}

func initialize(context *glsp.Context, params *lsp.InitializeParams) (any, error) {
	return lsp.InitializeResult{
		Capabilities: handler.CreateServerCapabilities(),
	}, nil
}

func shutdown(context *glsp.Context) error {
	return nil
}

func textDocumentCompletion(context *glsp.Context, params *lsp.CompletionParams) (any, error) {
	// TODO: use document synchronization
	text, err := os.ReadFile(fileProtocolRegexp.ReplaceAllString(params.TextDocument.URI, ""))
	if err != nil {
		return nil, err
	}
	completions, err := captureCompletions(text)
	if err != nil {
		return nil, err
	}
	var completionItems []lsp.CompletionItem
	for _, completion := range completions {
		completionItems = append(completionItems, lsp.CompletionItem{Label: completion, InsertText: &completion})
	}
	return completionItems, nil
}

func main() {
	fileProtocolRegexp = regexp.MustCompile("^file://")
	handler = lsp.Handler{
		Initialize:             initialize,
		Shutdown:               shutdown,
		TextDocumentCompletion: textDocumentCompletion,
	}
	server := server.NewServer(&handler, "server", false)
	server.RunStdio()
}
