package main

import (
	"sync"
	"time"

	dcmgmt "github.com/vpngen/dc-mgmt"
	"github.com/vpngen/dc-mgmt/internal/snap"
)

type walkConfig struct {
	snapFile string
	psk      string
	epsk     string
	stime    int64

	*config
}

// pairsWalk - walk through pairs and collect snapshots.
func pairsWalk(opts *walkConfig) error {
	data := &dcmgmt.AggrSnaps{
		Version: dcmgmt.AggrSnapsVersion,

		Tag:          opts.tag,
		DatacenterID: opts.dcID,

		GlobalSnapAt: time.Unix(opts.stime, 0).UTC(),

		RealmKeyFP:               opts.realmFP,
		EncryptedPreSharedSecret: opts.epsk,
	}

	stream := make(chan *snap.IncomingSnaps, 1)
	var wgh sync.WaitGroup

	wgh.Add(1)
	go snap.HandleSnapsStream(LogTag, data, opts.snapFile, stream, &wgh)

	collectSnaps(stream, &collectConfig{
		tag:     opts.tag,
		realmFP: opts.realmFP,
		psk:     opts.psk,
		stime:   opts.stime,

		dbdir:   opts.storageDir,
		confdir: opts.realmsKeysPath,

		ids: []string{opts.config.BrigadeID},
	})

	close(stream)

	wgh.Wait() // Wait for all goroutines to finish

	return nil
}
