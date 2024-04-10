package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	dcmgmt "github.com/vpngen/dc-mgmt"
	"github.com/vpngen/dc-mgmt/internal/snap"
)

type collectConfig struct {
	tag     string
	realmFP string
	psk     string
	stime   int64

	dbdir   string
	confdir string

	ids []string
}

// collectSnaps - collect stats from the pair.
func collectSnaps(stream chan<- *dcmgmt.InstancedSnaps, opts *collectConfig) {
	groupStats, err := fetchSnapsByScript(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: fetch snaps: %s\n", LogTag, err)

		return
	}

	// fmt.Fprintf(os.Stderr, "fetch stats: %s\n", groupStats)

	var parsedSnaps snap.IncomingSnaps
	if err := json.Unmarshal(groupStats, &parsedSnaps); err != nil {
		fmt.Fprintf(os.Stderr, "%s: unmarshal snaps: %s\n", LogTag, err)

		return
	}

	instancedSnaps := &dcmgmt.InstancedSnaps{
		Snaps:       make([]*dcmgmt.EncryptedBrigade, 0, len(opts.ids)),
		TotalCount:  parsedSnaps.TotalCount,
		ErrorsCount: parsedSnaps.ErrorsCount,
	}

	for _, snap := range parsedSnaps.Snaps {
		instancedSnaps.Snaps = append(instancedSnaps.Snaps, &dcmgmt.EncryptedBrigade{
			EncryptedBrigade: *snap,
			InstanceID:       uuid.New().String(),
		})
	}

	stream <- instancedSnaps
}

// fetchSnapsByScript - fetch brigades stats from remote host by ssh.
func fetchSnapsByScript(opts *collectConfig) ([]byte, error) {
	cmdstr := fmt.Sprintf(
		"../../../../vpngen-keydesk-snap/cmd/fetchsnaps/fetchsnaps.sh -tag %s -list %s -rfp %s -stime %d -d %s -c %s",
		opts.tag,
		strings.Join(opts.ids, ","),
		opts.realmFP,
		opts.stime,
		opts.dbdir,
		opts.confdir,
	)

	fmt.Fprintf(os.Stderr, "%s\n", cmdstr)

	cmd := exec.Command("/bin/sh", "-c", cmdstr)

	// Assign the buffer to the command's Stdin
	cmd.Stdin = strings.NewReader(opts.psk)

	// Create buffers to capture standard output and standard error
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Failed to execute command: %s\n", err)
	}

	// Print the output of the command
	fmt.Fprintf(os.Stderr, "stderr: %s\n", stderr.String())

	return stdout.Bytes(), nil
}
