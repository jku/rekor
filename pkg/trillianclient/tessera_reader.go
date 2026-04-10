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
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/sigstore/rekor/pkg/util"
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

func (r *TesseraReader) GetLeafAndProofByHash(ctx context.Context, hash []byte) *Response {
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetLeafAndProofByHash not implemented"),
	}
}

func (r *TesseraReader) GetLeafAndProofByIndex(ctx context.Context, index int64) *Response {
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetLeafAndProofByIndex not implemented"),
	}
}

func (r *TesseraReader) GetLatest(ctx context.Context, leafSizeInt int64) *Response {
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
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetConsistencyProof not implemented"),
	}
}

func (r *TesseraReader) GetLeavesByRange(ctx context.Context, startIndex, count int64) *Response {
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetLeavesByRange not implemented"),
	}
}

func (r *TesseraReader) GetLeafWithoutProof(ctx context.Context, index int64) *Response {
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetLeafWithoutProof not implemented"),
	}
}
