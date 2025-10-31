package main

import (
	"os"
	"slices"
	"testing"

	"github.com/myleshyson/lsprotocol-go/protocol"
)

func TestCaptureCompletions(t *testing.T) {
	makefile, err := os.ReadFile("testdata/captureCompletions.mk")
	if err != nil {
		t.Error(err)
	}
	completionsWant := []string{
		"snake_glue_function",
		"glued_variable",
		"target_echo_template",
	}
	completionsHave, err := captureCompletions(makefile)
	if !slices.Equal(completionsHave, completionsWant) || err != nil {
		t.Errorf("completions\nwant: %v\nhave: %v", completionsWant, completionsHave)
	}
}

func TestGetDefinitionRange(t *testing.T) {
	makefile, err := os.ReadFile("testdata/captureCompletions.mk")
	if err != nil {
		t.Error(err)
	}
	definitionRangeHave, err := getDefinitionRange(makefile, "target_echo_template")
	definitionRangeWant := protocol.Range{
		Start: protocol.Position{
			Character: 7,
			Line:      2,
		},
		End: protocol.Position{
			Character: 27,
			Line:      2,
		},
	}
	if definitionRangeWant.Start.Character != definitionRangeHave.Start.Character || definitionRangeWant.Start.Line != definitionRangeHave.Start.Line {
		t.Errorf("\ndefinition range want: %+v\ndefinition range have: %+v", definitionRangeWant, definitionRangeHave)
	}
}
