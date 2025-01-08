package auth

import (
	"context"
	"fmt"

	infisical "github.com/infisical/go-sdk"

	"github.com/brimblehq/migration/internal/core"
)

func InitializeInfisical(ctx context.Context, infisicalSiteURL string) (infisical.InfisicalClientInterface, error) {
	client := infisical.NewInfisicalClient(ctx, infisical.Config{
		SiteUrl:          infisicalSiteURL,
		AutoTokenRefresh: true,
		SilentMode:       true,
	})

	_, err := client.Auth().UniversalAuthLogin(
		"881d58d5-44ed-4950-bfd1-b77f04b9a8e4",
		"c0ef8cff37718b02a5603c05dbc84ae3109c20edd0b31db2a505602da2295f22",
	)
	return client, err
}

func GetDecryptedSecrets(dbUrl, tailScaleToken, apiKeySecret string) (string, string, error) {
	decryptedDB, err := core.Decrypt(dbUrl, apiKeySecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to decrypt DB URL: %v", err)
	}

	decryptedTailScale, err := core.Decrypt(tailScaleToken, apiKeySecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to decrypt TailScale token: %v", err)
	}

	return decryptedDB, decryptedTailScale, nil
}
