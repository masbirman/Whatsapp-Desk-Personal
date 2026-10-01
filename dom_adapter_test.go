package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDOMAdapterSelectorResolution(t *testing.T) {
	node := os.Getenv("WA_NODE_PATH")
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			t.Skip("node is not available")
		}
	}

	cmd := exec.Command(node, filepath.Join("testdata", "dom_adapter_test.js"), "dom_adapter.js")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("DOM adapter fixture failed: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
}
