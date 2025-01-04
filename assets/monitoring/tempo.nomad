job "tempo" {
  datacenters = ["dc1"]
  type        = "service"

  group "tempo" {
    count = 1

    network {
      port "tempo" {
        static = 3200
      }
      port "tempo_read" {
        static = 3201
      }
    }

    service {
      name = "tempo"
      port = "tempo"
      
      tags = ["tempo", "traces"]

      check {
        name     = "tempo port alive"
        type     = "tcp"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "tempo" {
      driver = "docker"

      config {
        image = "grafana/tempo:latest"
        ports = ["tempo", "tempo_read"]

        args = [
          "-config.file=/etc/tempo/config.yml"
        ]
      }

      template {
        data = <<EOH
server:
  http_listen_port: 3200

distributor:
  receivers:
    jaeger:
      protocols:
        thrift_http:
          endpoint: "0.0.0.0:14268"

ingester:
  trace_idle_period: 10s
  max_block_bytes: 1_000_000
  max_block_duration: 5m

compactor:
  compaction:
    compaction_window: 1h
    max_block_bytes: 100_000_000
    block_retention: 1h
    compacted_block_retention: 10m

storage:
  trace:
    backend: local
    local:
      path: /tmp/tempo/blocks
    pool:
      max_workers: 100
      queue_depth: 10000

overrides:
  per_tenant_override_config: {}
EOH

        destination = "local/config.yml"
      }

      resources {
        cpu    = 500
        memory = 512
      }
    }
  }
}