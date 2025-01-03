package main

import (
	"flag"
	"io/ioutil"
	"log"
	"sync"

	"github.com/brimblehq/migration/internal/types"
	"github.com/brimblehq/migration/internal/ui"

	"github.com/brimblehq/migration/internal/license"
	"github.com/brimblehq/migration/internal/manager"
	"github.com/brimblehq/migration/internal/ssh"

	"gopkg.in/yaml.v2"
)

func main() {
	licenseKey := flag.String("license-key", "", "License key for runner")

	flag.Parse()

	if *licenseKey == "" {
		log.Fatal("License key is required")
	}

	licenseResp, err := license.ValidateLicenseKey(*licenseKey)

	if err != nil || !licenseResp.Valid {
		log.Fatal("Invalid license key")
	}

	data, err := ioutil.ReadFile("nomad_config.yaml")

	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config types.Config

	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	clusterRoles := manager.NewClusterRoles(config.Servers)

	clusterRoles.CalculateRoles(config.Servers)

	var wg sync.WaitGroup

	for _, server := range config.Servers {

		wg.Add(1)

		go func(server types.Server) {
			defer wg.Done()

			spinner := ui.NewStepSpinner(server.Host)

			client, err := ssh.NewSSHClient(server)
			if err != nil {
				spinner.Start("Connecting to server")
				spinner.Stop(false)
				log.Printf("Error connecting to %s: %v", server.Host, err)
				return
			}
			defer client.Close()

			roles := clusterRoles.RoleMapping[server.Host]
			im := manager.NewInstallationManager(client, server, roles, &config)

			steps := []struct {
				name string
				fn   func() error
			}{
				{"Installing base packages", im.InstallBasePackages},
				{"Setting up Consul client", im.SetupConsulClient},
				{"Setting up Nomad", im.SetupNomad},
				{"Setting up monitoring", im.SetupMonitoring},
				{"Starting runner", func() error { return im.StartRunner(*licenseKey) }},
			}

			for _, step := range steps {
				spinner.Start(step.name)
				if err := step.fn(); err != nil {
					spinner.Stop(false)
					log.Printf("Error during %s on %s: %v", step.name, server.Host, err)
					return
				}
				spinner.Stop(true)
			}
		}(server)
	}

	wg.Wait()
	log.Println("Infrastructure setup completed")
}
