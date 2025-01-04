job "promtail" {
  datacenters = ["dc1"]
  type        = "system"

  group "promtail" {
    network {
      port "promtail" {
        static = 9080
      }
    }

    service {
      name = "promtail"
      port = "promtail"
      
      tags = ["promtail", "logs"]

      check {
        name     = "promtail port alive"
        type     = "tcp"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "promtail" {
      driver = "docker"

      config {
        image = "grafana/promtail:latest"
        ports = ["promtail"]

        volumes = [
          "/var/log:/var/log",
          "/var/lib/docker/containers:/var/lib/docker/containers"
        ]

        args = [
          "-config.file=/etc/promtail/config.yml"
        ]
      }

      template {
        data = <<EOH
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://{{ range service "loki" }}{{ .Address }}:{{ .Port }}{{ end }}/loki/api/v1/push

scrape_configs:
  - job_name: system
    static_configs:
      - targets:
          - localhost
        labels:
          job: varlogs
          __path__: /var/log/*log

  - job_name: docker
    static_configs:
      - targets:
          - localhost
        labels:
          job: docker
          host: {{ env "node.unique.name" }}
          __path__: /var/lib/docker/containers/*/*-json.log
EOH

        destination = "local/config.yml"
      }

      resources {
        cpu    = 200
        memory = 256
      }
    }
  }
}