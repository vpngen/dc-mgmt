package main

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/dc-mgmt/internal/snap"
	"golang.org/x/crypto/ssh"
)

type collectConfig struct {
	sshconf *ssh.ClientConfig

	addr     netip.Addr
	brigades [][]byte

	tag     string
	realmFP string
	psk     string
	stime   int64

	maintenanceMode int64
}

// collectSnaps - collect stats from the pair.
func collectSnaps(wg *sync.WaitGroup, stream chan<- *snap.IncomingSnaps, sem <-chan struct{}, opts *collectConfig) {
	defer func() {
		<-sem // Release the semaphore
	}()

	defer wg.Done()

	ids := make([]string, 0, len(opts.brigades))
	for _, id := range opts.brigades {
		ids = append(ids, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:]))
	}

	cleanup, groupStats, err := fetchSnapsBySSH(opts, ids)

	defer cleanup(LogTag)

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: fetch snaps: %s\n", LogTag, err)

		return
	}

	// fmt.Fprintf(os.Stderr, "fetch stats: %s\n", groupStats)

	var parsedStats snap.IncomingSnaps
	if err := json.Unmarshal(groupStats, &parsedStats); err != nil {
		fmt.Fprintf(os.Stderr, "%s: unmarshal snaps: %s\n", LogTag, err)

		return
	}

	if len(opts.brigades) != parsedStats.TotalCount {
		fmt.Fprintf(os.Stderr,
			"%s: brigades count mismatch: %d != %d\n", LogTag,
			len(opts.brigades), parsedStats.TotalCount)

		parsedStats.TotalCount = len(opts.brigades)
	}

	if parsedStats.TotalCount-parsedStats.ErrorsCount != len(parsedStats.Snaps) {
		fmt.Fprintf(os.Stderr,
			"%s: brigades count mismatch: %d != %d\n", LogTag,
			parsedStats.TotalCount-parsedStats.ErrorsCount, len(parsedStats.Snaps))

		parsedStats.ErrorsCount = parsedStats.TotalCount - len(parsedStats.Snaps)
	}

	stream <- &parsedStats
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
