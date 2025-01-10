package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/types"
	"github.com/google/uuid"
	"golang.org/x/term"
)

func GetCleanErrorMessage(err error) string {
	msg := err.Error()

	noisePatterns := []string{
		"failed to run update: exit status",
		"sdk-v2/provider2.go:515:",
		"urn:pulumi:provision::serverProvision::",
	}

	for _, pattern := range noisePatterns {
		msg = strings.ReplaceAll(msg, pattern, "")
	}

	if idx := strings.Index(msg, "error:"); idx != -1 {
		msg = msg[idx+6:]
	}

	return strings.TrimSpace(msg)
}

func PrintSuccessMessage(filePath string, provisionedConfig *types.Config) error {
	if provisionedConfig == nil || len(provisionedConfig.Servers) == 0 {
		return fmt.Errorf("no servers were provisioned")
	}

	fmt.Println("\n✅ Successfully provisioned servers!")

	fmt.Println("\n🖥️  Server Details:")
	for i, server := range provisionedConfig.Servers {
		fmt.Printf("   Server %d: %s\n", i+1, server.PublicIP)
	}

	fmt.Println("\n🔑 Connection Information:")
	for i, server := range provisionedConfig.Servers {
		fmt.Printf("   Server %d SSH command: ssh -i %s %s@%s\n",
			i+1,
			server.KeyPath,
			server.Username,
			server.PublicIP)
	}
	fmt.Println("   Remember to wait a few minutes for the server to complete initialization")

	fmt.Println("\n📝 Next Steps:")
	fmt.Println("1. Wait 2-3 minutes for the server to finish setup")
	fmt.Printf("2. Run, runner setup --config_path=\"%s\"\n", filePath)

	return nil
}

func ValidateFlags(cfg *types.FlagConfig) error {
	if len(os.Args) < 2 {
		return errors.New("insufficient arguments")
	}

	if cfg.LicenseKey == "" {
		return errors.New("license key is required")
	}

	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		return fmt.Errorf("config file error: %v", err)
	}

	if instances, err := strconv.Atoi(cfg.Instances); err != nil || instances <= 0 {
		return errors.New("instances must be a positive number")
	}

	return nil
}

func GenerateUniqueReference(database *db.PostgresDB) (string, error) {
	reference := strings.ToLower(uuid.New().String()[:8])

	exists, err := database.CheckPulumiProvisionExists(context.Background(), reference)
	if err != nil {
		return "", fmt.Errorf("failed to check reference existence: %v", err)
	}

	if exists {
		return GenerateUniqueReference(database)
	}

	return reference, nil
}

func ReadBase64Input(prompt string) (string, error) {
	fmt.Printf("%s\n", prompt)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	var input strings.Builder
	buffer := make([]byte, 1)

	for {
		n, err := os.Stdin.Read(buffer)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}

		if buffer[0] == '\r' || buffer[0] == '\n' {
			if input.Len() > 0 {
				break
			}
			continue
		}

		if buffer[0] == 127 || buffer[0] == 8 {
			if input.Len() > 0 {
				str := input.String()
				input.Reset()
				input.WriteString(str[:len(str)-1])
			}
			continue
		}

		if buffer[0] < 32 {
			continue
		}

		input.WriteByte(buffer[0])
	}

	fmt.Printf("\n✓ Base64 content received successfully\n")

	return input.String(), nil
}

func SaveConfigToFile(config *types.Config) (string, error) {
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d-setup.json", timestamp)

	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling to JSON: %v", err)
	}

	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return "", fmt.Errorf("error writing file: %v", err)
	}

	fmt.Printf("Configuration saved to: %s\n", filename)
	return filename, nil
}

func GenerateProvisionedConfig(publicConfig interface{}, privateConfig interface{}, keyPathsConfig interface{}) (*types.Config, error) {
	publicIPs, ok := extractStringSlice(publicConfig)
	if !ok {
		return nil, fmt.Errorf("invalid public IPs format")
	}

	privateIPs, ok := extractStringSlice(privateConfig)
	if !ok {
		return nil, fmt.Errorf("invalid private IPs format")
	}

	keyPaths, ok := extractStringSlice(keyPathsConfig)
	if !ok {
		return nil, fmt.Errorf("invalid key paths format")
	}

	config, err := generateConfig(publicIPs, privateIPs, keyPaths)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func extractStringSlice(data interface{}) ([]string, bool) {
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Struct {
		valueField := val.FieldByName("Value")
		if valueField.IsValid() {
			if strSlice, ok := valueField.Interface().([]string); ok {
				return strSlice, true
			}
			if ifaceSlice, ok := valueField.Interface().([]interface{}); ok {
				result := make([]string, len(ifaceSlice))
				for i, v := range ifaceSlice {
					if str, ok := v.(string); ok {
						result[i] = str
					} else {
						return nil, false
					}
				}
				return result, true
			}
		}
	}

	switch v := data.(type) {
	case []string:
		return v, true
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[i] = str
		}
		return result, true
	}

	if m, ok := data.(map[string]interface{}); ok {
		if val, exists := m["Value"]; exists {
			if arr, ok := val.([]string); ok {
				return arr, true
			} else if arr, ok := val.([]interface{}); ok {
				result := make([]string, len(arr))
				for i, v := range arr {
					if str, ok := v.(string); ok {
						result[i] = str
					} else {
						return nil, false
					}
				}
				return result, true
			}
		}
	}

	// For debugging
	fmt.Printf("Failed to extract string slice. Type: %T, Value: %+v\n", data, data)
	return nil, false
}

func generateConfig(publicIPs []string, privateIPs []string, keyPaths []string) (*types.Config, error) {
	if len(publicIPs) != len(privateIPs) || len(publicIPs) != len(keyPaths) {
		return nil, fmt.Errorf("mismatched lengths: public IPs (%d), private IPs (%d), key paths (%d)",
			len(publicIPs), len(privateIPs), len(keyPaths))
	}

	servers := make([]types.Server, len(publicIPs))
	for i := range publicIPs {
		servers[i] = types.Server{
			Host:       fmt.Sprintf("instance-%d", i+1),
			Username:   "root",
			KeyPath:    keyPaths[i],
			Region:     "europe-west4",
			PublicIP:   publicIPs[i],
			PrivateIP:  privateIPs[i],
			AuthMethod: "key_path",
		}
	}

	config := &types.Config{
		Servers: servers,
		ClusterConfig: types.ClusterConfig{
			ConsulConfig: types.ConsulConfig{
				ConsulImage: "hashicorp/consul:1.16",
			},
			MonitoringConfig: types.MonitoringConfig{
				GrafanaPassword: "password",
				MetricsPort:     9100,
			},
			Versions: types.Versions{
				Docker: "latest",
				NodeJS: "20.x",
				Nomad:  "1.6.3",
			},
			Runner: types.Runner{
				Port:     3000,
				Instance: 4,
			},
		},
	}

	return config, nil
}
