package helpers

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/brimblehq/migration/internal/types"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
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

func PrintSuccessMessage(upRes auto.UpResult) {
	fmt.Println("\n✅ Successfully provisioned servers!")

	if ips, ok := upRes.Outputs["publicIps"].Value.([]interface{}); ok {
		fmt.Println("\n🖥️  Server Details:")
		for i, ip := range ips {
			fmt.Printf("   Server %d: %v\n", i+1, ip)
		}

		fmt.Println("\n🔑 Connection Information:")
		fmt.Printf("   SSH command: ssh root@%v\n", ips[0])
		fmt.Println("   Remember to wait a few minutes for the server to complete initialization")
	}

	fmt.Println("\n📝 Next Steps:")
	fmt.Println("1. Wait 2-3 minutes for the server to finish setup")
	fmt.Println("2. Try connecting via SSH")
	fmt.Println("3. Check server logs if needed: /var/log/cloud-init-output.log")
}

func ValidateFlags(cfg *types.FlagConfig) error {
	if len(os.Args) < 2 {
		// ui.PrintBanner()
		return errors.New("insufficient arguments")
	}

	if cfg.LicenseKey == "" {
		// ui.PrintBanner()
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
