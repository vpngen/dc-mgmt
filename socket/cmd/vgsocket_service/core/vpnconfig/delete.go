package vpnconfig

import (
	"context"
	"encoding/base32"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"
)

func DeleteUser(ctx context.Context, logger *slog.Logger, opts *core.Options, brigadeID uuid.UUID, userID uuid.UUID) error {
	// 1. get control ip from brigade
	// 2. delete user (update active flag to false)

	controlIP, err := getControlAddr(ctx, logger, opts, brigadeID)
	if err != nil {
		return fmt.Errorf("getting control addr: %w", err)
	}

	if opts.KdTesting {
		if err := DeleteRandomUser(); err != nil {
			return fmt.Errorf("creating random config: %w", err)
		}

		return nil
	}

	if err := callForDel(ctx, logger, opts.AccessKey, brigadeID, controlIP); err != nil {
		return fmt.Errorf("calling for config: %w", err)
	}

	return nil
}

func callForDel(ctx context.Context, logger *slog.Logger, token string,
	brigadeID uuid.UUID, controlIP netip.Addr,
) error {
	c := &http.Client{
		Timeout: 120 * time.Second,
	}

	apiurl := fmt.Sprintf("http://%s/shuffler/%s/configs",
		controlIP.String(),
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID[:]),
	)

	req, err := http.NewRequestWithContext(ctx, "DELETE", apiurl, nil)
	if nil != err {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	for i := 0; i < core.MaxKdCallAttempts; i++ {
		resp, err := c.Do(req)
		if nil != err {
			return fmt.Errorf("failed to do request: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

	}

	logger.Error("max attempts reached", "attempts", core.MaxKdCallAttempts)

	return core.ErrMaxKdCallAttemptsExceeded
}
