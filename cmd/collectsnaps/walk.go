package main

import (
	"fmt"
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
	var (
		err    error
		groups GroupsList
	)

	switch opts.plan {
	case nil:
		groups, err = getBrigadesGroups(opts.db, opts.extFilter, opts.ctrlFilter)
		if err != nil {
			return fmt.Errorf("get brigades groups: %w", err)
		}
	default:
		groups, err = getBrigadesGroupsFromPlan(opts.db, opts.plan)
		if err != nil {
			return fmt.Errorf("get brigades groups from plan: %w", err)
		}
	}

	data := &dcmgmt.AggrSnaps{
		Version: dcmgmt.AggrSnapsVersion,

		Tag:          opts.tag,
		DatacenterID: opts.dcID,

		GlobalSnapAt: time.Unix(opts.stime, 0).UTC(),

		RealmKeyFP:               opts.realmFP,
		EncryptedPreSharedSecret: opts.epsk,
	}

	data.ExternalIPFiltered = opts.extFilter
	data.ControlNodeFiltered = opts.ctrlFilter

	sem := make(chan struct{}, ParallelCollectorsLimit) // Semaphore for limiting parallel collectors.
	var wgg sync.WaitGroup

	stream := make(chan *dcmgmt.InstancedSnaps, ParallelCollectorsLimit)
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
