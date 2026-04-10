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
	"testing"

	"google.golang.org/grpc/codes"
)

func TestTesseraReader_GetLatest(t *testing.T) {
	reader := NewTesseraReader()
	ctx := context.Background()

	resp := reader.GetLatest(ctx, 0)
	if resp.Status != codes.OK {
		t.Logf("Current status (expected to fail until implemented): %v", resp.Status)
	}
	if resp.Err != nil {
		t.Logf("Current error (expected to fail until implemented): %v", resp.Err)
	}
	
	// These assertions will fail now, but should pass later.
	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLatestResult == nil {
		t.Error("Expected GetLatestResult to be non-nil")
	}
}

func TestTesseraReader_GetLeafAndProofByIndex(t *testing.T) {
	reader := NewTesseraReader()
	ctx := context.Background()

	resp := reader.GetLeafAndProofByIndex(ctx, 0)
	if resp.Status != codes.OK {
		t.Logf("Current status (expected to fail until implemented): %v", resp.Status)
	}
	if resp.Err != nil {
		t.Logf("Current error (expected to fail until implemented): %v", resp.Err)
	}

	// These assertions will fail now, but should pass later.
	if resp.Status != codes.OK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
	if resp.GetLeafAndProofResult == nil {
		t.Error("Expected GetLeafAndProofResult to be non-nil")
	}
}
