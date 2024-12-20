package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/vpngen/keydesk/keydesk/storage"
)

func readBrigadefile(path string) (*storage.Brigade, error) {
	var r io.Reader

	switch path {
	case "-":
		r = os.Stdin
	default:
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open: %w", err)
		}

		defer f.Close()

		r = f
	}

	data := &storage.Brigade{}

	if err := json.NewDecoder(r).Decode(data); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return data, nil
}

func writeBrigadeFile(path string, data *storage.Brigade) error {
	var w io.Writer

	switch path {
	case "-":
		w = os.Stdout
	default:
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}

		defer f.Close()

		w = f
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	return nil
}
