package snap

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	dcmgmt "github.com/vpngen/dc-mgmt"
)

// HandleSnapsStream - handle stats stream and update snaps and write to the file.
func HandleSnapsStream(logTag string, data *dcmgmt.AggrSnaps, filename string, stream <-chan *IncomingSnaps, wg *sync.WaitGroup) {
	defer wg.Done()

	for snap := range stream {
		data.Snaps = append(data.Snaps, snap.Snaps...)
		data.TotalCount += snap.TotalCount
		data.ErrorsCount += snap.ErrorsCount
	}

	f, err := os.Create(filename + fileTempSuffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: create stats file: %s\n", logTag, err)

		return
	}

	defer f.Close()

	data.UpdateTime = time.Now().UTC()

	if err := json.NewEncoder(f).Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "%s: encode stats: %s\n", logTag, err)

		return
	}

	f.Close()

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		if err := os.Remove(filename); err != nil {
			fmt.Fprintf(os.Stderr, "%s: remove stats file: %s\n", logTag, err)

			return
		}
	}

	if err := os.Link(filename+fileTempSuffix, filename); err != nil {
		fmt.Fprintf(os.Stderr, "%s: link stats file: %s\n", logTag, err)

		return
	}

	if _, err := os.Stat(filename + fileTempSuffix); !os.IsNotExist(err) {
		if err := os.Remove(filename + fileTempSuffix); err != nil {
			fmt.Fprintf(os.Stderr, "%s: remove temp stats file: %s\n", logTag, err)

			return
		}
	}
}
