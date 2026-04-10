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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/trillian/types"
	"github.com/transparency-dev/merkle/rfc6962"
	"github.com/transparency-dev/tessera/api/layout"
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

	leafData := []byte("example_leaf_data")
	leafHash := rfc6962.DefaultHasher.HashLeaf(leafData)

	// Write a dummy checkpoint file for tree size 1
	checkpointContent := fmt.Sprintf("example.com\n1\n%s\n", base64.StdEncoding.EncodeToString(leafHash))
	err := os.WriteFile(filepath.Join(tmpDir, "checkpoint"), []byte(checkpointContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write entry bundle 0 with 1 entry
	bundleBuf := &bytes.Buffer{}
	binary.Write(bundleBuf, binary.BigEndian, uint16(len(leafData)))
	bundleBuf.Write(leafData)

	bundlePath := filepath.Join(tmpDir, layout.EntriesPath(0, 1))
	err = os.MkdirAll(filepath.Dir(bundlePath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(bundlePath, bundleBuf.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write tile 0,0 with 1 leaf hash
	tilePath := filepath.Join(tmpDir, layout.TilePath(0, 0, 1))
	err = os.MkdirAll(filepath.Dir(tilePath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(tilePath, leafHash, 0644)
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTesseraReader(tmpDir)
	ctx := context.Background()

	resp := reader.GetLeafAndProofByIndex(ctx, 0)

	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLeafAndProofResult == nil {
		t.Fatal("Expected GetLeafAndProofResult to be non-nil")
	}

	// Verify content
	if string(resp.GetLeafAndProofResult.Leaf.LeafValue) != string(leafData) {
		t.Errorf("Expected leaf value %s, got %s", string(leafData), string(resp.GetLeafAndProofResult.Leaf.LeafValue))
	}
	if !bytes.Equal(resp.GetLeafAndProofResult.Leaf.MerkleLeafHash, leafHash) {
		t.Errorf("Expected leaf hash %x, got %x", leafHash, resp.GetLeafAndProofResult.Leaf.MerkleLeafHash)
	}
	if len(resp.GetLeafAndProofResult.Proof.Hashes) != 0 {
		t.Errorf("Expected empty proof for size 1, got %d hashes", len(resp.GetLeafAndProofResult.Proof.Hashes))
	}
}

func TestTesseraReader_GetLeavesByRange(t *testing.T) {
	tmpDir := t.TempDir()

	leafData := []byte("example_leaf_data")
	leafHash := rfc6962.DefaultHasher.HashLeaf(leafData)

	// Write a dummy checkpoint file for tree size 1
	checkpointContent := fmt.Sprintf("example.com\n1\n%s\n", base64.StdEncoding.EncodeToString(leafHash))
	err := os.WriteFile(filepath.Join(tmpDir, "checkpoint"), []byte(checkpointContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write entry bundle 0 with 1 entry
	bundleBuf := &bytes.Buffer{}
	binary.Write(bundleBuf, binary.BigEndian, uint16(len(leafData)))
	bundleBuf.Write(leafData)

	bundlePath := filepath.Join(tmpDir, layout.EntriesPath(0, 1))
	err = os.MkdirAll(filepath.Dir(bundlePath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(bundlePath, bundleBuf.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTesseraReader(tmpDir)
	ctx := context.Background()

	resp := reader.GetLeavesByRange(ctx, 0, 1)

	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLeavesByRangeResult == nil {
		t.Fatal("Expected GetLeavesByRangeResult to be non-nil")
	}

	if len(resp.GetLeavesByRangeResult.Leaves) != 1 {
		t.Errorf("Expected 1 leaf, got %d", len(resp.GetLeavesByRangeResult.Leaves))
	}
	if string(resp.GetLeavesByRangeResult.Leaves[0].LeafValue) != string(leafData) {
		t.Errorf("Expected leaf value %s, got %s", string(leafData), string(resp.GetLeavesByRangeResult.Leaves[0].LeafValue))
	}
}

func TestTesseraReader_GetConsistencyProof(t *testing.T) {
	tmpDir := t.TempDir()

	reader := NewTesseraReader(tmpDir)
	ctx := context.Background()

	// Test from size 0 to 1
	resp := reader.GetConsistencyProof(ctx, 0, 1)
	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if len(resp.GetConsistencyProofResult.Proof.Hashes) != 0 {
		t.Errorf("Expected empty proof, got %d hashes", len(resp.GetConsistencyProofResult.Proof.Hashes))
	}
}

func TestTesseraReader_GetLeafAndProofByIndex_Size2(t *testing.T) {
	tmpDir := t.TempDir()
	
	leaf0 := []byte("leaf0_data")
	leaf1 := []byte("leaf1_data")
	
	h0 := rfc6962.DefaultHasher.HashLeaf(leaf0)
	h1 := rfc6962.DefaultHasher.HashLeaf(leaf1)
	
	rootHash := rfc6962.DefaultHasher.HashChildren(h0, h1)
	
	// Write a dummy checkpoint file for tree size 2
	checkpointContent := fmt.Sprintf("example.com\n2\n%s\n", base64.StdEncoding.EncodeToString(rootHash))
	err := os.WriteFile(filepath.Join(tmpDir, "checkpoint"), []byte(checkpointContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write entry bundle 0 with 2 entries
	bundleBuf := &bytes.Buffer{}
	binary.Write(bundleBuf, binary.BigEndian, uint16(len(leaf0)))
	bundleBuf.Write(leaf0)
	binary.Write(bundleBuf, binary.BigEndian, uint16(len(leaf1)))
	bundleBuf.Write(leaf1)
	
	bundlePath := filepath.Join(tmpDir, layout.EntriesPath(0, 2))
	err = os.MkdirAll(filepath.Dir(bundlePath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(bundlePath, bundleBuf.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write tile 0,0 with 2 leaf hashes
	tilePath := filepath.Join(tmpDir, layout.TilePath(0, 0, 2))
	err = os.MkdirAll(filepath.Dir(tilePath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	tileBuf := append(h0, h1...)
	err = os.WriteFile(tilePath, tileBuf, 0644)
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTesseraReader(tmpDir)
	ctx := context.Background()

	// Fetch leaf 0
	resp := reader.GetLeafAndProofByIndex(ctx, 0)
	
	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLeafAndProofResult == nil {
		t.Fatal("Expected GetLeafAndProofResult to be non-nil")
	}
	
	// Verify content
	if string(resp.GetLeafAndProofResult.Leaf.LeafValue) != string(leaf0) {
		t.Errorf("Expected leaf value %s, got %s", string(leaf0), string(resp.GetLeafAndProofResult.Leaf.LeafValue))
	}
	if !bytes.Equal(resp.GetLeafAndProofResult.Leaf.MerkleLeafHash, h0) {
		t.Errorf("Expected leaf hash %x, got %x", h0, resp.GetLeafAndProofResult.Leaf.MerkleLeafHash)
	}
	
	// Proof for leaf 0 in size 2 should contain h1!
	if len(resp.GetLeafAndProofResult.Proof.Hashes) != 1 {
		t.Errorf("Expected proof of length 1, got %d", len(resp.GetLeafAndProofResult.Proof.Hashes))
	} else if !bytes.Equal(resp.GetLeafAndProofResult.Proof.Hashes[0], h1) {
		t.Errorf("Expected proof hash %x, got %x", h1, resp.GetLeafAndProofResult.Proof.Hashes[0])
	}
}
