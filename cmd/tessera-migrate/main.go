package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/storage/posix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type dummySigner struct{}

func (d *dummySigner) Name() string                  { return "dummy" }
func (d *dummySigner) KeyHash() uint32               { return 0 }
func (d *dummySigner) Sign(_ []byte) ([]byte, error) { return []byte("signature"), nil }

func main() {
	trillianAddr := flag.String("trillian-addr", "localhost:8090", "Trillian log server gRPC address")
	treeID := flag.Int64("tree-id", 1, "Trillian tree ID")
	tesseraDir := flag.String("tessera-dir", "", "Output directory for Tessera tiles")
	batchSize := flag.Int("batch-size", 1000, "Batch size for fetching leaves")

	flag.Parse()

	if *tesseraDir == "" {
		log.Fatal("-tessera-dir must be specified")
	}

	fmt.Printf("Migrating Trillian tree %d from %s to Tessera dir %s (batch size %d)\n", *treeID, *trillianAddr, *tesseraDir, *batchSize)

	// Connect to Trillian
	conn, err := grpc.NewClient(*trillianAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Trillian: %v", err)
	}

	client := trillian.NewTrillianLogClient(conn)

	ctx := context.Background()

	// Get latest signed log root to know the tree size
	resp, err := client.GetLatestSignedLogRoot(ctx, &trillian.GetLatestSignedLogRootRequest{
		LogId: *treeID,
	})
	if err != nil {
		log.Fatalf("Failed to get latest signed log root: %v", err)
	}

	var root types.LogRootV1
	if err := root.UnmarshalBinary(resp.SignedLogRoot.LogRoot); err != nil {
		log.Fatalf("Failed to unmarshal log root: %v", err)
	}

	treeSize := int64(root.TreeSize)
	fmt.Printf("Source tree size: %d\n", treeSize)

	// Initialize Tessera Appender
	driver, err := posix.New(ctx, posix.Config{Path: *tesseraDir})
	if err != nil {
		log.Fatalf("Failed to create Tessera POSIX driver: %v", err)
	}

	opts := tessera.NewAppendOptions().WithCheckpointSigner(&dummySigner{})

	appender, shutdown, _, err := tessera.NewAppender(ctx, driver, opts)
	if err != nil {
		log.Fatalf("Failed to create Tessera appender: %v", err)
	}

	// Fetch leaves in batches and add to Tessera
	for start := int64(0); start < treeSize; start += int64(*batchSize) {
		count := int64(*batchSize)
		if start+count > treeSize {
			count = treeSize - start
		}

		fmt.Printf("Fetching leaves %d to %d...\n", start, start+count-1)
		req := &trillian.GetLeavesByRangeRequest{
			LogId:      *treeID,
			StartIndex: start,
			Count:      count,
		}

		resp, err := client.GetLeavesByRange(ctx, req)
		if err != nil {
			log.Fatalf("Failed to fetch leaves: %v", err)
		}

		fmt.Printf("Fetched %d leaves, adding to Tessera...\n", len(resp.Leaves))

		for _, leaf := range resp.Leaves {
			ret := appender.Add(ctx, tessera.NewEntry(leaf.LeafValue))
			// Wait for the entry to be sequenced to ensure it's written.
			// This makes the migration synchronous per entry within the batch,
			// which is fine for now.
			if _, err := ret(); err != nil {
				log.Fatalf("Failed to add entry to Tessera: %v", err)
			}
		}
	}

	fmt.Println("All entries added to Tessera appender. Shutting down to flush...")
	if err := shutdown(ctx); err != nil {
		log.Fatalf("Failed to shutdown appender: %v", err)
	}
	conn.Close()
}
