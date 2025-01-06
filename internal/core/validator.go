package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type DeviceInfo struct {
	DeviceId string `json:"deviceId"`
	Hostname string `json:"hostname"`
}

type DevicePayload struct {
	DeviceInfo DeviceInfo `json:"deviceInfo"`
}

type SubscriptionResponse struct {
	ID             string    `json:"_id"`
	AdminID        string    `json:"admin_id"`
	BillableID     string    `json:"billable_id"`
	ProjectID      *string   `json:"project_id"`
	PlanType       string    `json:"plan_type"`
	Status         string    `json:"status"`
	DebitDate      time.Time `json:"debit_date"`
	StartDate      time.Time `json:"start_date"`
	ExpiryDate     time.Time `json:"expiry_date"`
	TriggerCreated bool      `json:"trigger_created"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Version        int       `json:"__v"`
	JobIdentifier  string    `json:"job_identifier"`
}

type LicenseResponse struct {
	Valid        bool                 `json:"valid"`
	Key          string               `json:"key"`
	ExpireIn     *string              `json:"expireIn"`
	Tag          string               `json:"tag"`
	Subscription SubscriptionResponse `json:"subscription,omitempty"`
}

type SetupResponse struct {
	Valid          bool   `json:"valid"`
	MaxDevices     int    `json:"max_devices"`
	DatabaseURI    string `json:"dbUri"`
	TailScaleToken string `json:"tailScaleToken"`
}

type APIResponse struct {
	Data LicenseResponse `json:"data"`
}

type SetupAPIResponse struct {
	Data SetupResponse `json:"data"`
}

func GetDatabaseUrl(licenseKey string) (string, string, int, error) {
	url := "https://core.brimble.io/v1/license/setup"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Brimble-Key", licenseKey)
	req.Header.Set("X-Setup-Type", "installation")

	resp, err := http.DefaultClient.Do(req)
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

	return apiResp.Data.DatabaseURI, apiResp.Data.TailScaleToken, apiResp.Data.MaxDevices, nil
}

func ValidateLicenseKey(licenseKey string, deviceId string, hostname string) (*LicenseResponse, error) {
	url := "https://core.brimble.io/v1/license"

	payload := DevicePayload{
		DeviceInfo: DeviceInfo{
			DeviceId: deviceId,
			Hostname: hostname,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return &LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return &LicenseResponse{
			Valid:    false,
			Key:      licenseKey,
			ExpireIn: nil,
		}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Brimble-Key", licenseKey)
	req.Header.Set("X-Setup-Type", "installation")

	resp, err := http.DefaultClient.Do(req)
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
		}, fmt.Errorf("failed to decode response: %w", err)
	}

	return &apiResp.Data, nil
}
