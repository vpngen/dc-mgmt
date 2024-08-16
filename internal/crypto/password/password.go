package password

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	// PasswordHashLen - Seed Len in bytes.
	PasswordHashLen = 32

	// PasswordNonce - Password nonce.
	PasswordNonce = "экзистенциальный кризис трансцендентального эго"
)

// Argon2 constants.
const (
	DefaultArgon2Times   = 1
	DefaultArgon2Mem     = 64 * 1024
	DefaultArgon2Threads = 4
	DefaultArgon2SaltLen = 16 // bytes
)

// PasswordHash - generate a password hash using Argon2.
func PasswordHash(password, salt []byte, seedLen uint32) string {
	return hex.EncodeToString(argon2.IDKey(
		password,
		salt,
		DefaultArgon2Times,
		DefaultArgon2Mem,
		DefaultArgon2Threads,
		seedLen,
	))
}

// PasswordSalt - generate a random salt for Argon2.
func PasswordSalt() []byte {
	salt := make([]byte, DefaultArgon2SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic(err) // out of randomness, should never happen
	}

	return salt
}

// NewPasswordHash - generate a salt and a password hash using Argon2 with gnerated salt.
func NewPasswordHash(password []byte, seedLen uint32) (string, []byte) {
	salt := PasswordSalt()

	return PasswordHash(password, salt, seedLen), salt
}
