package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunCompletionBash(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"completion", "bash"}); err != nil {
		t.Fatalf("completion bash error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"complete -F _kronos_completion kronos", "backup", "restore", "user", "--no-color --output --server --token", "token) COMPREPLY", "inspect list revoke"} {
		if !strings.Contains(text, want) {
			t.Fatalf("bash completion missing %q in %s", want, text)
		}
	}
}

func TestRunCompletionZsh(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"completion", "zsh"}); err != nil {
		t.Fatalf("completion zsh error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "#compdef kronos") || !strings.Contains(text, "'completion'") || !strings.Contains(text, "'--no-color' '--output' '--server' '--token'") || !strings.Contains(text, "token) subcommands=('create' 'inspect' 'list' 'revoke' 'verify')") {
		t.Fatalf("zsh completion output = %q", text)
	}
}

func TestRunCompletionFish(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"completion", "fish"}); err != nil {
		t.Fatalf("completion fish error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "complete -c kronos") || !strings.Contains(text, "'token'") || !strings.Contains(text, "-l no-color") || !strings.Contains(text, "__fish_seen_subcommand_from 'agent'") || !strings.Contains(text, "'inspect list'") {
		t.Fatalf("fish completion output = %q", text)
	}
}

func TestRunCompletionRejectsMissingOrUnknownShell(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"completion"}); err == nil {
		t.Fatal("completion without shell error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"completion", "powershell"}); err == nil {
		t.Fatal("completion unknown shell error = nil, want error")
	}
}
