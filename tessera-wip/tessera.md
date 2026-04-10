# Plan: Modifying Rekor to use Tessera Backend (Read-Path Only)

This document outlines the plan to modify Rekor to work on top of the tile-based transparency log **Tessera** instead of **Trillian**.

## Objectives
*   **Focus on read path only** initially (assuming the log is in a read-only/frozen state).
*   **Migrate existing log**: Support migrating an existing Rekor log to this new implementation.
*   **No API changes**: The external Rekor HTTP API must remain unchanged.

## 1. Internal Abstraction

To minimize changes to the existing Rekor codebase, we should implement a compatibility layer that mimics the behavior of the current `TrillianClient` but fetches data from Tessera tiles.

The following methods from `pkg/trillianclient/trillian_client.go` are needed for the read path and must be implemented by the Tessera backend:

| Method | Purpose | Tessera Client Equivalent |
| :--- | :--- | :--- |
| `GetLeafAndProofByHash(ctx, hash)` | Fetch leaf and inclusion proof by Merkle leaf hash | `client.ProofBuilder.InclusionProof` (see [Lookup by Hash](#lookup-by-hash) below) |
| `GetLeafAndProofByIndex(ctx, index)` | Fetch leaf and inclusion proof by log index | `client.ProofBuilder.InclusionProof` + `client.GetEntryBundle` |
| `GetLatest(ctx, leafSize)` | Fetch the latest signed log root | `client.FetchCheckpoint` |
| `GetConsistencyProof(ctx, first, last)` | Fetch consistency proof between two tree sizes | `client.ProofBuilder.ConsistencyProof` |
| `GetLeavesByRange(ctx, start, count)` | Fetch a range of leaves without proofs | `client.GetEntryBundle` (iterating over tiles if needed) |
| `GetLeafWithoutProof(ctx, index)` | Fetch a single leaf by index | `client.GetEntryBundle` |


### Lookup by Hash

Tessera does not natively support lookup by hash. For the initial read-only migration, `GetLeafAndProofByHash` is left unimplemented. Production deployments requiring lookup by hash will need a sidecar index (e.g., a database mapping entry hash to log index).

### Response Translation

Tessera returns raw bytes for entries and proofs. The compatibility layer must translate these into the Trillian-specific structures currently expected by Rekor's handlers (e.g., `trillian.GetEntryAndProofResponse`).

### Configuration Flags

The following flags were added to `rekor-server` to support the Tessera backend:

*   `--rekor_server.backend`: Specifies the log backend to use. Options are `trillian` (default) and `tessera`.
*   `--rekor_server.tessera.storage_path`: Specifies the directory path where Tessera tiles and checkpoint are stored (applicable when backend is `tessera`).

## 2. Public Endpoints to Support

The following Rekor public HTTP read endpoints will be supported. They will use the internal abstraction (which maps to Tessera client calls) to fulfill requests.

### `GET /api/v1/log`
*   **Purpose**: Get information about the current state of the transparency log (tree size, root hash, signed tree head).
*   **Implementation**:
    *   Calls `GetLatest(ctx, leafSize)` on the internal abstraction.
    *   The internal abstraction fetches the static `checkpoint` file from Tessera storage using `client.FetchCheckpoint`.
    *   Returns the parsed checkpoint data (root hash, size) and the raw signed note.

### `GET /api/v1/log/entries`
*   **Purpose**: Retrieves an entry and inclusion proof from the transparency log by index.
*   **Implementation**:
    *   Calls `GetLeafAndProofByIndex(ctx, index)` on the internal abstraction.
    *   The abstraction uses `client.GetEntryBundle` to fetch the tile containing the entry and extract the specific entry data.
    *   It uses `client.ProofBuilder.InclusionProof` to construct the inclusion proof for that index against the latest checkpoint.
    *   Translates the result into a `LogEntry` structure with verification details.

### `GET /api/v1/log/entries/{entryUUID}`
*   **Purpose**: Get log entry and information required to generate an inclusion proof by UUID (which is usually the entry hash).
*   **Implementation**:
    *   Calls `GetLeafAndProofByHash(ctx, hash)` on the internal abstraction.
    *   *Note*: Tessera is indexed by sequence number, not by hash. We assume that Rekor's existing index storage (e.g., Redis) is still available and populated during migration to map hashes to indices.
    *   The abstraction looks up the index for the hash in the index storage, and then proceeds as in `GetLeafAndProofByIndex`.

### `GET /api/v1/log/proof`
*   **Purpose**: Get information required to generate a consistency proof between two tree sizes.
*   **Implementation**:
    *   Calls `GetConsistencyProof(ctx, firstSize, lastSize)` on the internal abstraction.
    *   The abstraction uses `client.ProofBuilder.ConsistencyProof` to fetch the necessary tile nodes and compute the proof hashes.
    *   Returns the hashes list.

## 3. Migration Plan

Since the target state is a read-only log, we can perform a static migration from Trillian to Tessera:

1.  **Export from Trillian**: Read leaves from the existing Trillian log using `GetLeavesByRange`.
2.  **Construct Entry Bundles**: Package the leaves into Tessera's entry bundle format (2-byte length prefix + data).
3.  **Write to Tessera**: Use a custom migration script or Tessera's `setEntryBundle` (from `migrate.go`) to write the bundles to the target storage (e.g., POSIX files or Cloud Storage).
4.  **Compute Hashes and Checkpoint**: Tessera's migration tools or a custom script must compute the Merkle tree hashes and generate the final checkpoint.

> [!NOTE]
> Could talk to Tessera folks about  "Trillian v1 to Tessera migration" -- maybe they can provide something esier to use.

## 4. Proposed Implementation Steps

### Phase 1: Research & Scaffolding (Completed)
1.  [x] Define a `LogReader` interface in Rekor that abstracts the read operations currently performed by `TrillianClient`.
2.  [x] Refactor existing Rekor API handlers to use this interface instead of directly calling `TrillianClient`.
3.  [x] Create a dummy `TesseraReader` implementation that returns errors to verify the refactoring.

### Phase 2: Tessera Reader Implementation
1.  Implement the `LogReader` interface using the Tessera client library (`reference/tessera/client`).
2.  This implementation will read from a specified storage location (e.g., a directory containing POSIX tiles).
3.  Add configuration options to Rekor to select the Tessera backend and specify the storage path.

### Phase 3: Migration Tooling
1.  Develop a migration tool that connects to a Trillian log, reads all entries, and writes them to Tessera storage in the correct tile layout.
2.  Verify the migrated log by comparing the root hash with the original Trillian log.

## 5. Scalability (1B+ Entries)

For large-scale log like Sigstores Rekor there may be scalability concerns:

*   **Storage Layout**: Tessera's sharded directory structure naturally maps to flat object stores like GCS (using `/` delimiters). It avoids any single directory listing limits and scales horizontally.
*   **Migration Throughput**: Migrating 1 billion entries will take significant time. Parallelization is mandatory. Tessera's migration tools support parallel workers.
*   **Cloud Storage Costs**: Storing 1B+ entries and associated tiles in GCS will require terabytes of storage. Plan for storage costs and potential egress costs if reading from Trillian across clouds.
*   **Trillian Load**: The source Trillian database must be able to handle the sustained read load required to export 1B entries.
*   **Resumability**: The migration tool should support resuming from a specific index to handle failures without restarting the entire process.

## 6. Testing Strategy

To ensure correctness and maintain API compatibility without changes, we will employ the following testing strategies:

### Unit Testing
*   **Data Translation**: Test the mapping between Tessera's raw bytes (entries and proofs) and the Trillian-specific structures expected by Rekor.
*   **Reader Implementation**: Use a mock or a small set of static tiles on the local filesystem to unit test the `TesseraReader` methods without requiring a full running log.

### Integration Testing (End-to-End)
*   **Local E2E Flow**:
    1.  Set up a small Trillian log with a few hundred entries.
    2.  Run the migration tool to create a POSIX Tessera log.
    3.  Start the modified Rekor server pointing to the POSIX log.
    4.  Run a subset of Rekor's existing E2E tests (specifically read paths like `get-log-info`, `get-log-entry`) against the server.

### Differential Testing (Golden Tests)
*   Compare responses from the existing Trillian-backed Rekor and the new Tessera-backed Rekor for the exact same queries (e.g., fetching specific indices or proofs). The JSON responses must match exactly.

### Migration Verification
*   Verify that the root hash computed by Tessera after migration matches the root hash in Trillian for the same tree size.

## 7. Decisions Made
*   **Storage Backend**: We will target **POSIX** first for local testing and initial implementation, but we will need to support **GCS** as well for production readiness (especially for scale).
*   **Key Management**: We will keep using the **same key** as the existing Rekor log for signing the checkpoint in the migrated Tessera log, to avoid breaking trust for existing clients.

## 8. Architectural Notes

### Read-Only Assumption
The current implementation of the Tessera backend in Rekor assumes that the entire log is read-only. It does not support appending new entries (write path). This is suitable for migrating historical data or serving a frozen log.

### Future Extension: Hybrid Backend for Sharded Logs
While the current implementation applies the Tessera backend globally, it could be extended to support a hybrid model. In this model frozen shards of the log could be served from a cost-effective Tessera tile storage and active shard (writable) would continue to be served by Trillian to support high-throughput writes.
