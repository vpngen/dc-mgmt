package brigade

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/strfmt/conv"
	"github.com/go-openapi/swag"
	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/models"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"
)

func GetActivityInfo(ctx context.Context, logger *slog.Logger, opts *core.Options,
	brigadeID uuid.UUID,
) (models.BrigadeActivity, error) {
	// 1. get control ip from brigade
	// 2. call control node for activity
	// 3. return activity info

	controlIP, err := core.GetControlAddr(ctx, logger, opts.Db, opts.SqFmt, brigadeID)
	if err != nil {
		return nil, fmt.Errorf("getting control addr: %w", err)
	}

	//if opts.KdTesting {
	//conf, _, err := CreateRandomConfig(ctx, brigadeID, configType)
	//if err != nil {
	//	return nil, fmt.Errorf("creating random config: %w", err)
	//}

	// return conf, nil
	//}

	act, err := callForActivity(ctx, logger, opts.AccessKey, brigadeID, controlIP)
	if err != nil {
		return nil, fmt.Errorf("calling for brigade activity: %w", err)
	}

	return act, nil
}

func callForActivity(ctx context.Context, logger *slog.Logger, token string,
	brigadeID uuid.UUID, controlIP netip.Addr,
) (models.BrigadeActivity, error) {
	c := &http.Client{
		Timeout: 120 * time.Second,
	}

	apiurl := fmt.Sprintf("http://%s/shuffler/%s/activity",
		controlIP.String(),
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID[:]),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiurl, nil)
	if nil != err {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	for i := 0; i < core.MaxKdCallAttempts; i++ {
		resp, err := c.Do(req)
		if nil != err {
			return nil, fmt.Errorf("failed to do request: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Debug("unexpected status code", "status_code", resp.StatusCode)

			continue
		}

		acts := make(BrigadeActivity)
		if err := json.NewDecoder(resp.Body).Decode(&acts); nil != err {
			logger.Debug("failed to decode response", "error", err)

			continue
		}

		return toModel(acts), nil
	}

	logger.Error("max attempts reached", "attempts", core.MaxKdCallAttempts)

	return nil, core.ErrMaxKdCallAttemptsExceeded
}

func toModel(acts BrigadeActivity) models.BrigadeActivity {
	model := make(models.BrigadeActivity)

	for k, v := range acts {
		model[k] = models.AcitivityData{
			LastSeen: conv.DateTime(strfmt.DateTime(v.LastSeen)),

			TotalTraffic:   swag.Int64(v.TotalTraffic),
			MonthlyTraffic: swag.Int64(v.MonthlyTraffic),
			PrevDayTraffic: swag.Int64(v.PrevDayTraffic),

			Updated: conv.DateTime(strfmt.DateTime(v.Updated)),
		}
	}

	return model
}
