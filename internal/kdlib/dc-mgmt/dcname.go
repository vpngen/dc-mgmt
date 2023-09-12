package dcmgmt

import "os"

func ParseDCNameEnv() (string, string, error) {
	id := os.Getenv("DC_ID")
	if id == "" {
		return "", "", ErrEmptyID
	}

	ident := os.Getenv("DC_NAME")
	if ident == "" {
		return "", "", ErrEmptyIdent
	}

	return id, ident, nil
}
