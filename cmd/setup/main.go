package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/license"
	"github.com/brimblehq/migration/internal/manager"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/brimblehq/migration/internal/ui"
	infisical "github.com/infisical/go-sdk"
)

func main() {
	licenseKey := flag.String("license-key", "", "License key for runner")
	configPath := flag.String("config", "./config.json", "Path to config file")

	flag.Parse()

	if *licenseKey == "" {
		log.Fatal("License key is required")
	}

	configFile, err := os.ReadFile(*configPath)

	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config types.Config

	if err := json.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	dbUrl, err := license.GetDatabaseUrl(*licenseKey)

	if err != nil {
		log.Fatalf("Failed to get database URL: %v", err)
	}

	client := infisical.NewInfisicalClient(context.Background(), infisical.Config{
		SiteUrl:          "https://app.infisical.com",
		AutoTokenRefresh: true,
	})

	_, err = client.Auth().UniversalAuthLogin("881d58d5-44ed-4950-bfd1-b77f04b9a8e4", "c0ef8cff37718b02a5603c05dbc84ae3109c20edd0b31db2a505602da2295f22")

	if err != nil {
		fmt.Printf("Authentication failed: %v", err)
		os.Exit(1)
	}

	apiKeySecret, err := client.Secrets().Retrieve(infisical.RetrieveSecretOptions{
		SecretKey:   "CLI_DECRYPTION_KEY",
		Environment: "staging",
		ProjectID:   "64a5804271976de3e38c59c3",
		SecretPath:  "/",
	})

	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	decryptedValue, err := license.Decrypt(dbUrl, apiKeySecret.SecretValue)

	if err != nil {
		log.Fatalf("Failed to get database URL: %v", err)
	}

	if dbUrl == "" {
		log.Fatal("Unable to setup this installation: missing database connection URL")
	}

	database, err := db.NewPostgresDB(db.Config{
		URI: decryptedValue,
	})

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	defer database.Close()

	existingServers, err := database.GetAllServers()
	if err != nil {
		log.Fatalf("Failed to get existing servers: %v", err)
	}

	var allServers []types.Server
	existingIPs := make(map[string]struct {
		step      types.ServerStep
		publicIP  string
		privateIP string
	})

	for _, srv := range existingServers {
		server := types.Server{
			Host:      srv.MachineID,
			PublicIP:  srv.PublicIP,
			PrivateIP: srv.PrivateIP,
		}
		allServers = append(allServers, server)
		existingIPs[srv.PrivateIP] = struct {
			step      types.ServerStep
			publicIP  string
			privateIP string
		}{
			step:      srv.CurrentStep,
			publicIP:  srv.PublicIP,
			privateIP: srv.PrivateIP,
		}
	}

	for _, configServer := range config.Servers {
		if existingInfo, exists := existingIPs[configServer.PrivateIP]; exists {
			if existingInfo.step != types.StepCompleted {
				allServers = append(allServers, configServer)
				// log.Printf("Server %s found in incomplete state (step: %s), will resume setup",
				// 	configServer.Host, existingInfo.step)
			}
			continue
		}
		allServers = append(allServers, configServer)
	}

	clusterRoles := manager.NewClusterRoles(allServers)

	clusterRoles.CalculateRoles(config.Servers)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	errorChan := make(chan error)

	var wg sync.WaitGroup

	// In your main.go
	for _, server := range config.Servers {
		wg.Add(1)

		go func(server types.Server) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
				spinner := ui.NewStepSpinner(server.Host)

				client, err := ssh.NewSSHClient(server)

				if err != nil {
					spinner.Start("Connecting to server")
					spinner.Stop(false)
					log.Printf("Error connecting to %s: %v", server.Host, err)
					return
				}
				defer client.Close()

				spinner.Start("Getting machine info")
				machineID, err := client.ExecuteCommandWithOutput("cat /etc/machine-id")
				if err != nil {
					spinner.Stop(false)
					log.Printf("Error getting machine-id from %s: %v", server.Host, err)
					return
				}

				hostname, err := client.ExecuteCommandWithOutput("hostname")
				if err != nil {
					spinner.Stop(false)
					log.Printf("Error getting hostname from %s: %v", server.Host, err)
					return
				}
				spinner.Stop(true)

				spinner.Start("Validating license")
				licenseResp, err := license.ValidateLicenseKey(*licenseKey, strings.TrimSpace(machineID), strings.TrimSpace(hostname))
				if err != nil || !licenseResp.Valid {
					spinner.Stop(false)
					log.Printf("Invalid license for server %s, reach out to hello@brimble.app for support", server.Host)
					os.Exit(1)
					return
				}
				spinner.Stop(true)

				roles := clusterRoles.RoleMapping[server.Host]

				currentStep, err := database.GetServerStep(machineID)
				if err != nil {
					role := "client"
					if len(roles) > 1 {
						role = "both"
					}

					err = database.RegisterServer(
						machineID,
						server.PublicIP,
						server.PrivateIP,
						role,
						licenseResp.Subscription.ID,
						types.StepInitialized,
					)
					if err != nil {
						spinner.Stop(false)
						log.Printf("Error registering server %s: %v", server.Host, err)
						return
					}
					currentStep = types.StepInitialized
				}

				im := manager.NewInstallationManager(client, server, roles, &config, licenseResp.TailScaleToken, database)

				steps := []struct {
					name    string
					fn      func() error
					step    types.ServerStep
					require types.ServerStep
				}{
					// {
					// 	name:    "Verifying machine requirements",
					// 	fn:      im.VerifyMachineRequirement,
					// 	step:    types.StepVerified,
					// 	require: types.StepInitialized,
					// },
					{
						name:    "Installing base packages",
						fn:      im.InstallBasePackages,
						step:    types.StepBaseInstalled,
						require: types.StepVerified,
					},
					{
						name:    "Setting up Consul client",
						fn:      im.SetupConsulClient,
						step:    types.StepConsulSetup,
						require: types.StepBaseInstalled,
					},
					{
						name:    "Setting up Nomad",
						fn:      im.SetupNomad,
						step:    types.StepNomadSetup,
						require: types.StepConsulSetup,
					},
					{
						name:    "Setting up monitoring",
						fn:      im.SetupMonitoring,
						step:    types.StepMonitoringSetup,
						require: types.StepNomadSetup,
					},
					{
						name:    "Starting runner",
						fn:      func() error { return im.StartRunner(*licenseKey) },
						step:    types.StepRunnerStarted,
						require: types.StepMonitoringSetup,
					},
				}

				for _, step := range steps {
					select {
					case <-ctx.Done():
						return
					default:
						if currentStep > step.step {
							continue
						}

						if currentStep < step.require {
							log.Printf("Prerequisite step %s not completed for %s", step.require, server.Host)
							return
						}

						spinner.Start(step.name)
						if err := step.fn(); err != nil {
							spinner.Stop(false)
							errorChan <- fmt.Errorf("error during %s on %s: %v", step.name, server.Host, err)
							cancel()
							return
						}

						if err := database.UpdateServerStep(machineID, step.step); err != nil {
							spinner.Stop(false)
							log.Printf("Error updating step for server %s: %v", server.Host, err)
							return
						}
						currentStep = step.step
						spinner.Stop(true)
					}
				}

				if err := database.UpdateServerStep(machineID, types.StepCompleted); err != nil {
					log.Printf("Error marking server %s as completed: %v", server.Host, err)
				}
			}
		}(server)
	}

	var setupFailed bool

	go func() {
		wg.Wait()
		close(errorChan)
	}()

	for err := range errorChan {
		if err != nil {
			log.Printf("❌ Setup failed: %v", err)
			setupFailed = true
			os.Exit(1)
		}
	}

	if !setupFailed {
		log.Println("Infrastructure setup completed ✅")
	}
}
