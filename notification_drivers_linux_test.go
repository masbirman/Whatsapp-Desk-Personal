//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestBuildNotifySendArgsRejectsOptionInjection(t *testing.T) {
	args := buildNotifySendArgs("--help=evil", "body", "/tmp/icon.png")
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(strings.Join(args[0:1], " "), "--app-name") {
		t.Fatalf("app name must come first: %q", joined)
	}
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 {
		t.Fatalf("arguments must end with a -- separator before content: %q", joined)
	}
	rest := args[sep+1:]
	if len(rest) != 2 || rest[0] != "--help=evil" || rest[1] != "body" {
		t.Fatalf("content must be passed verbatim after --: %q", joined)
	}
	args = buildNotifySendArgs("title", "body", "")
	for _, arg := range args {
		if arg == "/tmp/icon.png" {
			t.Fatalf("empty icon path must not add an --icon flag: %q", strings.Join(args, " "))
		}
	}
}
