package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/storage/posix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type dummySigner struct {
	name string
}

func (d *dummySigner) Name() string                  { return d.name }
func (d *dummySigner) KeyHash() uint32               { return 0 }
func (d *dummySigner) Sign(_ []byte) ([]byte, error) { return []byte("signature"), nil }

func main() {
	trillianAddr := flag.String("trillian-addr", "localhost:8090", "Trillian log server gRPC address")
	treeID := flag.Int64("tree-id", 1, "Trillian tree ID")
	tesseraDir := flag.String("tessera-dir", "", "Output directory for Tessera tiles")
	batchSize := flag.Int("batch-size", 1000, "Batch size for fetching leaves")
	origin := flag.String("origin", "dummy", "Origin name for the checkpoint")

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
	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)

	ctx := context.Background()

	// Initialize Tessera Appender
	driver, err := posix.New(ctx, posix.Config{Path: *tesseraDir})
	if err != nil {
		log.Fatalf("Failed to create Tessera POSIX driver: %v", err)
	}

	opts := tessera.NewAppendOptions().WithCheckpointSigner(&dummySigner{name: *origin})

	appender, shutdown, _, err := tessera.NewAppender(ctx, driver, opts)
	if err != nil {
		log.Fatalf("Failed to create Tessera appender: %v", err)
	}

	if err := runMigration(ctx, client, *treeID, appender, *batchSize); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Println("All entries added to Tessera appender. Shutting down to flush...")
	startShutdown := time.Now()
	if err := shutdown(ctx); err != nil {
		log.Fatalf("Failed to shutdown appender: %v", err)
	}
	fmt.Printf("Shutdown and flushed in %v\n", time.Since(startShutdown))
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
