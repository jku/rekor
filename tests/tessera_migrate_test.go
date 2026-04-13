//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTesseraMigrate(t *testing.T) {
	treeID := getTreeID(t)
	t.Logf("Using tree ID: %d", treeID)

	// Create a temp dir for Tessera tiles
	tempDir, err := os.MkdirTemp("", "tessera-migrate-e2e")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Run tessera-migrate
	// We assume it was built by the calling script (e.g. tests/e2e-test.sh) and is in the root directory
	migrateCmd := exec.Command("../tessera-migrate",
		"-trillian-addr", "localhost:8090",
		"-tree-id", fmt.Sprintf("%d", treeID),
		"-tessera-dir", tempDir,
		"-batch-size", "50",
	)
	
	output, err := migrateCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Migration failed: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Migration output: %s", string(output))

	// Verify output
	checkpointPath := fmt.Sprintf("%s/checkpoint", tempDir)
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		t.Errorf("Checkpoint file was not generated")
	}

	// Only expect tiles if tree size > 0
	if !strings.Contains(string(output), "Source tree size: 0") {
		tilesPath := fmt.Sprintf("%s/tile", tempDir)
		if _, err := os.Stat(tilesPath); os.IsNotExist(err) {
			t.Errorf("Tiles directory was not generated")
		}
	} else {
		t.Log("Skipping tiles directory check because tree size is 0")
	}
}
