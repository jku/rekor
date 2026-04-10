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

import "context"

// LogReader abstracts the read operations performed by TrillianClient.
type LogReader interface {
	GetLeafAndProofByHash(ctx context.Context, hash []byte) *Response
	GetLeafAndProofByIndex(ctx context.Context, index int64) *Response
	GetLatest(ctx context.Context, leafSizeInt int64) *Response
	GetConsistencyProof(ctx context.Context, firstSize, lastSize int64) *Response
	GetLeavesByRange(ctx context.Context, startIndex, count int64) *Response
	GetLeafWithoutProof(ctx context.Context, index int64) *Response
}
