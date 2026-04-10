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

	"google.golang.org/grpc/codes"
)

// TesseraReader implements LogReader by reading from Tessera tiles.
// Currently a dummy implementation that returns errors.
type TesseraReader struct {
}

// NewTesseraReader creates a new dummy TesseraReader.
func NewTesseraReader() *TesseraReader {
	return &TesseraReader{}
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
	return &Response{
		Status: codes.Unimplemented,
		Err:    errors.New("TesseraReader.GetLatest not implemented"),
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
