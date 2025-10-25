package main

import (
	"log"

	tree_sitter_make "github.com/make-language-server/tree-sitter-make/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

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

func main() {
	log.Fatal("TODO")
}
