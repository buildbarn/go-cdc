package main

import (
	"archive/tar"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/buildbarn/go-cdc"

	"golang.org/x/sync/errgroup"
)

func main() {
	pr, pw := io.Pipe()
	var g errgroup.Group
	timeZero := time.Unix(0, 0)
	g.Go(func() error {
		tr := tar.NewReader(os.Stdin)
		tw := tar.NewWriter(pw)
		for {
			h, err := tr.Next()
			if err != nil {
				if err == io.EOF {
					pw.Close()
					return nil
				}
				pw.CloseWithError(err)
				return err
			}
			if h.Typeflag == tar.TypeXGlobalHeader {
				continue
			}
			h.ModTime = timeZero
			if err := tw.WriteHeader(h); err != nil {
				pw.CloseWithError(err)
				return err
			}
			if _, err := io.Copy(tw, tr); err != nil {
				pw.CloseWithError(err)
				return err
			}
		}
	})
	g.Go(func() error {
		r := cdc.NewFastContentDefinedChunker(bufio.NewReaderSize(pr, 16*1024*1024), &cdc.FastContentDefinedChunkerGearTable)
		// r := cdc.NewMaxContentDefinedChunker(bufio.NewReaderSize(pr, 16*1024*1024), &cdc.FastContentDefinedChunkerGearTable, 4*1024, 15051)
		// r := cdc.NewRepMaxContentDefinedChunker(bufio.NewReaderSize(pr, 16*1024*1024), &cdc.FastContentDefinedChunkerGearTable, 7608, 8*7608)

		for {
			chunk, err := r.ReadNextChunk()
			if err != nil {
				if err == io.EOF {
					pr.Close()
					return nil
				}
				pr.CloseWithError(err)
				return err
			}
			h := sha256.Sum256(chunk)
			fmt.Printf("%s,%d\n", hex.EncodeToString(h[:]), len(chunk))
		}
	})
	if err := g.Wait(); err != nil {
		log.Fatal(err)
	}
}
