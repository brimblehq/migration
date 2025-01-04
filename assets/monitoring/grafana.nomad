job "grafana" {
  datacenters = ["dc1"]
  type        = "service"

  group "grafana" {
    count = 1

    network {
      port "grafana_ui" {
        to = 3000
      }
    }

    service {
      name = "grafana"
      port = "grafana_ui"
      
      tags = ["grafana", "ui"]
      

      check {
        name     = "grafana_ui port alive"
        type     = "tcp"
        interval = "10s"
        timeout  = "2s"
        check_restart {
          limit = 3
          grace = "180s"
          ignore_warnings = false
        }
      }
    }

    task "grafana" {
      driver = "docker"

      config {
        image = "grafana/grafana:latest"
        ports = ["grafana_ui"]
      }

      template {
        data = <<EOH
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://{{ range service "prometheus" }}{{ .Address }}:{{ .Port }}{{ end }}
    isDefault: true

  - name: Loki
    type: loki
    access: proxy
    url: http://{{ range service "loki" }}{{ .Address }}:{{ .Port }}{{ end }}
    isDefault: false
EOH

        destination = "local/datasources.yaml"
      }

      env {
        GF_AUTH_ANONYMOUS_ENABLED  = "true"
        GF_AUTH_ANONYMOUS_ORG_ROLE = "Admin"
      }

      resources {
        cpu    = 200
        memory = 256
      }
    }
  }
}