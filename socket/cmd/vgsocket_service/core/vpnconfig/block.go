package vpnconfig

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"
)

func BlockUnblockUser(ctx context.Context, logger *slog.Logger, opts *core.Options, brigadeID uuid.UUID, userID uuid.UUID, block bool) (int, error) {
	// 1. get control ip from brigade
	// 2. delete user (update active flag to false)

	controlIP, err := core.GetControlAddr(ctx, logger, opts.Db, opts.SqFmt, brigadeID)
	if err != nil {
		return 0, fmt.Errorf("getting control addr: %w", err)
	}

	if opts.KdTesting {
		return 100, nil
	}

	slots, err := callForBlockUnblock(ctx, logger, opts.AccessKey, brigadeID, controlIP, userID, block)
	if err != nil {
		return 0, fmt.Errorf("calling for block config: %w", err)
	}

	return slots, nil
}

func callForBlockUnblock(ctx context.Context, logger *slog.Logger, token string,
	brigadeID uuid.UUID, controlIP netip.Addr, userID uuid.UUID, block bool,
) (int, error) {
	c := &http.Client{
		Timeout: 120 * time.Second,
	}

	apiurl := fmt.Sprintf("http://%s/shuffler/%s/configs/%s",
		controlIP.String(),
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID[:]),
		userID.String(),
	)

	switch block {
	case true:
		apiurl += "/block"
	case false:
		apiurl += "/unblock"
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", apiurl, nil)
	if nil != err {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	for i := 0; i < core.MaxKdCallAttempts; i++ {
		resp, err := c.Do(req)
		if nil != err {
			logger.Error("failed to do request", "error", err)

			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
			return 0, core.ErrUserNotFound
		}

		if resp.StatusCode != http.StatusOK {
			logger.Error("unexpected status code", "status_code", resp.StatusCode)

			continue
		}

		slots := &SocketDeleteUser{}
		if err := json.NewDecoder(resp.Body).Decode(slots); nil != err {
			logger.Debug("failed to decode response", "error", err)

			continue
		}

		return slots.FreeSlots, nil
	}

	logger.Error("max attempts reached", "attempts", core.MaxKdCallAttempts)

	return 0, core.ErrMaxKdCallAttemptsExceeded
}
