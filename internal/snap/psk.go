package snap

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"

	"github.com/vpngen/keydesk-snap/core/crypto"
)

// GenPSK - generate psk and encrypt it.
func GenPSK(key *rsa.PublicKey) (string, string, error) {
	psk, err := crypto.GenSecret(PSKLen)
	if err != nil {
		return "", "", fmt.Errorf("generate secret: %w", err)
	}

	epsk, err := crypto.EncryptSecret(key, psk)
	if err != nil {
		return "", "", fmt.Errorf("encrypt psk: %w", err)
	}

	return base64.StdEncoding.EncodeToString(psk),
		base64.StdEncoding.EncodeToString(epsk),
		nil
}
