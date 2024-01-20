package kdlib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	SSHKeyED25519Filename = "id_ed25519"
	SSHDefaultFilename    = SSHKeyED25519Filename
	SSHDefaultTimeOut     = time.Duration(15 * time.Second)
)

var ErrNoSSHKeyFile = errors.New("no ssh key file")

// CreateSSHConfig - creates ssh client config.
func CreateSSHConfig(filename, username string, timeout time.Duration) (*ssh.ClientConfig, error) {
	// var hostKey ssh.PublicKey

	key, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		// HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	return config, nil
}

func LookupForSSHKeyfile(keyFilename, path string) (string, error) {
	if keyFilename != "" {
		return keyFilename, nil
	}

	sysUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("user: %w", err)
	}

	sshKeyDirs := []string{filepath.Join(sysUser.HomeDir, ".ssh"), path}
	for _, dir := range sshKeyDirs {
		if fstat, err := os.Stat(dir); err != nil || !fstat.IsDir() {
			continue
		}

		keyFilename := filepath.Join(dir, SSHKeyED25519Filename)
		if _, err := os.Stat(keyFilename); err == nil {
			return keyFilename, nil
		}
	}

	return "", ErrNoSSHKeyFile
}

func SSHSessionStart(client *ssh.Client, b, e *bytes.Buffer, cmd string, data io.Reader) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}

	defer session.Close()

	session.Stdout = b
	session.Stderr = e

	go func() {
		stdin, err := session.StdinPipe()
		if err != nil {
			// return fmt.Errorf("stdin pipe: %w", err)
			return
		}

		defer stdin.Close()

		if _, err := io.Copy(stdin, data); err != nil {
			// return fmt.Errorf("copy: %w", err)
			return
		}
	}()

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("wait: %w", err)
	}

	return nil
}

func SSHSessionRun(client *ssh.Client, b, e *bytes.Buffer, cmd string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	defer session.Close()

	session.Stdout = b
	session.Stderr = e

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

func NewSSHCient(sshconf *ssh.ClientConfig, server string) (*ssh.Client, *bytes.Buffer, *bytes.Buffer, func(string), error) {
	client, err := ssh.Dial("tcp", server, sshconf)
	if err != nil {
		return nil, nil, nil, func(s string) {}, fmt.Errorf("ssh dial: %w", err)
	}

	var b, e bytes.Buffer

	f := func(logtag string) {
		switch errstr := e.String(); errstr {
		case "":
			fmt.Fprintf(os.Stderr, "%s: SSH Session StdErr: empty\n", logtag)
		default:
			fmt.Fprintf(os.Stderr, "%s: SSH Session StdErr:\n", logtag)
			for _, line := range strings.Split(errstr, "\n") {
				fmt.Fprintf(os.Stderr, "%s: | %s\n", logtag, line)
			}
		}
	}

	return client, &b, &e, f, nil
}
