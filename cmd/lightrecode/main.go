package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	opts, err := conf()
	if err != nil {
		logger.Error("read conf", "error", err)

		os.Exit(1)
	}

	if err := recode(opts); err != nil {
		logger.Error("recode", "error", err)

		os.Exit(1)
	}
}

func recode(o *opts) error {
	data, err := readBrigadefile(o.infile)
	if err != nil {
		return fmt.Errorf("read brigade: %w", err)
	}

	if err := recodeBrigade(data, &o.routerKey, &o.masterPrivKey); err != nil {
		return fmt.Errorf("recode data: %w", err)
	}

	if err := writeBrigadeFile(o.outfile, data); err != nil {
		return fmt.Errorf("write brigade: %w", err)
	}

	return nil
}
