package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"

	dcmgmt "github.com/vpngen/dc-mgmt"
)

type walkConfig struct {
	db      *pgxpool.Pool
	sshconf *ssh.ClientConfig

	snapFile string
	psk      string
	epsk     string
	stime    int64

	*config
}

// pairsWalk - walk through pairs and collect snapshots.
func pairsWalk(opts *walkConfig) error {
	groups, err := getBrigadesGroups(opts.db, opts.pairsSchema, opts.brigadesSchema, opts.cidrFilter)
	if err != nil {
		return fmt.Errorf("get brigades groups: %w", err)
	}

	data := &dcmgmt.AggrSnaps{
		Version: dcmgmt.AggrSnapsVersion,

		Tag:          opts.tag,
		DatacenterID: opts.dcID,

		GlobalSnapAt: time.Unix(opts.stime, 0).UTC(),

		RealmKeyFP:               opts.realmFP,
		EncryptedPreSharedSecret: opts.epsk,
	}

	if opts.cidrFilter != "" {
		data.Filtered, _ = netip.ParsePrefix(opts.cidrFilter)
	}

	sem := make(chan struct{}, ParallelCollectorsLimit) // Semaphore for limiting parallel collectors.
	var wgg sync.WaitGroup

	stream := make(chan *IncomingSnaps, ParallelCollectorsLimit)
	var wgh sync.WaitGroup

	wgh.Add(1)
	go handleSnapsStream(data, opts.snapFile, stream, &wgh)

	for _, group := range groups {
		sem <- struct{}{} // Acquire the semaphore
		wgg.Add(1)

		collectSnaps(&wgg, stream, sem, &collectConfig{
			sshconf: opts.sshconf,

			addr:     group.ConnectAddr,
			brigades: group.Brigades,

			tag:     opts.tag,
			realmFP: opts.realmFP,
			psk:     opts.psk,
			stime:   opts.stime,

			maintenanceMode: opts.maintenanceMode,
		})
	}

	wgg.Wait() // Wait for all goroutines to finish

	close(stream)

	wgh.Wait() // Wait for all goroutines to finish

	return nil
}

// handleSnapsStream - handle stats stream and update snaps and write to the file.
func handleSnapsStream(data *dcmgmt.AggrSnaps, filename string, stream <-chan *IncomingSnaps, wg *sync.WaitGroup) {
	defer wg.Done()

	for snap := range stream {
		data.Snaps = append(data.Snaps, snap.Snaps...)
	}

	f, err := os.Create(filename + fileTempSuffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: create stats file: %s\n", LogTag, err)

		return
	}

	defer f.Close()

	data.UpdateTime = time.Now().UTC()

	if err := json.NewEncoder(f).Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "%s: encode stats: %s\n", LogTag, err)

		return
	}

	f.Close()

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		if err := os.Remove(filename); err != nil {
			fmt.Fprintf(os.Stderr, "%s: remove stats file: %s\n", LogTag, err)

			return
		}
	}

	if err := os.Link(filename+fileTempSuffix, filename); err != nil {
		fmt.Fprintf(os.Stderr, "%s: link stats file: %s\n", LogTag, err)

		return
	}

	if _, err := os.Stat(filename + fileTempSuffix); !os.IsNotExist(err) {
		if err := os.Remove(filename + fileTempSuffix); err != nil {
			fmt.Fprintf(os.Stderr, "%s: remove temp stats file: %s\n", LogTag, err)

			return
		}
	}
}
