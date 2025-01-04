package license

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type LicenseResponse struct {
	Valid           bool                   `json:"valid"`
	Key             string                 `json:"key"`
	ExpireIn        *string                `json:"expireIn"`
	TailScaleToken  string                 `json:"tailScaleToken"`
	DbConnectionUrl string                 `json:"connectionString"`
	Subscription    map[string]interface{} `json:"subscription,omitempty"`
}

type APIResponse struct {
	Data LicenseResponse `json:"data"`
}

func ValidateLicenseKey(licenseKey string) (*LicenseResponse, error) {
	url := fmt.Sprintf("https://d7e1-2605-6440-4002-1000-00-7656.ngrok-free.app/v1/license?key=%s", licenseKey)

	resp, err := http.Get(url)
	if err != nil {
		return &LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, nil
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return &LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, nil
	}

	return &apiResp.Data, nil
}
