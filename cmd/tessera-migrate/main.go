package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/sigstore/rekor/pkg/util"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/sigstore/sigstore/pkg/signature/options"
	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/storage/posix"
	"go.step.sm/crypto/pemutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type realSigner struct {
	name   string
	signer signature.Signer
}

func (s *realSigner) Name() string { return s.name }
func (s *realSigner) KeyHash() uint32 {
	pk, err := s.signer.PublicKey()
	if err != nil {
		return 0
	}
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pk)
	if err != nil {
		return 0
	}
	pkSha := sha256.Sum256(pubKeyBytes)
	return binary.BigEndian.Uint32(pkSha[:])
}
func (s *realSigner) Sign(msg []byte) ([]byte, error) {
	return s.signer.SignMessage(bytes.NewReader(msg))
}

func main() {
	trillianAddr := flag.String("trillian-addr", "localhost:8090", "Trillian log server gRPC address")
	treeID := flag.Int64("tree-id", 1, "Trillian tree ID")
	tesseraDir := flag.String("tessera-dir", "", "Output directory for Tessera tiles")
	batchSize := flag.Int("batch-size", 1000, "Batch size for fetching leaves")
	origin := flag.String("origin", "dummy", "Origin name for the checkpoint")
	signerKey := flag.String("signer", "", "Path to PEM-encoded private key file")

	flag.Parse()

	if *tesseraDir == "" {
		log.Fatal("-tessera-dir must be specified")
	}

	if err := run(*trillianAddr, *treeID, *tesseraDir, *batchSize, *origin, *signerKey); err != nil {
		log.Fatal(err)
	}
}

func run(trillianAddr string, treeID int64, tesseraDir string, batchSize int, origin string, signerKey string) error {
	fmt.Printf("Migrating Trillian tree %d from %s to Tessera dir %s (batch size %d)\n", treeID, trillianAddr, tesseraDir, batchSize)

	// Connect to Trillian
	conn, err := grpc.NewClient(trillianAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to Trillian: %w", err)
	}
	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)

	ctx := context.Background()

	// Initialize Tessera Appender
	driver, err := posix.New(ctx, posix.Config{Path: tesseraDir})
	if err != nil {
		return fmt.Errorf("failed to create Tessera POSIX driver: %w", err)
	}

	opaqueKey, err := pemutil.Read(signerKey)
	if err != nil {
		return fmt.Errorf("failed to read signer key: %w", err)
	}

	sig, err := signature.LoadSignerVerifier(opaqueKey, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("failed to create signer: %w", err)
	}

	// Workaround: Tessera's use of note.Sign requires Ed25519. The rekor-tiles
	// signer is not usable because it does checks on origin name. We could
	// write our own but for now use an ephemeral Ed25519 key for Tessera and
	// fix up the final checkpoint later.
	_, edPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	edSigner, err := signature.LoadED25519Signer(edPriv)
	if err != nil {
		return fmt.Errorf("failed to create ephemeral signer: %w", err)
	}

	tesseraOrigin := "tessera-migration"
	ts := &realSigner{name: tesseraOrigin, signer: edSigner}

	opts := tessera.NewAppendOptions().WithCheckpointSigner(ts)

	appender, shutdown, _, err := tessera.NewAppender(ctx, driver, opts)
	if err != nil {
		return fmt.Errorf("failed to create Tessera appender: %w", err)
	}

	if err := runMigration(ctx, client, treeID, appender, batchSize); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println("All entries added to Tessera appender. Shutting down to flush...")
	startShutdown := time.Now()
	if err := shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown appender: %w", err)
	}
	fmt.Printf("Shutdown and flushed in %v\n", time.Since(startShutdown))

	// Fix up the final checkpoint with the real key and origin (supporting spaces)
	fmt.Println("Fixing up final checkpoint signature...")
	checkpointPath := filepath.Join(tesseraDir, "checkpoint")
	cpBytes, err := os.ReadFile(checkpointPath)
	if err != nil {
		return fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	sn := util.SignedNote{}
	if err := sn.UnmarshalText(cpBytes); err != nil {
		return fmt.Errorf("failed to parse checkpoint: %w", err)
	}

	// Tessera wrote the checkpoint with "tessera-migration" as origin.
	// We need to update it to the real origin (which may contain spaces).
	c := util.Checkpoint{}
	if err := c.UnmarshalCheckpoint([]byte(sn.Note)); err != nil {
		return fmt.Errorf("failed to parse checkpoint note: %w", err)
	}
	c.Origin = origin

	noteBytes, err := c.MarshalCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint note: %w", err)
	}
	sn.Note = string(noteBytes)

	// Clear ephemeral signatures
	sn.Signatures = nil

	// Sign with the real key and original origin
	_, err = sn.Sign(origin, sig, options.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to sign checkpoint: %w", err)
	}

	finalBytes, err := sn.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(checkpointPath, finalBytes, 0600); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}
	fmt.Println("Checkpoint fixed up successfully!")
	return nil
}

func runMigration(ctx context.Context, client trillian.TrillianLogClient, treeID int64, appender *tessera.Appender, batchSize int) error {
	// Get latest signed log root to know the tree size
	resp, err := client.GetLatestSignedLogRoot(ctx, &trillian.GetLatestSignedLogRootRequest{
		LogId: treeID,
	})
	if err != nil {
		return fmt.Errorf("failed to get latest signed log root: %w", err)
	}

	var root types.LogRootV1
	if err := root.UnmarshalBinary(resp.SignedLogRoot.LogRoot); err != nil {
		return fmt.Errorf("failed to unmarshal log root: %w", err)
	}

	treeSize := int64(root.TreeSize)
	fmt.Printf("Source tree size: %d\n", treeSize)

	// Fetch leaves in batches and add to Tessera
	for start := int64(0); start < treeSize; start += int64(batchSize) {
		count := int64(batchSize)
		if start+count > treeSize {
			count = treeSize - start
		}

		fmt.Printf("Fetching leaves %d to %d...\n", start, start+count-1)
		req := &trillian.GetLeavesByRangeRequest{
			LogId:      treeID,
			StartIndex: start,
			Count:      count,
		}

		startFetch := time.Now()
		resp, err := client.GetLeavesByRange(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to fetch leaves: %w", err)
		}
		fmt.Printf("Fetched %d leaves in %v, adding to Tessera...\n", len(resp.Leaves), time.Since(startFetch))

		startAdd := time.Now()
		rets := make([]tessera.IndexFuture, 0, len(resp.Leaves))
		for _, leaf := range resp.Leaves {
			rets = append(rets, appender.Add(ctx, tessera.NewEntry(leaf.LeafValue)))
		}

		for _, ret := range rets {
			if _, err := ret(); err != nil {
				return fmt.Errorf("failed to add entry to Tessera: %w", err)
			}
		}
		fmt.Printf("Added %d leaves to Tessera in %v\n", len(resp.Leaves), time.Since(startAdd))
	}
	return nil
}
