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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/sigstore/rekor/pkg/util"
	"github.com/transparency-dev/merkle/rfc6962"
	"github.com/transparency-dev/tessera/api"
	"github.com/transparency-dev/tessera/api/layout"
	"github.com/transparency-dev/tessera/client"
	"google.golang.org/grpc/codes"
)

// TesseraReader implements LogReader by reading from Tessera tiles.
type TesseraReader struct {
	basePath string
}

// NewTesseraReader creates a new TesseraReader reading from the specified path.
func NewTesseraReader(basePath string) *TesseraReader {
	return &TesseraReader{basePath: basePath}
}

func (r *TesseraReader) tileFetcher(_ context.Context, level, index uint64, p uint8) ([]byte, error) {
	if p > 0 {
		path := filepath.Join(r.basePath, layout.TilePath(level, index, p))
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	path := filepath.Join(r.basePath, layout.TilePath(level, index, 0))
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

func (r *TesseraReader) entryBundleFetcher(_ context.Context, bundleIndex uint64, p uint8) ([]byte, error) {
	if p > 0 {
		path := filepath.Join(r.basePath, layout.EntriesPath(bundleIndex, p))
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	path := filepath.Join(r.basePath, layout.EntriesPath(bundleIndex, 0))
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

func (r *TesseraReader) GetLeafAndProofByHash(_ context.Context, _ []byte) *Response {
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetLeafAndProofByHash not implemented"),
	}
}

func (r *TesseraReader) GetLeafAndProofByIndex(ctx context.Context, index int64) *Response {
	if index < 0 {
		return &Response{
			Status: codes.InvalidArgument,
			Err:    fmt.Errorf("invalid index %d", index),
		}
	}

	latestResp := r.GetLatest(ctx, 0)
	if latestResp.Status != codes.OK {
		return latestResp
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(latestResp.GetLatestResult.SignedLogRoot.LogRoot); err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}
	treeSize := root.TreeSize

	if uint64(index) >= treeSize {
		return &Response{
			Status: codes.NotFound,
			Err:    fmt.Errorf("index %d out of bounds for tree size %d", index, treeSize),
		}
	}

	bundleIndex := uint64(index) / layout.EntryBundleWidth
	pBundle := layout.PartialTileSize(0, bundleIndex, treeSize)

	bundleRaw, err := r.entryBundleFetcher(ctx, bundleIndex, pBundle)
	if err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	var bundle api.EntryBundle
	if err := bundle.UnmarshalText(bundleRaw); err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	intraBundleIndex := uint64(index) % layout.EntryBundleWidth
	if intraBundleIndex >= uint64(len(bundle.Entries)) {
		return &Response{
			Status: codes.Internal,
			Err:    fmt.Errorf("index %d not found in bundle %d (len %d)", index, bundleIndex, len(bundle.Entries)),
		}
	}
	leafData := bundle.Entries[intraBundleIndex]
	leafHash := rfc6962.DefaultHasher.HashLeaf(leafData)

	pb, err := client.NewProofBuilder(ctx, treeSize, r.tileFetcher)
	if err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	proof, err := pb.InclusionProof(ctx, uint64(index))
	if err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	return &Response{
		Status: codes.OK,
		GetLeafAndProofResult: &trillian.GetEntryAndProofResponse{
			Leaf: &trillian.LogLeaf{
				LeafValue:      leafData,
				LeafIndex:      index,
				MerkleLeafHash: leafHash,
			},
			Proof: &trillian.Proof{
				LeafIndex: index,
				Hashes:    proof,
			},
			SignedLogRoot: latestResp.GetLatestResult.SignedLogRoot,
		},
	}
}

func (r *TesseraReader) GetLatest(_ context.Context, _ int64) *Response {
	cpRaw, err := os.ReadFile(filepath.Join(r.basePath, "checkpoint"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Response{
				Status: codes.NotFound,
				Err:    err,
			}
		}
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}
	var cp util.Checkpoint
	if err := cp.UnmarshalCheckpoint(cpRaw); err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	root := types.LogRootV1{
		TreeSize: cp.Size,
		RootHash: cp.Hash,
	}
	rootBytes, err := root.MarshalBinary()
	if err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	return &Response{
		Status: codes.OK,
		GetLatestResult: &trillian.GetLatestSignedLogRootResponse{
			SignedLogRoot: &trillian.SignedLogRoot{
				LogRoot: rootBytes,
			},
		},
	}
}

func (r *TesseraReader) GetConsistencyProof(ctx context.Context, firstSize, lastSize int64) *Response {
	if firstSize < 0 || lastSize < 0 || firstSize > lastSize {
		return &Response{
			Status: codes.InvalidArgument,
			Err:    fmt.Errorf("invalid sizes: first=%d, last=%d", firstSize, lastSize),
		}
	}

	if firstSize == 0 {
		return &Response{
			Status: codes.OK,
			GetConsistencyProofResult: &trillian.GetConsistencyProofResponse{
				Proof: &trillian.Proof{
					Hashes: [][]byte{},
				},
			},
		}
	}

	pb, err := client.NewProofBuilder(ctx, uint64(lastSize), r.tileFetcher)
	if err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	proofHashes, err := pb.ConsistencyProof(ctx, uint64(firstSize), uint64(lastSize))
	if err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}

	return &Response{
		Status: codes.OK,
		GetConsistencyProofResult: &trillian.GetConsistencyProofResponse{
			Proof: &trillian.Proof{
				Hashes: proofHashes,
			},
		},
	}
}

func (r *TesseraReader) GetLeavesByRange(ctx context.Context, startIndex, count int64) *Response {
	if startIndex < 0 || count <= 0 {
		return &Response{
			Status: codes.InvalidArgument,
			Err:    fmt.Errorf("invalid startIndex %d or count %d", startIndex, count),
		}
	}

	latestResp := r.GetLatest(ctx, 0)
	if latestResp.Status != codes.OK {
		return latestResp
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(latestResp.GetLatestResult.SignedLogRoot.LogRoot); err != nil {
		return &Response{
			Status: codes.Internal,
			Err:    err,
		}
	}
	treeSize := root.TreeSize

	if uint64(startIndex) >= treeSize {
		return &Response{
			Status: codes.NotFound,
			Err:    fmt.Errorf("startIndex %d out of bounds for tree size %d", startIndex, treeSize),
		}
	}

	endIndex := uint64(startIndex + count)
	if endIndex > treeSize {
		endIndex = treeSize
	}

	leaves := []*trillian.LogLeaf{}
	currIndex := uint64(startIndex)

	for currIndex < endIndex {
		bundleIndex := currIndex / layout.EntryBundleWidth
		pBundle := layout.PartialTileSize(0, bundleIndex, treeSize)

		bundleRaw, err := r.entryBundleFetcher(ctx, bundleIndex, pBundle)
		if err != nil {
			return &Response{
				Status: codes.Internal,
				Err:    err,
			}
		}

		var bundle api.EntryBundle
		if err := bundle.UnmarshalText(bundleRaw); err != nil {
			return &Response{
				Status: codes.Internal,
				Err:    err,
			}
		}

		intraBundleIndex := currIndex % layout.EntryBundleWidth

		for intraBundleIndex < uint64(len(bundle.Entries)) && currIndex < endIndex {
			leafData := bundle.Entries[intraBundleIndex]
			leafHash := rfc6962.DefaultHasher.HashLeaf(leafData)

			leaves = append(leaves, &trillian.LogLeaf{
				LeafValue:      leafData,
				LeafIndex:      int64(currIndex),
				MerkleLeafHash: leafHash,
			})

			intraBundleIndex++
			currIndex++
		}
	}

	return &Response{
		Status: codes.OK,
		GetLeavesByRangeResult: &trillian.GetLeavesByRangeResponse{
			Leaves: leaves,
		},
	}
}

func (r *TesseraReader) GetLeafWithoutProof(ctx context.Context, index int64) *Response {
	return r.GetLeavesByRange(ctx, index, 1)
}
