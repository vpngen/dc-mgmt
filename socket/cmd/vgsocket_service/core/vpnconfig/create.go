package vpnconfig

import (
	"bytes"
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

	sq "github.com/Masterminds/squirrel"
)

func GreateUser(ctx context.Context, logger *slog.Logger, opts *core.Options,
	brigadeID uuid.UUID, configType string,
) (*models.VPNConfig, error) {
	// 1. get control ip from brigade
	// 2. call control node for config
	// 3. return vpn config

	controlIP, err := getControlAddr(ctx, logger, opts, brigadeID)
	if err != nil {
		return nil, fmt.Errorf("getting control addr: %w", err)
	}

	if opts.KdTesting {
		conf, _, err := CreateRandomConfig(ctx, brigadeID, configType)
		if err != nil {
			return nil, fmt.Errorf("creating random config: %w", err)
		}

		return conf, nil
	}

	conf, _, err := callForConfig(ctx, logger, opts.AccessKey, brigadeID, controlIP, configType)
	if err != nil {
		return nil, fmt.Errorf("calling for config: %w", err)
	}

	return conf, nil
}

func getControlAddr(ctx context.Context, _ *slog.Logger, opts *core.Options, brigadeID uuid.UUID) (netip.Addr, error) {
	// 1. get control ip from brigade
	// 2. return control ip

	tx, err := opts.Db.Begin(ctx)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("beginning transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	query := opts.SqFmt.Select("p.control_ip").
		From("brigades.brigades b").
		Join("pairs.pairs p ON b.pair_id = p.pair_id").
		Where(sq.Eq{"b.brigade_id": brigadeID})

	sql, args, err := query.ToSql()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("building query: %w", err)
	}

	var controlIP netip.Addr

	if err := tx.QueryRow(ctx, sql, args...).Scan(&controlIP); err != nil {
		return netip.Addr{}, fmt.Errorf("querying control ip: %w", err)
	}

	return controlIP, nil
}

type ConfigRequest struct {
	Configs []string `json:"configs"`
}

func callForConfig(ctx context.Context, logger *slog.Logger, token string,
	brigadeID uuid.UUID, controlIP netip.Addr, configType string,
) (*models.VPNConfig, string, error) {
	data, err := json.Marshal(&ConfigRequest{Configs: []string{configType}})
	if err != nil {
		return nil, "", fmt.Errorf("marshaling request: %w", err)
	}

	c := &http.Client{
		Timeout: 120 * time.Second,
	}

	apiurl := fmt.Sprintf("http://%s/shuffler/%s/configs",
		controlIP.String(),
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID[:]),
	)

	req, err := http.NewRequestWithContext(ctx, "POST", apiurl, bytes.NewBuffer(data))
	if nil != err {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	for i := 0; i < core.MaxKdCallAttempts; i++ {
		resp, err := c.Do(req)
		if nil != err {
			return nil, "", fmt.Errorf("failed to do request: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			logger.Debug("unexpected status code", "status_code", resp.StatusCode)

			continue
		}

		user := &SocketNewUser{}
		if err := json.NewDecoder(resp.Body).Decode(user); nil != err {
			logger.Debug("failed to decode response", "error", err)

			continue
		}

		m, name, err := kmodelToModel(user)
		if err != nil {
			logger.Debug("failed to convert model", "error", err)

			continue
		}

		logger.Debug("config created", "config_id", m.UserID.String(), "config_name", name)

		return m, name, nil
	}

	logger.Error("max attempts reached", "attempts", core.MaxKdCallAttempts)

	return nil, "", core.ErrMaxKdCallAttemptsExceeded
}

func kmodelToModel(nu *SocketNewUser) (*models.VPNConfig, string, error) {
	m := &models.VPNConfig{
		UserID: conv.UUID4(strfmt.UUID4(nu.ID.String())),
		Name:   swag.String(nu.Name),
		Domain: swag.String(nu.Domain),
	}

	if nu.Configs.Wireguard != nil {
		m.WireGuardConfig = &models.WireGuardConfig{
			FileName:    &nu.Configs.Wireguard.FileName,
			TunnelName:  &nu.Configs.Wireguard.TunnelName,
			FileContent: &nu.Configs.Wireguard.FileContent,
		}

		return m, nu.Name, nil
	}

	if nu.Configs.Amnezia != nil {
		m.AmneziaOVCConfig = &models.AmneziaOVCConfig{
			FileName:    &nu.Configs.Amnezia.FileName,
			TunnelName:  &nu.Configs.Amnezia.TunnelName,
			FileContent: &nu.Configs.Amnezia.FileContent,
		}

		return m, nu.Name, nil
	}

	if nu.Configs.Outline != nil {
		m.OutlineConfig = &models.OutlineConfig{
			AccessKey: nu.Configs.Outline,
		}

		return m, nu.Name, nil
	}

	if nu.Configs.Vgc != nil {
		m.VPNGenConfig = &models.VPNGenConfig{
			AccessKey: nu.Configs.Vgc,
		}

		return m, nu.Name, nil
	}

	return nil, "", fmt.Errorf("unknown config type")
}
