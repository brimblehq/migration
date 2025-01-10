package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/brimblehq/migration/internal/auth"
	"github.com/brimblehq/migration/internal/core"
	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/helpers"
	"github.com/brimblehq/migration/internal/infra"
	"github.com/brimblehq/migration/internal/manager"
	"github.com/brimblehq/migration/internal/notification"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	infisical "github.com/infisical/go-sdk"
	"github.com/spf13/cobra"
)

const (
	infisicalSiteURL = "https://app.infisical.com"
	projectID        = "64a5804271976de3e38c59c3"
)

var (
	licenseKey string
	configPath string
	useTemp    bool

	rootCmd = &cobra.Command{
		Use:   "runner",
		Short: "Infrastructure setup and management tool",
		Long:  `Brimble cli tool for infrastructure setup and management, including server provisioning and configuration.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	setupCmd = &cobra.Command{
		Use:   "setup",
		Short: "Run infrastructure setup",
		RunE:  runSetup,
	}

	provisionCmd = &cobra.Command{
		Use:   "provision",
		Short: "Provision infrastructure",
		RunE:  runProvision,
	}

	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize runner configuration",
		RunE:  runInit,
	}
)

func Execute() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Exit(1)
	}()

	rootCmd.SilenceErrors = false
	rootCmd.SilenceUsage = false

	template.New("help").Parse(`{{.Long | trimTrailingWhitespaces}}{{if or .Runnable .HasSubCommands}}{{.UsageString | trimTrailingWhitespaces}}{{end}}`)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "./config.json", "Path to config file")

	rootCmd.PersistentFlags().BoolVar(&useTemp, "use-temp", false, "Use temporary SSH key")

	rootCmd.PersistentFlags().StringVar(&licenseKey, "license-key", "", "License key for runner (optional)")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "init" || cmd.Name() == "help" {
			return nil
		}

		if licenseKey == "" {
			var err error
			licenseKey, err = auth.LoadLicenseKey()
			if err != nil {
				return err
			}
			if licenseKey == "" {
				return fmt.Errorf("license key not found. Please run 'runner init' to configure your license key")
			}
		}
		return nil
	}

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(provisionCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Print("Please enter your license key: ")

	reader := bufio.NewReader(os.Stdin)

	licenseKey, err := reader.ReadString('\n')

	if err != nil {
		return fmt.Errorf("failed to read license key: %v", err)
	}

	licenseKey = strings.TrimSpace(licenseKey)

	_, _, _, err = core.GetSetupConfigurations(licenseKey)

	if err != nil {
		return fmt.Errorf("invalid license key: %v", err)
	}

	if err := auth.SaveLicenseKey(licenseKey); err != nil {
		return err
	}

	fmt.Println("License key successfully saved!")

	return nil
}

func runProvision(cmd *cobra.Command, args []string) error {
	notifier := notification.New()

	database, _, _, maxDevices, err := setupInitialServices()

	if err != nil {
		return fmt.Errorf("failed to setup initial services: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sshManager, err := setupSSHManager(ctx, database, maxDevices)

	if err != nil {
		return err
	}

	if err := infra.ProvisionInfrastructure(licenseKey, maxDevices, database, sshManager, notifier); err != nil {
		return err
	}

	return nil
}

func runSetup(cmd *cobra.Command, args []string) error {
	notifier := notification.New()

	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	var config types.Config
	if err := json.Unmarshal(configFile, &config); err != nil {
		return fmt.Errorf("error parsing config: %v", err)
	}

	database, _, decryptedTailScale, maxDevices, err := setupInitialServices()
	if err != nil {
		return err
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sshManager, err := setupSSHManager(ctx, database, maxDevices)
	if err != nil {
		return err
	}

	if err := setupServers(ctx, &config, database, sshManager, decryptedTailScale, notifier); err != nil {
		return err
	}

	log.Println("Infrastructure setup completed ✅")

	err = notifier.Send("Installation Complete", "Brimble is now ready to use !")

	if err != nil {
		log.Printf("Failed to send notification: %v", err)
	}
	return nil
}

func setupInitialServices() (*db.PostgresDB, infisical.InfisicalClientInterface, string, int, error) {
	dbUrl, tailScaleToken, maxDevices, err := core.GetSetupConfigurations(licenseKey)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("failed to get database URL: %v", err)
	}

	ctx := context.Background()
	client, err := auth.InitializeInfisical(ctx, infisicalSiteURL)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("authentication failed: %v", err)
	}

	apiKeySecret, err := client.Secrets().Retrieve(infisical.RetrieveSecretOptions{
		SecretKey:   "CLI_DECRYPTION_KEY",
		Environment: "staging",
		ProjectID:   projectID,
		SecretPath:  "/",
	})
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("error retrieving secret: %v", err)
	}

	decryptedDB, decryptedTailScale, err := auth.GetDecryptedSecrets(dbUrl, tailScaleToken, apiKeySecret.SecretValue)
	if err != nil {
		return nil, nil, "", 0, err
	}

	database, err := db.NewPostgresDB(db.DbConfig{URI: decryptedDB})
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("failed to connect to database: %v", err)
	}

	return database, client, decryptedTailScale, maxDevices, nil
}

func setupServers(ctx context.Context, config *types.Config, database *db.PostgresDB,
	sshManager *ssh.TempSSHManager, decryptedTailScale string, notifier *notification.DefaultNotifier) error {

	servers := helpers.GetServerList(database, *config)

	clusterRoles := manager.NewClusterRoles(servers)

	clusterRoles.CalculateRoles(config.Servers)

	var wg sync.WaitGroup
	errorChan := make(chan error, len(config.Servers))
	semaphore := make(chan struct{}, 5)

	for _, server := range config.Servers {
		wg.Add(1)
		go func(server types.Server) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := processServer(ctx, server, sshManager, config, database,
				decryptedTailScale, clusterRoles, errorChan, notifier); err != nil {
				errorChan <- err
			}
		}(server)
	}

	wg.Wait()
	close(errorChan)

	return helpers.ProcessErrors(errorChan)
}

func processServer(ctx context.Context, server types.Server, sshManager *ssh.TempSSHManager,
	config *types.Config, database *db.PostgresDB, decryptedTailScale string,
	clusterRoles *manager.ClusterManager, errorChan chan error, notifier *notification.DefaultNotifier) error {

	client, err := ssh.HandleServerAuth(server, sshManager, useTemp)
	if err != nil {
		return fmt.Errorf("error connecting to %s: %v", server.Host, err)
	}

	machineID, err := client.ExecuteCommandWithOutput("cat /etc/machine-id")
	if err != nil {
		return fmt.Errorf("error getting machine-id from %s: %v", server.Host, err)
	}

	hostname, err := client.ExecuteCommandWithOutput("hostname")
	if err != nil {
		return fmt.Errorf("error getting hostname from %s: %v", server.Host, err)
	}

	setup := manager.ServerSetup{
		Client:    client,
		Server:    server,
		MachineID: strings.TrimSpace(machineID),
		Hostname:  strings.TrimSpace(hostname),
	}

	manager.SetupServer(ctx, setup, config, sshManager, useTemp, decryptedTailScale,
		database, licenseKey, clusterRoles, errorChan, notifier)

	return nil
}

func setupSSHManager(ctx context.Context, database *db.PostgresDB, maxDevices int) (*ssh.TempSSHManager, error) {
	fmt.Println("Setting up SSH Manager...")

	configFile, err := os.ReadFile(configPath)
	var config types.Config

	if err != nil {
		config = types.Config{}
	} else {
		if err := json.Unmarshal(configFile, &config); err != nil {
			return nil, fmt.Errorf("error parsing config: %v", err)
		}
	}

	setupManager := ssh.NewSSHSetupManager(database, &config)

	sshManager, err := setupManager.ValidateAndInitializeSSH(ctx, useTemp, maxDevices)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SSH: %v", err)
	}

	if sshManager == nil {
		return nil, fmt.Errorf("SSH manager is nil after initialization")
	}

	fmt.Println("SSH Manager successfully initialized")
	return sshManager, nil
}
