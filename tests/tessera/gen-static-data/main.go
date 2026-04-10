package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/transparency-dev/merkle/rfc6962"
	"github.com/transparency-dev/tessera/api/layout"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <output_dir>")
		os.Exit(1)
	}
	outDir := os.Args[1]

	pemPub := []byte(`-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIxaAc+TaxYoQU0X9NNUJgWffbn6h
juoEDPQQn80nX/Eus9I/t00ccNpcSrUw1+IPyGp1p9fgmL0DlBf05BNFNQ==
-----END PUBLIC KEY-----
`)
	pubEncoded := base64.StdEncoding.EncodeToString(pemPub)

	leaf0 := []byte(fmt.Sprintf(`{"apiVersion":"0.0.1","kind":"hashedrekord","spec":{"data":{"hash":{"algorithm":"sha256","value":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}},"signature":{"content":"YmFzZTY0IHNpZ25hdHVyZQ==","publicKey":{"content":"%s"}}}}`, pubEncoded))
	leaf1 := []byte(fmt.Sprintf(`{"apiVersion":"0.0.1","kind":"hashedrekord","spec":{"data":{"hash":{"algorithm":"sha256","value":"b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"}},"signature":{"content":"YmFzZTY0IHNpZ25hdHVyZQ==","publicKey":{"content":"%s"}}}}`, pubEncoded))

	h0 := rfc6962.DefaultHasher.HashLeaf(leaf0)
	h1 := rfc6962.DefaultHasher.HashLeaf(leaf1)

	rootHash := rfc6962.DefaultHasher.HashChildren(h0, h1)

	// Write checkpoint
	checkpointContent := fmt.Sprintf("example.com\n2\n%s\n", base64.StdEncoding.EncodeToString(rootHash))
	err := os.WriteFile(filepath.Join(outDir, "checkpoint"), []byte(checkpointContent), 0644) // nolint:gosec
	if err != nil {
		panic(err)
	}

	// Write entry bundle 0 with 2 entries
	bundleBuf := &bytes.Buffer{}
	_ = binary.Write(bundleBuf, binary.BigEndian, uint16(len(leaf0)))
	bundleBuf.Write(leaf0)
	_ = binary.Write(bundleBuf, binary.BigEndian, uint16(len(leaf1)))
	bundleBuf.Write(leaf1)

	bundlePath := filepath.Join(outDir, layout.EntriesPath(0, 2))
	err = os.MkdirAll(filepath.Dir(bundlePath), 0755) // nolint:gosec
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(bundlePath, bundleBuf.Bytes(), 0644) // nolint:gosec
	if err != nil {
		panic(err)
	}

	// Write tile 0,0 with 2 leaf hashes
	tilePath := filepath.Join(outDir, layout.TilePath(0, 0, 2))
	err = os.MkdirAll(filepath.Dir(tilePath), 0755) // nolint:gosec
	if err != nil {
		panic(err)
	}
	tileBuf := append([]byte(nil), h0...)
	tileBuf = append(tileBuf, h1...)
	err = os.WriteFile(tilePath, tileBuf, 0644) // nolint:gosec
	if err != nil {
		panic(err)
	}

	fmt.Println("Static Tessera data generated in", outDir)
}
