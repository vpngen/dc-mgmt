package main

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"

	dcmgmt "github.com/vpngen/dc-mgmt"
	"github.com/vpngen/dc-mgmt/internal/snap"
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

	stream := make(chan *snap.IncomingSnaps, ParallelCollectorsLimit)
	var wgh sync.WaitGroup

	wgh.Add(1)
	go snap.HandleSnapsStream(LogTag, data, opts.snapFile, stream, &wgh)

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
