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

	"github.com/brimblehq/migration/internal/core"
	"github.com/brimblehq/migration/internal/db"
	"github.com/brimblehq/migration/internal/manager"
	"github.com/brimblehq/migration/internal/ssh"
	"github.com/brimblehq/migration/internal/types"
	"github.com/brimblehq/migration/internal/ui"
	infisical "github.com/infisical/go-sdk"
)

func main() {
	var wg sync.WaitGroup

	var setupFailed bool

	licenseKey := flag.String("license-key", "", "License key for runner")
	instances := flag.String("instances", "6", "Number of instances for your brimble builder")
	configPath := flag.String("config", "./config.json", "Path to config file")
	useTemp := flag.Bool("temp-ssh", false, "Use temporary SSH keys for setup")

	flag.Usage = func() {
		ui.PrintBanner(false)
	}

	flag.Parse()

	if len(os.Args) < 2 {
		ui.PrintBanner()
		os.Exit(1)
	}

	if *licenseKey == "" {
		log.Fatal("License key is required")
		ui.PrintBanner()
		os.Exit(1)
	}

	configFile, err := os.ReadFile(*configPath)

	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config types.Config

	if err := json.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	dbUrl, tailScaleToken, maxDevices, err := core.GetDatabaseUrl(*licenseKey)

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

	decryptedDatabaseValue, err := core.Decrypt(dbUrl, apiKeySecret.SecretValue)

	if err != nil {
		log.Fatalf("Failed to get database URL: %v", err)
	}

	if dbUrl == "" {
		log.Fatal("Unable to setup this installation: missing database connection URL")
	}

	decryptedTailScaleValue, err := core.Decrypt(tailScaleToken, apiKeySecret.SecretValue)

	if err != nil {
		log.Fatalf("Failed to get tailscale token: %v", err)
	}

	database, err := db.NewPostgresDB(db.Config{
		URI: decryptedDatabaseValue,
	})

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	defer database.Close()

	ctx, cancel := context.WithCancel(context.Background())

	setupManager := ssh.NewSSHSetupManager(database, &config)

	sshManager, err := setupManager.ValidateAndInitializeSSH(ctx, *useTemp, maxDevices)

	if err != nil {
		log.Fatalf("%v", err)
	}

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
			}
			continue
		}
		allServers = append(allServers, configServer)
	}

	clusterRoles := manager.NewClusterRoles(allServers)

	clusterRoles.CalculateRoles(config.Servers)

	defer cancel()

	errorChan := make(chan error)

	for _, server := range config.Servers {
		wg.Add(1)

		go func(server types.Server) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
				terminalOutput := ui.NewTerminalOutput(server.Host)

				spinner := ui.NewStepSpinner(server.Host, terminalOutput)

				var (
					client *ssh.SSHClient
					err    error
				)

				client, err = ssh.HandleServerAuth(server, sshManager, *useTemp)

				if err != nil {
					setupFailed = true
					spinner.Start("Connecting to server")
					spinner.Stop(false)
					log.Printf("Error connecting to (%s): %v", server.Host, err)
					return
				}

				if *useTemp {
					defer func() {
						if err := sshManager.Cleanup(ctx, client); err != nil {
							log.Printf("Warning: Failed to cleanup SSH key on %s: %v", server.Host, err)
						}
					}()
				}

				defer client.Close()

				spinner.Start("Getting machine info")

				machineID, err := client.ExecuteCommandWithOutput("cat /etc/machine-id")
				if err != nil {
					setupFailed = true
					spinner.Stop(false)
					log.Printf("Error getting machine-id from (%s): %v", server.Host, err)
					return
				}

				hostname, err := client.ExecuteCommandWithOutput("hostname")

				if err != nil {
					setupFailed = true
					spinner.Stop(false)
					log.Printf("Error getting hostname from (%s): %v", server.Host, err)
					return
				}

				spinner.Stop(true)

				spinner.Start("Validating license")

				licenseResp, err := core.ValidateLicenseKey(*licenseKey, strings.TrimSpace(machineID), strings.TrimSpace(hostname))
				if err != nil || !licenseResp.Valid {
					setupFailed = true
					spinner.Stop(false)
					log.Printf("Invalid license for server (%s), reach out to hello@brimble.app for support", server.Host)
					os.Exit(1)
					return
				}
				spinner.Stop(true)

				roles := clusterRoles.RoleMapping[server.Host]

				currentStep, err := database.GetServerStep(machineID, licenseResp.Subscription.ID)

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
						setupFailed = true
						spinner.Stop(false)
						log.Printf("Error registering server %s: %v", server.Host, err)
						return
					}
					currentStep = types.StepInitialized
				}

				im := manager.NewInstallationManager(client, server, roles, &config, decryptedTailScaleValue, database, licenseResp)

				steps := []struct {
					name    string
					fn      func() error
					step    types.ServerStep
					require types.ServerStep
				}{
					{
						name:    "Verifying machine requirements",
						fn:      im.VerifyMachineRequirement,
						step:    types.StepVerified,
						require: types.StepInitialized,
					},
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
						fn:      func() error { return im.StartRunner(*licenseKey, *instances) },
						step:    types.StepRunnerStarted,
						require: types.StepMonitoringSetup,
					},
				}

				stepOrder := map[types.ServerStep]int{
					types.StepInitialized:     0,
					types.StepVerified:        1,
					types.StepBaseInstalled:   2,
					types.StepConsulSetup:     3,
					types.StepNomadSetup:      4,
					types.StepMonitoringSetup: 5,
					types.StepRunnerStarted:   6,
					types.StepCompleted:       7,
				}

				currentStepOrder := stepOrder[currentStep]

				for _, step := range steps {
					select {
					case <-ctx.Done():
						return
					default:
						requiredStepOrder := stepOrder[step.require]
						currentLoopStepOrder := stepOrder[step.step]

						if currentStepOrder < currentLoopStepOrder && currentStepOrder >= requiredStepOrder {
							spinner.Start(step.name)
							if err := step.fn(); err != nil {
								setupFailed = true
								spinner.Stop(false)
								errorChan <- fmt.Errorf("error during (%s) on %s: %v", step.name, server.Host, err)
								cancel()
								return
							}
							currentStep = step.step
							currentStepOrder = stepOrder[currentStep]
							if err := database.UpdateServerStep(machineID, step.step); err != nil {
								setupFailed = true
								spinner.Stop(false)
								log.Printf("Error updating step for server (%s): %v", server.Host, err)
								return
							}
							spinner.Stop(true)
						} else if currentStepOrder >= currentLoopStepOrder {
						} else {
							return
						}
					}
				}

				if err := database.UpdateServerStep(machineID, types.StepCompleted); err != nil {
					log.Printf("Error marking server (%s) as completed: %v", server.Host, err)
				}
			}
		}(server)
	}

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
