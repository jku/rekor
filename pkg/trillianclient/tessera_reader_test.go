//
// Copyright 2026 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trillianclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
)

func TestTesseraReader_GetLatest(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Write a dummy checkpoint file
	// Format: origin\nsize\nhash\n
	checkpointContent := "example.com\n42\nZXhhbXBsZWhhc2g=\n" // ZXhhbXBsZWhhc2g= is base64 for "examplehash"
	err := os.WriteFile(filepath.Join(tmpDir, "checkpoint"), []byte(checkpointContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTesseraReader(tmpDir)
	ctx := context.Background()

	resp := reader.GetLatest(ctx, 0)
	
	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLatestResult == nil {
		t.Fatal("Expected GetLatestResult to be non-nil")
	}
	
	// Verify content
	root := &types.LogRootV1{}
	if err := root.UnmarshalBinary(resp.GetLatestResult.SignedLogRoot.LogRoot); err != nil {
		t.Fatal(err)
	}
	if root.TreeSize != 42 {
		t.Errorf("Expected tree size 42, got %d", root.TreeSize)
	}
	if string(root.RootHash) != "examplehash" {
		t.Errorf("Expected root hash 'examplehash', got %s", string(root.RootHash))
	}
}

func TestTesseraReader_GetLeafAndProofByIndex(t *testing.T) {
	tmpDir := t.TempDir()
	reader := NewTesseraReader(tmpDir)
	ctx := context.Background()

	resp := reader.GetLeafAndProofByIndex(ctx, 0)
	
	// This assertion will fail now, but should pass later.
	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLeafAndProofResult == nil {
		t.Error("Expected GetLeafAndProofResult to be non-nil")
	}
}
