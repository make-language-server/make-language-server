package main

import (
	"os"
	"slices"
	"testing"
)

func Test(t *testing.T) {
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
