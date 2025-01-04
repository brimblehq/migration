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

	licenseResp, err := license.ValidateLicenseKey(*licenseKey)

	if err != nil || !licenseResp.Valid {
		log.Fatal("Invalid license key")
	}

	var config types.Config

	if err := json.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	log.Printf("\n=== Configuration Details ===")
	log.Printf("Servers:")
	for i, server := range config.Servers {
		log.Printf("  Server %d:", i+1)
		log.Printf("    Host: %s", server.Host)
		log.Printf("    Username: %s", server.Username)
		log.Printf("    KeyPath: %s", server.KeyPath)
		log.Printf("    Datacenter: %s", server.DataCenter)
		log.Printf("    Public IP: %s", server.PublicIP)
		log.Printf("    Private IP: %s", server.PrivateIP)
	}

	log.Printf("\nCluster Config:")
	log.Printf("  Consul:")
	log.Printf("    Server Address: %s", config.ClusterConfig.ConsulConfig.ServerAddress)
	log.Printf("    Token: %s", config.ClusterConfig.ConsulConfig.Token)
	log.Printf("    Datacenter: %s", config.ClusterConfig.ConsulConfig.DataCenter)
	log.Printf("    Consul Image: %s", config.ClusterConfig.ConsulConfig.ConsulImage)
	log.Printf("\n========================")

	if licenseResp.DbConnectionUrl == "" || strings.TrimSpace(licenseResp.DbConnectionUrl) == "" {
		log.Fatal("Unable to setup this installation")
	}

	database, err := db.NewPostgresDB(db.Config{
		URI: licenseResp.DbConnectionUrl,
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

	existingIPs := make(map[string]bool)

	for _, srv := range existingServers {
		server := types.Server{
			Host:      srv.MachineID,
			PublicIP:  srv.PublicIP,
			PrivateIP: srv.PrivateIP,
		}
		allServers = append(allServers, server)
		existingIPs[srv.PrivateIP] = true
	}

	for _, configServer := range config.Servers {
		if existingIPs[configServer.PrivateIP] {
			log.Printf("Warning: Server with IP %s already exists in database, skipping", configServer.PrivateIP)
			continue
		}
		allServers = append(allServers, configServer)
		existingIPs[configServer.PrivateIP] = true
	}

	clusterRoles := manager.NewClusterRoles(allServers)

	clusterRoles.CalculateRoles(config.Servers)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	errorChan := make(chan error)

	var wg sync.WaitGroup

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

				fmt.Printf("ssh clients: %v", client)

				if err != nil {
					spinner.Start("Connecting to server")
					spinner.Stop(false)
					log.Printf("Error connecting to %s: %v", server.Host, err)
					return
				}

				defer client.Close()

				roles := clusterRoles.RoleMapping[server.Host]

				im := manager.NewInstallationManager(client, server, roles, &config, licenseResp.TailScaleToken, database)

				spinner.Start("Getting machine ID")

				machineID, err := client.ExecuteCommandWithOutput("cat /etc/machine-id")

				if err != nil {
					spinner.Stop(false)
					log.Printf("Error getting machine-id from %s: %v", server.Host, err)
					return
				}

				spinner.Stop(true)

				spinner.Start("Checking server status")

				//check if server has been registered, maybe from previous setup
				isRegistered, err := database.IsServerRegistered(strings.TrimSpace(machineID))

				if err != nil {
					spinner.Stop(false)
					log.Printf("Error checking server registration for %s: %v", server.Host, err)
				}

				if isRegistered {
					spinner.Stop(true)
					log.Printf("Server %s is already configured, skipping setup", server.Host)
					return
				}

				spinner.Stop(true)

				log.Printf("Setting up new server: %s", server.Host)

				steps := []struct {
					name string
					fn   func() error
				}{
					// {"Installing base packages ", im.InstallBasePackages},
					// {"Setting up Consul client", im.SetupConsulClient},
					{"Setting up Nomad", im.SetupNomad},
					// {"Setting up monitoring", im.SetupMonitoring},
					// {"Starting runner", func() error { return im.StartRunner(*licenseKey) }},
				}

				for _, step := range steps {
					select {
					case <-ctx.Done():
						return
					default:
						spinner.Start(step.name)
						if err := step.fn(); err != nil {
							spinner.Stop(false)
							errorChan <- fmt.Errorf("error during %s on %s: %v", step.name, server.Host, err)
							cancel() // Signal other goroutines to stop
							return
						}
						spinner.Stop(true)
					}
				}

				spinner.Start("Registering server")

				role := "client"

				if len(roles) > 1 {
					role = "both"
				}

				err = database.RegisterServer(
					strings.TrimSpace(machineID),
					server.PublicIP,
					server.PrivateIP,
					role,
				)

				if err != nil {
					spinner.Stop(false)
					log.Printf("Error registering server %s: %v", server.Host, err)
					return
				}
				spinner.Stop(true)

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
			os.Exit(1)
		}
	}

	log.Println("Infrastructure setup completed ✅")
}
