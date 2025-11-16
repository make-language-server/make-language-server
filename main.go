package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"

	tree_sitter_make "github.com/make-language-server/tree-sitter-make/bindings/go"
	"github.com/myleshyson/lsprotocol-go/protocol"
	"github.com/sourcegraph/jsonrpc2"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type rpcHandler struct{}

var fileProtocolRegexp *regexp.Regexp
var documents map[string]string

func (h *rpcHandler) Handle(context context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	switch request.Method {
	case "initialize":
		var params protocol.InitializeParams
		params.UnmarshalJSON(*request.Params)
		conn.Reply(context, request.ID, protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				CompletionProvider: &protocol.CompletionOptions{},
				DefinitionProvider: &protocol.Or2[bool, protocol.DefinitionOptions]{Value: true},
				TextDocumentSync:   &protocol.Or2[protocol.TextDocumentSyncOptions, protocol.TextDocumentSyncKind]{Value: protocol.TextDocumentSyncKindFull},
			},
		})
	case "shutdown":
		conn.Close()
	case "textDocument/completion":
		// TODO: log error. does it work like this?
		// logMessageNotification := protocol.LogMessageNotification{
		// 	Method: protocol.WindowLogMessageMethod,
		// 	Params: protocol.LogMessageParams{
		// 		Message: "",
		// 		Type:    protocol.MessageTypeError,
		// 	},
		// }
		var completionParams protocol.CompletionParams
		completionParams.UnmarshalJSON(*request.Params)
		text, _ := os.ReadFile(fileProtocolRegexp.ReplaceAllString(string(completionParams.TextDocument.Uri), ""))
		var completionItems []protocol.CompletionItem
		completions, _ := captureCompletions(text)
		for _, completion := range completions {
			completionItems = append(completionItems, protocol.CompletionItem{Label: completion, InsertText: completion})
		}
		conn.Reply(context, request.ID, completionItems)
	case "textDocument/definition":
		var definitionParams protocol.DefinitionParams
		definitionParams.UnmarshalJSON(*request.Params)
		text, _ := os.ReadFile(fileProtocolRegexp.ReplaceAllString(string(definitionParams.TextDocument.Uri), ""))
		lines := bytes.Split(text, []byte("\n"))
		line := string(lines[definitionParams.Position.Line])
		character := int(definitionParams.Position.Character)
		var begin int
		var end int
		for i := 0; character+i > 0; i-- {
			if string(line[character+i]) == " " ||
				string(line[character+i]) == "," ||
				string(line[character+i]) == "(" ||
				string(line[character+i]) == ")" {
				begin = character + i + 1
				break
			}
		}
		for k := 0; int(definitionParams.Position.Character)+k <= len(line); k++ {
			if string(line[character+k]) == " " ||
				string(line[character+k]) == "," ||
				string(line[character+k]) == "(" ||
				string(line[character+k]) == ")" {
				end = character + k
				break
			}
		}
		definitionRange, _ := getDefinitionRange(text, line[begin:end])
		result := protocol.Location{Range: definitionRange, Uri: definitionParams.TextDocument.Uri}
		conn.Reply(context, request.ID, result)
	case "textDocument/didOpen":
		// TODO: https://pkg.go.dev/github.com/myleshyson/lsprotocol-go@v1.0.2/protocol#DidOpenTextDocumentParams
	case "textDocument/didChange":
		var didChangeTextDocumentParams protocol.DidChangeTextDocumentParams
		didChangeTextDocumentParams.UnmarshalJSON(*request.Params)
		document, ok := didChangeTextDocumentParams.ContentChanges[0].Value.(protocol.TextDocumentContentChangeWholeDocument)
		if ok {
			// TODO: update documents
		}
	}
}

type node struct {
	ByteRangeStart, ByteRangeEnd, StartPositionColumn, StartPositionRow, EndPositionColumn, EndPositionRow uint
}

func captureNodes(text []byte) ([]node, error) {
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
	captures := completionsQueryCursor.Captures(completionQuery, root, text)
	var nodes []node
	for {
		match, _ := captures.Next()
		if match == nil {
			break
		}
		for _, match := range match.Captures {
			byteRangeStart, byteRangeEnd := match.Node.ByteRange()
			nodes = append(nodes, node{
				ByteRangeStart:      byteRangeStart,
				ByteRangeEnd:        byteRangeEnd,
				StartPositionColumn: match.Node.StartPosition().Column,
				StartPositionRow:    match.Node.StartPosition().Row,
				EndPositionColumn:   match.Node.EndPosition().Column,
				EndPositionRow:      match.Node.EndPosition().Row,
			})
		}
	}
	return nodes, nil
}

func captureCompletions(text []byte) ([]string, error) {
	nodes, err := captureNodes(text)
	if err != nil {
		return nil, err
	}
	var completions []string
	for _, node := range nodes {
		completions = append(completions, string(text[node.ByteRangeStart:node.ByteRangeEnd]))
	}
	return completions, nil
}

func getDefinitionRange(text []byte, word string) (protocol.Range, error) {
	nodes, err := captureNodes(text)
	if err != nil {
		return protocol.Range{}, err
	}
	for _, node := range nodes {
		if string(text[node.ByteRangeStart:node.ByteRangeEnd]) == word {
			return protocol.Range{
				Start: protocol.Position{
					Character: uint32(node.StartPositionColumn),
					Line:      uint32(node.StartPositionRow),
				},
				End: protocol.Position{
					Character: uint32(node.EndPositionColumn),
					Line:      uint32(node.EndPositionRow),
				},
			}, nil
		}
	}
	return protocol.Range{}, nil
}

type stream struct {
	in  io.Reader
	out io.Writer
}

func (s stream) Read(b []byte) (int, error) {
	return os.Stdin.Read(b)
}

func (s stream) Write(b []byte) (int, error) {
	return os.Stdout.Write(b)
}

func (s stream) Close() error {
	return nil
}

func main() {
	fileProtocolRegexp = regexp.MustCompile("^file://")
	context := context.Background()
	conn := jsonrpc2.NewConn(context, jsonrpc2.NewBufferedStream(stream{}, jsonrpc2.VSCodeObjectCodec{}), &rpcHandler{})
	<-conn.DisconnectNotify()
}
