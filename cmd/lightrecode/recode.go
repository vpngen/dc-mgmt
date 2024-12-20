package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/vpngen/keydesk/keydesk/storage"
	"github.com/vpngen/vpngine/naclkey"
	"golang.org/x/crypto/nacl/box"
)

func recodeBrigade(brigade *storage.Brigade, routerPub *[naclkey.NaclBoxKeyLength]byte, masterPrivKey *naclkey.NaclBoxKeypair) error {
	wgPrivateRouterEnc, err := reEncodeNACL(brigade.WgPrivateShufflerEnc, routerPub, masterPrivKey)
	if err != nil {
		return fmt.Errorf("re-encode wg private: %w: %s", err, brigade.BrigadeID)
	}

	brigade.WgPrivateRouterEnc = wgPrivateRouterEnc

	if brigade.OvCAKeyShufflerEnc != "" {
		ovCAKeyShufflerEnc, err := base64.StdEncoding.DecodeString(brigade.OvCAKeyShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode ovca key: %w", err)
		}

		ovCAKeyRouterEnc, err := reEncodeNACL(ovCAKeyShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode ovca key: %w", err)
		}

		brigade.OvCAKeyRouterEnc = base64.StdEncoding.EncodeToString(ovCAKeyRouterEnc)
	}

	if brigade.IPSecPSKShufflerEnc != "" {
		ipSecPSKShufflerEnc, err := base64.StdEncoding.DecodeString(brigade.IPSecPSKShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode ipsec psk: %w", err)
		}

		ipSecPSKRouterEnc, err := reEncodeNACL(ipSecPSKShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode ipsec psk: %w", err)
		}

		brigade.IPSecPSKRouterEnc = base64.StdEncoding.EncodeToString(ipSecPSKRouterEnc)
	}

	for _, user := range brigade.Users {
		if err := recodeUser(user, routerPub, masterPrivKey); err != nil {
			return fmt.Errorf("recode user: %w", err)
		}
	}

	return nil
}

func recodeUser(user *storage.User, routerPub *[naclkey.NaclBoxKeyLength]byte, masterPrivKey *naclkey.NaclBoxKeypair) error {
	wgPSKShufflerEnc, err := reEncodeNACL(user.WgPSKShufflerEnc, routerPub, masterPrivKey)
	if err != nil {
		return fmt.Errorf("re-encode wg psk: %w", err)
	}

	user.WgPSKRouterEnc = wgPSKShufflerEnc

	if user.CloakByPassUIDShufflerEnc != "" {
		cloakByPassUIDShufflerEnc, err := base64.StdEncoding.DecodeString(user.CloakByPassUIDShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode cloak bypass uid: %w", err)
		}

		cloakByPassUIDRouterEnc, err := reEncodeNACL(cloakByPassUIDShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode cloak bypass uid: %w", err)
		}

		user.CloakByPassUIDRouterEnc = base64.StdEncoding.EncodeToString(cloakByPassUIDRouterEnc)
	}

	if user.IPSecUsernameShufflerEnc != "" {
		ipSecUsernameShufflerEnc, err := base64.StdEncoding.DecodeString(user.IPSecUsernameShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode ipsec username: %w", err)
		}

		ipSecUsernameRouterEnc, err := reEncodeNACL(ipSecUsernameShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode ipsec username: %w", err)
		}

		user.IPSecUsernameRouterEnc = base64.StdEncoding.EncodeToString(ipSecUsernameRouterEnc)
	}

	if user.IPSecPasswordShufflerEnc != "" {
		ipSecPasswordShufflerEnc, err := base64.StdEncoding.DecodeString(user.IPSecPasswordShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode ipsec password: %w", err)
		}

		ipSecPasswordRouterEnc, err := reEncodeNACL(ipSecPasswordShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode ipsec password: %w", err)
		}

		user.IPSecPasswordRouterEnc = base64.StdEncoding.EncodeToString(ipSecPasswordRouterEnc)
	}

	if user.OutlineSecretShufflerEnc != "" {
		outlineSecretShufflerEnc, err := base64.StdEncoding.DecodeString(user.OutlineSecretShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode outline secret: %w", err)
		}

		outlineSecretRouterEnc, err := reEncodeNACL(outlineSecretShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode outline secret: %w", err)
		}

		user.OutlineSecretRouterEnc = base64.StdEncoding.EncodeToString(outlineSecretRouterEnc)
	}

	if user.Proto0SecretShufflerEnc != "" {
		proto0SecretShufflerEnc, err := base64.StdEncoding.DecodeString(user.Proto0SecretShufflerEnc)
		if err != nil {
			return fmt.Errorf("decode proto0 secret: %w", err)
		}

		proto0SecretRouterEnc, err := reEncodeNACL(proto0SecretShufflerEnc, routerPub, masterPrivKey)
		if err != nil {
			return fmt.Errorf("re-encode proto0 secret: %w", err)
		}

		user.Proto0SecretRouterEnc = base64.StdEncoding.EncodeToString(proto0SecretRouterEnc)
	}

	return nil
}

var ErrDecryptionFailed = fmt.Errorf("decryption failed")

func reEncodeNACL(payload []byte, routerPub *[naclkey.NaclBoxKeyLength]byte, masterPriv *naclkey.NaclBoxKeypair) ([]byte, error) {
	decrypted, ok := box.OpenAnonymous(nil, payload, &masterPriv.Public, &masterPriv.Private)
	if !ok {
		return nil, ErrDecryptionFailed
	}

	// re-encode
	routerReEncrypted, err := box.SealAnonymous(nil, decrypted, routerPub, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("router seal: %w", err)
	}

	return routerReEncrypted, nil
}
