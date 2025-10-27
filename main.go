package main

import (
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

func (h *rpcHandler) Handle(context context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	switch request.Method {
	case "initialize":
		var params protocol.InitializeParams
		params.UnmarshalJSON(*request.Params)
		conn.Reply(context, request.ID, protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				CompletionProvider: &protocol.CompletionOptions{},
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
	}
}

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
