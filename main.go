package main

import (
	"bytes"
	"context"
	"fmt"
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
				DefinitionProvider: &protocol.Or2[bool, protocol.DefinitionOptions]{Value: true},
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
		f, _ := os.OpenFile("log.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		fmt.Fprintf(f, "%d:%d\n", begin, end)
		fmt.Fprintf(f, "%s\n", line[begin:end])
		definitionRange, _ := getDefinitionRange(text, line[begin:end])
		fmt.Fprintf(f, "%+v\n", definitionRange)
	}
}

func getDefinitionRange(text []byte, word string) (protocol.Range, error) {
	definitionCaptures, err := captureWords(text)
	if err != nil {
		return protocol.Range{}, err
	}
	for {
		match, _ := definitionCaptures.Next()
		if match == nil {
			break
		}
		for _, match := range match.Captures {
			byteRangeStart, byteRangeEnd := match.Node.ByteRange()
			startPosition, endPosition := match.Node.StartPosition(), match.Node.EndPosition()
			if string(text[byteRangeStart:byteRangeEnd]) == word {
				return protocol.Range{
					Start: protocol.Position{
						Character: uint32(startPosition.Column),
						Line:      uint32(startPosition.Row),
					},
					End: protocol.Position{
						Character: uint32(endPosition.Column),
						Line:      uint32(endPosition.Row),
					},
				}, nil
			}
		}
	}
	return protocol.Range{}, nil
}

func captureWords(text []byte) (tree_sitter.QueryCaptures, error) {
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
		return tree_sitter.QueryCaptures{}, err
	}
	defer completionQuery.Close()
	completionsQueryCursor := tree_sitter.NewQueryCursor()
	defer completionsQueryCursor.Close()
	return completionsQueryCursor.Captures(completionQuery, root, text), nil
}

func captureCompletions(text []byte) ([]string, error) {
	completionCaptures, err := captureWords(text)
	if err != nil {
		return nil, err
	}
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
