package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/brimblehq/migration/internal/db/cache"
	"github.com/brimblehq/migration/internal/types"
)

type DeviceInfo struct {
	DeviceId string `json:"deviceId"`
	Hostname string `json:"hostname"`
}

type DevicePayload struct {
	DeviceInfo DeviceInfo `json:"deviceInfo"`
}

type SetupResponse struct {
	Valid          bool   `json:"valid"`
	MaxDevices     int    `json:"max_devices"`
	DatabaseURI    string `json:"dbUri"`
	TailScaleToken string `json:"tailScaleToken"`
}

type APIResponse struct {
	Data types.LicenseResponse `json:"data"`
}

type SetupAPIResponse struct {
	Data SetupResponse `json:"data"`
}

var diskCache *cache.DiskCache

func GetSetupConfigurations(licenseKey string) (string, string, int, error) {
	// if diskCache != nil {
	// 	var config SetupResponse
	// 	if diskCache.Get(licenseKey, &config) {
	// 		return config.DatabaseURI, config.TailScaleToken, config.MaxDevices, nil
	// 	}
	// }

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	url := "https://core.brimble.io/v1/license/setup"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Brimble-Key", licenseKey)
	req.Header.Set("X-Setup-Type", "installation")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	var apiResp SetupAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", "", 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if !apiResp.Data.Valid {
		return "", "", 0, fmt.Errorf("invalid license key")
	}

	// if diskCache != nil {
	// 	config := SetupResponse{
	// 		DatabaseURI:    apiResp.Data.DatabaseURI,
	// 		TailScaleToken: apiResp.Data.TailScaleToken,
	// 		MaxDevices:     apiResp.Data.MaxDevices,
	// 	}

	// 	if err := diskCache.Set(licenseKey, config, 5*time.Minute); err != nil {
	// 		fmt.Printf("Warning: Failed to cache setup data: %v\n", err)
	// 	}
	// }

	return apiResp.Data.DatabaseURI, apiResp.Data.TailScaleToken, apiResp.Data.MaxDevices, nil
}

func ValidateOrRegisterMachineLicenseKey(licenseKey string, deviceId string, hostname string) (*types.LicenseResponse, error) {
	url := "https://core.brimble.io/v1/license"

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	payload := DevicePayload{
		DeviceInfo: DeviceInfo{
			DeviceId: deviceId,
			Hostname: hostname,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return &types.LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return &types.LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Brimble-Key", licenseKey)
	req.Header.Set("X-Setup-Type", "installation")

	resp, err := client.Do(req)
	if err != nil {
		return &types.LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, nil
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return &types.LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, fmt.Errorf("failed to decode response: %w", err)
	}

	return &apiResp.Data, nil
}
