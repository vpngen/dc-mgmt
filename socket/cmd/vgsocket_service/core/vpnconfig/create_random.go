package vpnconfig

import (
	"context"
	"fmt"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/strfmt/conv"
	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/models"
	"github.com/vpngen/wordsgens/namesgenerator"
)

const (
	wiregiuardConfig = `[Interface]
Address = 100.98.7.238/32,fdee:fedc::ac8f:decc:d51:cc4c/128
PrivateKey = uP/37uV+PGlzuIyTfrmPxo9lRVMClTz9NW4LvTCVY10=
DNS = 100.98.7.6,fdee:fedc::ba98

[Peer]
Endpoint = 152.89.168.95:51820
PublicKey = B9SqzE32hweyMm//fWOTmy2UkRekDXZOl/9G8lvAplE=
PresharedKey = yNMX8nJUXAJsJl6iKQcoiOICNBAUtbWSYYu2m5TpnKs=
AllowedIPs = 0.0.0.0/0,::/0
",
`
	amneziavpnConfig = `vpn://AAAA_3icjFfbruLI1b7_nwL5ls32gfNWzy-V7fIBsI1PgPn9a2TswgfsMtgGY0YtJddzl-s8xCTKRSZR8gz0G0WG7k7PpKOeLW2BV31r1apa6_u8-IHwc1x5MUZFSbz93xePxBvhZRjdYq-XHxG-HHHPT3PvQLwQz8-3H4jUK6vv_Rzv45B4I35wCbbI6xIVZhy6xJtL-FGRZ8glXlwCYr9ojlWcYwVVUR481j1U9kI_ewDUc8blGLvEG_3iEssivzZfID-m8EAuz7s09ueoeaywU_N0g30mqlGjZCS5X2tW1jD2wUAHfrPVUnIqTtILOKbwu4e7gbK8QlJeVg9_esi8Tqav9GjyOh1-AVjmxRMwGPQfZhMVF1SoXnugNvc0Rtdjgcry1c-fRzCrAnmZFWcoP7e-fYp6cQmr8HB5_BQtiAvkVw-4LfMPE1ww3NLekKY3Ql7ii_Zy5ujgu-9c4j3xQrSexFubBfFCVJ9ifX8s8ion3ojKPxLvXz7dz1eL8vz-LEgaI1y5Lg7QpVOdseviR6BO5R9dFxeozNNLr0BV0XRivI9xXCHXxTjfxThowago47LqHVDzxdMzkB8fI1R0ADR7zHDUEznFdbF3rqKOKYEhzbguvqBi1-m7Lq7Ssvc5l_bh0obKcS-LcYd-bbEH1PSelxXnuEM_smvr0vNRUfWqtOyUj4I8Fp7AXuhVqPaaToD2dGfXHL2y7AXR42jtmSP_2MsfPdjhVbNDU9QrzYxeqdc2-i7N_UMvP1dlHKBegMunU5GfK9T5RZN0mOHw9Yv_DkbV9x-3_pxmh2bGr1Qbu0PT08Ez2Dvf-1_Xxb32j4WirHY4aFiyIHPAgg-r62JFljkh4TiwiUNQyywIZdbmwZwNw1N0SLSlrvMsDy6KSdXz2uFXus7DRme2opE4id1uteWoRuEVemHplGr5lZMIsdrQsZMptMaHjJpARuX9Wop8VUmUWuNhX7F0WuVBvY581XWxyj_MzWdz8t93_Np2bYiv7BhuyxoCR5rnW_mWUBzQHWE-gBBwCh86DmCBdFVZuNwpA9518ak5jXZ9iRJVn7NDGKihcPNy-Saa5E3Zy4so7s6W41Wabc5k4F8q1GQnUu8HgzDMq_3CdF0MrzcENx6yGZYRQomUcptvUn047S84Mrr1Jzu9e-KZ7pXcQNnGDMkDc9s_8xolNOpNaZs4NU1lM4qMc1_bGsWCme-HSjeayjf1FFZbCgVWsyFHKqdAUPOhw68Magl0iWSBzoNQFNuigokIaDuANWTJWhcUoLBgP6kl_QHXWNaBwnxd4DE8m-h20aoJOENGjZvNbR_m8SNELnLcSTSVwZQFCgQDcQF4jo3qGcvSDdAYwCVzahxVJ_6kW9p-bJQkxw-jcm1ZmtoWFZWH0pnDzWht3Dy8a7LDSjeS3XSflUOjWSVX5hJ2ka8FECwdGiDPqzk9FPGV709TVju5LmYPR607cJrTaKJvgJdu_IWvjuC5XJ4EyxsOJ07_sq33iTlQhuFSqhVhQfGAsdabLnKsqea6ON75fUhfyeQAqazVuY-MgCr_NT60vCGfxHnXcv-3MShqGYQ-M8iWHGMmLdV1F53WfetgeuvoaFxPzWE0qbnQkT82JAS1XLsuXvFwoYBDWzPIRgpnp_YV3oDBhuqKBaHCpeplt7Yru29EgWhfZQvsn2u5IsL03MqNuGp22YqSheC4FYNjICmhLRlDGV6pLQ9VBZSP8KCu4Upc3QIehEJNNcoNDBTLvz5ICCnFMjwRTBqFt68KD2slaW36VRXy-ltZui7-VqLfyvLReL9OlGOzG9DZ0P8oSDLLhnYR6jqQa42NQgABX2xZMrfVzdluxRzkDUfzXH5a4yN1DGbXoXi-nq5gxk_PiqOZ1FDsLxOwy1E5yyOcC6KWBsw2YubnZKTNXBfbi2gaWQk9QOwYbK2BsyEvCSDzgDVJ9jxdmokSN0VtcVlJRwclZeDFpEaS34dpkZRnx3Wxo2QeyUXBROyKYEMzcaJqY1ZJo0US5dusP-nnXk4rtW2BoL0jSR9AIdTt_nXsK6N5-wK04EwdzfWLT_lSAjRe4-paqh8cTlg2rIUcfIZ_Deu6mH_CradC2DVoJeH2S30HocbGNahDx-d0u0rrMzsLrUFoinvJdXHjXafY8YXbiL9Z3maN-txWpPKFgWyrWE4mHHL26LIGF22MqsVENubFYKKt11m3BAZ_a-9ClEiZN-IQHGRWHgxrleZs8XZjE2FC7r2r2ow268g3D8J2Pc45w6CUtbcY7fBoij2UTOaui6msVIM5G1F2cxiYc-rUHagpq1hhHmfHja8Gh8m3uP2O_MTodwfU_IrZS0NeAQt25tD5N7OlMwhhK54i2zy1EIRQZM0zywKQsLK0VmTJAiFkWSiDsOR9dIhTfj-xume97aMptZNsJdsNhHg6CLTFGNNZ35nqw6imd91wWca1NSXXksBdAnZuSjtmM9jks0I47ji5oGTXxctNGcgTLgqdw6eXlu5104teinChzWNjW8rTOjPBykYrZXUddaPtnF7qXUAv8DKD7ejk2WqWLMYGSqtFQK2EiXlkqGp6vuIhWFJkGE1W1eYQmyGY7Etya2j7rV0NV2NyJOoQ3VpxkATrVCrabjYeJ2k254b9g2BT660QaqwK8SqLbpIRpAFnlDPMZxNACcpkupdmcFMF3qqdx_Rfqu9XbvxRpEdtiPcubkfNMvKCvC5z_1B-Zdx8T7x___8vRID23jmtuN_wayJApV_Ej7GMeCOYYb9z_8OHH-__uP_zw-_vf7__9f7nDz_ef-7c_3j_6f63-58-_O7-0_3n-19aT1zSxBvxxRz3NDL_YYzysmon93bhy1GOeP8__woAAP__fMhNjg`
	outlineConfig    = `ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNToyR1ZFeVFwbTQ4c2c0THNiRDRXWlc1SFpVUDFQS2ZiRUpwUDRWNFpnM1d0b3lxM0NHZHpDMnBvSk5ZaUVkQnh5b1VId21QZXhrYnhmUEV1eGFrWUR3RG1lZDgzaXZBRWRAMTUyLjg5LjE2OC45NTo1NDU4Nw#253+%D0%92%D1%8B%D0%BD%D0%BE%D1%81%D0%BB%D0%B8%D0%B2%D1%8B%D0%B9+%D0%9C%D0%B0%D0%BA%D0%B1%D1%80%D0%B0%D0%B9%D0%B4`
	vgcConfig        = `vgc://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNToyR1ZFeVFwbTQ4c2c0THNiRDRXWlc1SFpVUDFQS2ZiRUpwUDRWNFpnM1d0b3lxM0NHZHpDMnBvSk5ZaUVkQnh5b1VId21QZXhrYnhmUEV1eGFrWUR3RG1lZDgzaXZBRWRAMTUyLjg5LjE2OC45NTo1NDU4Nw#253+%D0%92%D1%8B%D0%BD%D0%BE%D1%81%D0%BB%D0%B8%D0%B2%D1%8B%D0%B9+%D0%9C%D0%B0%D0%BA%D0%B1%D1%80%D0%B0%D0%B9%D0%B4`
	username         = "000 Марк Аврелий"
)

func CreateRandomConfig(ctx context.Context, _ uuid.UUID, configType string) (*models.VPNConfig, string, error) {
	name, _, err := namesgenerator.ChemistryAwardeeShort()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate name: %w", err)
	}

	filname := "000_M_Avrelyi.conf"
	tonelname := "000_M_Avrelyi"

	id := uuid.New().String()

	switch configType {
	case "wireguard":
		content := wiregiuardConfig
		return &models.VPNConfig{
			UserID: conv.UUID4(strfmt.UUID4(id)),
			WireGuardConfig: &models.WireGuardConfig{
				FileName:    &filname,
				TunnelName:  &tonelname,
				FileContent: &content,
			},
		}, name, nil
	case "amneziavpn":
		content := amneziavpnConfig
		return &models.VPNConfig{
			UserID: conv.UUID4(strfmt.UUID4(id)),
			AmneziaOVCConfig: &models.AmneziaOVCConfig{
				FileName:    &filname,
				TunnelName:  &tonelname,
				FileContent: &content,
			},
		}, name, nil

	case "outline":
		accessKey := outlineConfig
		return &models.VPNConfig{
			UserID: conv.UUID4(strfmt.UUID4(id)),
			OutlineConfig: &models.OutlineConfig{
				AccessKey: &accessKey,
			},
		}, name, nil

	case "vgc":
		fallthrough
	default:
		accessKey := vgcConfig
		return &models.VPNConfig{
			UserID: conv.UUID4(strfmt.UUID4(id)),
			VPNGenConfig: &models.VPNGenConfig{
				AccessKey: &accessKey,
			},
		}, name, nil

	}
}
