package main

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/dc-mgmt/internal/snap"
	"golang.org/x/crypto/ssh"

	dcmgmt "github.com/vpngen/dc-mgmt"
)

type collectConfig struct {
	sshconf *ssh.ClientConfig

	addr     netip.Addr
	brigades map[uuid.UUID]uuid.UUID

	tag     string
	realmFP string
	psk     string
	stime   int64

	maintenanceMode int64
}

const (
	connectAttempts = 3
	connectSleep    = 2 * time.Second
)

// collectSnaps - collect stats from the pair.
func collectSnaps(wg *sync.WaitGroup, stream chan<- *dcmgmt.InstancedSnaps, sem <-chan struct{}, opts *collectConfig) {
	defer func() {
		<-sem // Release the semaphore
	}()

	defer wg.Done()

	ids := make([]string, 0, len(opts.brigades))
	for id := range opts.brigades {
		ids = append(ids, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:]))
	}

	var (
		err        error
		groupStats []byte
	)

	for i := 0; i < connectAttempts; i++ {
		func() {
			var cleanup func(string)

			cleanup, groupStats, err = fetchSnapsBySSH(opts, ids)

			defer cleanup(LogTag + "|" + opts.addr.String())
		}()

		if err == nil {
			break
		}
	}

	instancedSnaps := &dcmgmt.InstancedSnaps{
		Snaps: make([]*dcmgmt.EncryptedBrigade, 0, len(opts.brigades)),
	}

	defer func() {
		stream <- instancedSnaps
	}()

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: [%s]: fetch snaps: %s\n", LogTag, opts.addr, err)

		instancedSnaps.TotalCount = len(opts.brigades)
		instancedSnaps.ErrorsCount = instancedSnaps.TotalCount

		return
	}

	// fmt.Fprintf(os.Stderr, "fetch stats: %s\n", groupStats)

	var parsedSnaps snap.IncomingSnaps

	if err := json.Unmarshal(groupStats, &parsedSnaps); err != nil {
		fmt.Fprintf(os.Stderr, "%s: [%s]: unmarshal snaps: %s\n", LogTag, opts.addr, err)

		instancedSnaps.TotalCount = len(opts.brigades)
		instancedSnaps.ErrorsCount = parsedSnaps.TotalCount

		return
	}

	if len(opts.brigades) != parsedSnaps.TotalCount {
		fmt.Fprintf(os.Stderr,
			"%s: [%s]: brigades count mismatch: %d != %d\n", LogTag, opts.addr,
			len(opts.brigades), parsedSnaps.TotalCount)

		instancedSnaps.TotalCount = len(opts.brigades)
	}

	if parsedSnaps.TotalCount-parsedSnaps.ErrorsCount != len(parsedSnaps.Snaps) {
		fmt.Fprintf(os.Stderr,
			"%s: [%s]: brigades count mismatch: %d != %d\n", LogTag, opts.addr,
			parsedSnaps.TotalCount-parsedSnaps.ErrorsCount, len(parsedSnaps.Snaps))

		instancedSnaps.ErrorsCount = parsedSnaps.TotalCount - len(parsedSnaps.Snaps)
	}

	for _, e := range parsedSnaps.Snaps {
		brigadeID, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(e.BrigadeID)
		if err != nil || len(brigadeID) != 16 {
			fmt.Fprintf(os.Stderr, "%s: decode brigade id: %s\n", LogTag, err)

			continue
		}

		instanceID := opts.brigades[uuid.UUID(brigadeID)]
		if instanceID == uuid.Nil {
			fmt.Fprintf(os.Stderr, "%s: instance id not found: %s\n", LogTag, e.BrigadeID)

			continue
		}

		instancedSnaps.Snaps = append(instancedSnaps.Snaps, &dcmgmt.EncryptedBrigade{
			EncryptedBrigade: *e,
			InstanceID:       instanceID.String(),
		})
	}
}

// fetchSnapsBySSH - fetch brigades stats from remote host by ssh.
func fetchSnapsBySSH(opts *collectConfig, ids []string) (func(string), []byte, error) {
	cmd := fmt.Sprintf(
		"fetchsnaps -tag %s -list %s -rfp %s -stime %d -mnt %d",
		opts.tag,
		strings.Join(ids, ","),
		opts.realmFP,
		opts.stime,
		opts.maintenanceMode,
	)

	fmt.Fprintf(os.Stderr, "%s#%s:22 -> %s\n", sshkeyRemoteUsername, opts.addr, cmd)

	client, b, e, cleanup, err := kdlib.NewSSHCient(opts.sshconf, opts.addr.String()+":22")
	if err != nil {
		return cleanup, nil, fmt.Errorf("new ssh client: %w", err)
	}

	defer client.Close()

	if err := kdlib.SSHSessionStart(client, b, e, cmd, strings.NewReader(opts.psk)); err != nil {
		return cleanup, nil, fmt.Errorf("write remote file: %w", err)
	}

	return cleanup, b.Bytes(), nil
}
