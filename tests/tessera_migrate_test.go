//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestTesseraMigrate(t *testing.T) {
	// 1. Ensure data exists in Trillian
	// We upload a test file to ensure the log is not empty and we have known data
	t.Log("Uploading test data to Trillian...")
	runCli(t, "upload", "--artifact", "test_file.txt", "--signature", "test_file.sig", "--public-key", "test_public_key.key")

	treeID := getTreeID(t)
	t.Logf("Using tree ID: %d", treeID)

	// Create a temp dir for Tessera tiles
	tempDir, err := os.MkdirTemp("", "tessera-migrate-e2e")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 2. Run tessera-migrate
	// We assume it was built by the calling script (e.g. tests/e2e-test.sh) and is in the root directory
	t.Log("Running migration...")
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

	// 3. Start Tessera Rekor Server in background
	t.Log("Starting Tessera Rekor server...")
	// tests/e2e-test.sh builds rekor-server as a test binary in the root directory
	serverCmd := exec.Command("../rekor-server", "serve",
		"--rekor_server.backend=tessera",
		"--rekor_server.tessera.storage_path="+tempDir,
		"--port=3001",
		"--rekor_server.address=0.0.0.0",
		"--rekor_server.signer=" + os.Getenv("REKOR_TEST_KEY_PATH"),
		fmt.Sprintf("--trillian_log_server.tlog_id=%d", treeID),
	)
	
	// Ensure we don't inherit a TMPDIR that breaks things
	serverCmd.Env = append(os.Environ(), "TMPDIR=/tmp", "REKOR_MEMORY_SIGNER_SEED=memory-signer-seed")

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start Tessera server: %v", err)
	}
	defer func() {
		t.Log("Stopping Tessera server...")
		if err := serverCmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill server process: %v", err)
		}
	}()

	// Wait for server to be healthy
	t.Log("Waiting for Tessera server to be healthy...")
	healthy := false
	for i := 0; i < 10; i++ {
		resp, err := http.Get("http://localhost:3001/api/v1/log")
		if err == nil && resp.StatusCode == http.StatusOK {
			healthy = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !healthy {
		t.Fatalf("Tessera server failed to become healthy")
	}

	// 4. Verify API Parity
	t.Log("Verifying API parity...")

	// Compare public keys
	respPubTrillian, err := http.Get("http://localhost:3000/api/v1/log/publicKey")
	if err != nil {
		t.Fatalf("Failed to get public key from Trillian: %v", err)
	}
	defer respPubTrillian.Body.Close()
	
	respPubTessera, err := http.Get("http://localhost:3001/api/v1/log/publicKey")
	if err != nil {
		t.Fatalf("Failed to get public key from Tessera: %v", err)
	}
	defer respPubTessera.Body.Close()

	pubTrillian, _ := io.ReadAll(respPubTrillian.Body)
	pubTessera, _ := io.ReadAll(respPubTessera.Body)

	if string(pubTrillian) != string(pubTessera) {
		t.Errorf("Public keys do not match!\nTrillian: %s\nTessera: %s", string(pubTrillian), string(pubTessera))
	} else {
		t.Log("Public keys match!")
	}
	
	// Get latest log info from both
	respTrillian, err := http.Get("http://localhost:3000/api/v1/log")
	if err != nil {
		t.Fatalf("Failed to get log info from Trillian: %v", err)
	}
	defer respTrillian.Body.Close()
	
	respTessera, err := http.Get("http://localhost:3001/api/v1/log")
	if err != nil {
		t.Fatalf("Failed to get log info from Tessera: %v", err)
	}
	defer respTessera.Body.Close()

	var logTrillian, logTessera map[string]interface{}
	if err := json.NewDecoder(respTrillian.Body).Decode(&logTrillian); err != nil {
		t.Fatalf("Failed to decode Trillian log info: %v", err)
	}
	if err := json.NewDecoder(respTessera.Body).Decode(&logTessera); err != nil {
		t.Fatalf("Failed to decode Tessera log info: %v", err)
	}

	sizeTrillian := int(logTrillian["treeSize"].(float64))
	sizeTessera := int(logTessera["treeSize"].(float64))

	t.Logf("Tree sizes: Trillian=%d, Tessera=%d", sizeTrillian, sizeTessera)

	if sizeTrillian != sizeTessera {
		t.Errorf("Tree sizes do not match: Trillian=%d, Tessera=%d", sizeTrillian, sizeTessera)
	}

	// Compare all entries
	t.Logf("Comparing %d entries...", sizeTrillian)
	for i := 0; i < sizeTrillian; i++ {
		urlTrillian := fmt.Sprintf("http://localhost:3000/api/v1/log/entries?logIndex=%d", i)
		urlTessera := fmt.Sprintf("http://localhost:3001/api/v1/log/entries?logIndex=%d", i)

		respEntryTrillian, err := http.Get(urlTrillian)
		if err != nil {
			t.Fatalf("Failed to get entry from Trillian at index %d: %v", i, err)
		}
		defer respEntryTrillian.Body.Close()

		respEntryTessera, err := http.Get(urlTessera)
		if err != nil {
			t.Fatalf("Failed to get entry from Tessera at index %d: %v", i, err)
		}
		defer respEntryTessera.Body.Close()

		if respEntryTrillian.StatusCode != http.StatusOK {
			t.Errorf("Trillian returned status %d for index %d", respEntryTrillian.StatusCode, i)
			continue
		}
		if respEntryTessera.StatusCode != http.StatusOK {
			t.Errorf("Tessera returned status %d for index %d", respEntryTessera.StatusCode, i)
			continue
		}

		bodyTrillian, _ := io.ReadAll(respEntryTrillian.Body)
		bodyTessera, _ := io.ReadAll(respEntryTessera.Body)

		var mapTrillian, mapTessera map[string]interface{}
		json.Unmarshal(bodyTrillian, &mapTrillian)
		json.Unmarshal(bodyTessera, &mapTessera)

		if len(mapTrillian) != 1 || len(mapTessera) != 1 {
			t.Errorf("Expected exactly one key in response at index %d", i)
			continue
		}

		var uuidTrillian, uuidTessera string
		var valTrillian, valTessera map[string]interface{}

		for k, v := range mapTrillian {
			uuidTrillian = k
			valTrillian = v.(map[string]interface{})
		}
		for k, v := range mapTessera {
			uuidTessera = k
			valTessera = v.(map[string]interface{})
		}

		if uuidTrillian != uuidTessera {
			t.Errorf("UUIDs do not match at index %d: Trillian=%s, Tessera=%s", i, uuidTrillian, uuidTessera)
		}

		// Compare Body
		if valTrillian["body"] != valTessera["body"] {
			t.Errorf("Bodies do not match at index %d", i)
		}

		verTrillian := valTrillian["verification"].(map[string]interface{})
		verTessera := valTessera["verification"].(map[string]interface{})

		// Compare signedEntryTimestamp
		if verTrillian["signedEntryTimestamp"] != verTessera["signedEntryTimestamp"] {
			t.Errorf("signedEntryTimestamp does not match at index %d", i)
		}

		// Compare Inclusion Proof Hashes
		proofTrillian := verTrillian["inclusionProof"].(map[string]interface{})
		proofTessera := verTessera["inclusionProof"].(map[string]interface{})

		hashesTrillian := proofTrillian["hashes"].([]interface{})
		hashesTessera := proofTessera["hashes"].([]interface{})

		if len(hashesTrillian) != len(hashesTessera) {
			t.Errorf("Inclusion proof hashes length mismatch at index %d", i)
		} else {
			for j := range hashesTrillian {
				if hashesTrillian[j] != hashesTessera[j] {
					t.Errorf("Inclusion proof hash mismatch at index %d at position %d", i, j)
				}
			}
		}

		// Compare checkpoint in inclusionProof
		if proofTrillian["checkpoint"] != proofTessera["checkpoint"] {
			t.Logf("Trillian checkpoint at index %d:\n%s", i, proofTrillian["checkpoint"])
			t.Logf("Tessera checkpoint at index %d:\n%s", i, proofTessera["checkpoint"])
			t.Errorf("checkpoint does not match at index %d", i)
		}

		// We ignore integratedTime because Tessera doesn't preserve it yet.
	}
}
