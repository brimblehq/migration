package ui

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/types"
	"github.com/manifoldco/promptui"
)

func InteractiveProvisioning(database *db.PostgresDB, maxDevices int) (string, *types.ProvisionServerConfig, error) {
	dbProviders, err := database.GetProviderConfigs()

	if err != nil {
		return "", nil, fmt.Errorf("failed to get provider configs: %v", err)
	}

	var providers []types.Provider
	for _, p := range dbProviders {
		providers = append(providers, convertProvider(p))
	}

	providerPrompt := promptui.Select{
		Label: "Select Cloud Provider",
		Items: providers,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ .ID }}",
			Active:   "➤ {{ .ID }}",
			Inactive: "  {{ .ID }}",
			Selected: "✔ {{ .ID }}",
		},
	}

	providerIdx, _, err := providerPrompt.Run()
	if err != nil {
		return "", nil, fmt.Errorf("provider prompt failed: %v", err)
	}

	selectedProvider := providers[providerIdx]

	regionOptions, err := database.GetProviderRegions(selectedProvider.ID)

	if err != nil {
		return "", nil, fmt.Errorf("failed to get regions for provider %s: %v", selectedProvider.ID, err)
	}

	if len(regionOptions) == 0 {
		return "", nil, fmt.Errorf("no regions available for provider %s", selectedProvider.ID)
	}

	regionPrompt := promptui.Select{
		Label: "Select Region",
		Items: regionOptions,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ .DisplayName }}",
			Active:   "➤ {{ .DisplayName }}",
			Inactive: "  {{ .DisplayName }}",
			Selected: "✔ {{ .DisplayName }}",
		},
	}

	regionIdx, _, err := regionPrompt.Run()
	if err != nil {
		return "", nil, fmt.Errorf("region prompt failed: %v", err)
	}

	selectedRegion := regionOptions[regionIdx].ID

	var filteredMachines []types.Machine
	for _, machine := range selectedProvider.Machines {
		if machine.Region.Type == selectedRegion {
			filteredMachines = append(filteredMachines, machine)
		}
	}

	if len(filteredMachines) == 0 {
		return "", nil, fmt.Errorf("no machines available in %s region for provider %s",
			regionOptions[regionIdx].DisplayName, selectedProvider.ID)
	}

	machinePrompt := promptui.Select{
		Label: "Select Machine Type",
		Items: filteredMachines,
		Size:  5,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}",
			Active:   "➤ {{ .Description }} ({{ .Size }})",
			Inactive: "  {{ .Description }} ({{ .Size }})",
			Selected: "✔ {{ .Description }} ({{ .Size }})",
		},
		Searcher: func(input string, index int) bool {
			machine := filteredMachines[index]
			name := strings.ToLower(machine.Description + machine.Size)
			input = strings.ToLower(input)
			return strings.Contains(name, input)
		},
	}

	machineIdx, _, err := machinePrompt.Run()
	if err != nil {
		return "", nil, fmt.Errorf("machine prompt failed: %v", err)
	}

	selectedMachine := filteredMachines[machineIdx]

	validateCount := func(input string) error {
		count, err := strconv.Atoi(input)
		if err != nil {
			return errors.New("invalid number")
		}
		if count < 1 {
			return errors.New("must be at least 1")
		}
		// if count > maxDevices {
		// 	return errors.New(fmt.Sprintf("maximum %d instances allowed", maxDevices))
		// }
		return nil
	}

	countPrompt := promptui.Prompt{
		Label:    "Number of instances",
		Default:  "1",
		Validate: validateCount,
	}

	countStr, err := countPrompt.Run()
	if err != nil {
		return "", nil, fmt.Errorf("count prompt failed: %v", err)
	}

	count, _ := strconv.Atoi(countStr)

	if err := validateProviderAuth(selectedProvider.ID); err != nil {
		fmt.Printf("\n⚠️  %v\n", err)
		if err := promptForCredentials(selectedProvider.ID); err != nil {
			return "", nil, fmt.Errorf("failed to get credentials: %v", err)
		}
	} else {
		fmt.Printf("\n✅ Found %s credentials in environment\n", selectedProvider.ID)
	}

	fmt.Printf("\n🚀 Provisioning Summary:\n")
	fmt.Printf("✔ Cloud Provider: %s\n", selectedProvider.Name)
	fmt.Printf("✔ Region: %s (%s)\n", regionOptions[regionIdx].DisplayName, selectedRegion)
	fmt.Printf("✔ Machine Type: %s (%s)\n", selectedMachine.Description, selectedMachine.Size)
	fmt.Printf("✔ Number of Instances: %d\n", count)
	fmt.Println()

	confirm := promptui.Select{
		Label: "Do you want to proceed with the setup?",
		Items: []string{"Yes", "No"},
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}",
			Active:   "➤ {{ . | green }}",
			Inactive: "  {{ . }}",
			Selected: "✔ {{ . | green }}",
		},
	}

	idx, _, err := confirm.Run()
	if err != nil {
		return "", nil, fmt.Errorf("confirmation prompt failed: %v", err)
	}

	if idx == 1 {
		return "", nil, fmt.Errorf("setup cancelled by user")
	}

	config := &types.ProvisionServerConfig{
		Name:   selectedMachine.ID,
		Size:   selectedMachine.Size,
		Region: selectedMachine.Region.Name,
		Image:  selectedMachine.Image,
		Count:  count,
		Tags:   []string{"brimble", selectedMachine.Role},
	}

	return selectedProvider.Name, config, nil
}

func convertProvider(dbProvider types.Provider) types.Provider {
	machines := make([]types.Machine, len(dbProvider.Machines))
	for i, m := range dbProvider.Machines {
		machines[i] = types.Machine{
			ID:          m.ID,
			Size:        m.Size,
			Image:       m.Image,
			Description: m.Description,
			UseCase:     m.UseCase,
			Role:        m.Role,
			Region: types.Region{
				Name: m.Region.Name,
				Type: m.Region.Type,
			},
		}
	}

	return types.Provider{
		ID:       dbProvider.ID,
		Name:     dbProvider.ID,
		Machines: machines,
	}
}

func validateProviderAuth(provider string) error {
	switch provider {
	case "hetzner":
		if token := os.Getenv("HCLOUD_TOKEN"); token == "" {
			return fmt.Errorf("HCLOUD_TOKEN environment variable not set")
		}
	case "aws":
		if id := os.Getenv("AWS_ACCESS_KEY_ID"); id == "" {
			return fmt.Errorf("AWS_ACCESS_KEY_ID environment variable not set")
		}
		if secret := os.Getenv("AWS_SECRET_ACCESS_KEY"); secret == "" {
			return fmt.Errorf("AWS_SECRET_ACCESS_KEY environment variable not set")
		}
		if region := os.Getenv("AWS_REGION"); region == "" {
			return fmt.Errorf("AWS_REGION environment variable not set")
		}
	case "digitalocean":
		if token := os.Getenv("DIGITALOCEAN_ACCESS_TOKEN"); token == "" {
			return fmt.Errorf("DIGITALOCEAN_ACCESS_TOKEN environment variable not set")
		}
	case "gcp":
		if creds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); creds == "" {
			return fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS environment variable not set")
		}
	}
	return nil
}

func promptForCredentials(provider string) error {
	fmt.Printf("\n🔐 Authentication required for %s\n", provider)
	fmt.Println("Note: Credentials will only be used for this session and won't be stored.")

	switch provider {
	case "hetzner":
		token, err := promptSecret("Enter Hetzner Cloud API Token")
		if err != nil {
			return err
		}
		os.Setenv("HCLOUD_TOKEN", token)

	case "aws":
		accessKey, err := promptSecret("Enter AWS Access Key ID")
		if err != nil {
			return err
		}
		secretKey, err := promptSecret("Enter AWS Secret Access Key")
		if err != nil {
			return err
		}
		region, err := promptSecret("Enter AWS Region (e.g., us-east-1)", "text")
		if err != nil {
			return err
		}
		os.Setenv("AWS_ACCESS_KEY_ID", accessKey)
		os.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
		os.Setenv("AWS_REGION", region)

	case "digitalocean":
		token, err := promptSecret("Enter DigitalOcean Access Token")
		if err != nil {
			return err
		}
		os.Setenv("DIGITALOCEAN_ACCESS_TOKEN", token)

	case "gcp":
		fmt.Println("\nFor GCP authentication, you can provide:")
		fmt.Println("1. Path to service account JSON file")
		fmt.Println("2. Service account JSON content")
		fmt.Println("3. Base64 encoded service account JSON")

		prompt := promptui.Select{
			Label: "Select credential type",
			Items: []string{"File Path", "JSON Content", "Base64 Encoded (Recommended)"},
		}

		idx, _, err := prompt.Run()
		if err != nil {
			return err
		}

		var creds string
		switch idx {
		case 0:
			creds, err = promptSecret("Enter path to service account JSON file", "text")
			if err != nil {
				return err
			}
			if _, err := os.Stat(creds); os.IsNotExist(err) {
				return fmt.Errorf("file does not exist: %s", creds)
			}
		case 1:
			creds, err = promptSecret("Enter service account JSON content")
			if err != nil {
				return err
			}
			tmpFile, err := os.CreateTemp("", "gcp-creds-*.json")
			if err != nil {
				return err
			}
			if _, err := tmpFile.WriteString(creds); err != nil {
				return err
			}
			creds = tmpFile.Name()
		case 2:
			base64Creds, err := helpers.ReadBase64Input("Enter base64 encoded service account JSON:")
			if err != nil {
				return err
			}
			decoded, err := base64.StdEncoding.DecodeString(base64Creds)
			if err != nil {
				return fmt.Errorf("invalid base64 encoding: %v", err)
			}

			var jsonCheck map[string]interface{}
			if err := json.Unmarshal(decoded, &jsonCheck); err != nil {
				return fmt.Errorf("decoded content is not valid JSON: %v", err)
			}

			tmpFile, err := os.CreateTemp("", "gcp-creds-*.json")
			if err != nil {
				return err
			}
			if _, err := tmpFile.Write(decoded); err != nil {
				return err
			}
			creds = tmpFile.Name()
		}

		projectID, err := promptSecret("Enter GCP Project ID", "text")
		if err != nil {
			return err
		}

		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", creds)
		os.Setenv("GOOGLE_CLOUD_PROJECT", projectID)
	}

	return nil
}

func promptSecret(label string, promptType ...string) (string, error) {
	prompt := promptui.Prompt{
		Label: label,
		Validate: func(s string) error {
			if s == "" {
				return fmt.Errorf("value cannot be empty")
			}
			return nil
		},
	}

	if len(promptType) > 0 {
		switch promptType[0] {
		case "text":
			prompt.Mask = 0
		case "base64":
			prompt.Mask = 0
			prompt.HideEntered = true
			prompt.IsVimMode = true
			prompt.Validate = func(s string) error {
				if s == "" {
					return fmt.Errorf("value cannot be empty")
				}
				fmt.Printf("\r\033[K✓ Base64 content received successfully\n")
				return nil
			}
		default:
			prompt.Mask = '*'
		}
	} else {
		prompt.Mask = '*'
	}

	return prompt.Run()
}
