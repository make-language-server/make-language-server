package main

import (
	"os"
	"slices"
	"testing"
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
	if definitionRangeHave.Start.Character != 0 {
		t.Errorf("nono")
	}
}
