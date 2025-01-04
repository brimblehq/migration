job "loki" {
  datacenters = ["dc1"]
  type        = "service"

  group "loki" {
    count = 1

    network {
      port "loki" {
        static = 3100
      }
    }

    service {
      name = "loki"
      port = "loki"
      
      tags = ["loki", "logs"]

      check {
        name     = "loki port alive"
        type     = "tcp"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "loki" {
      driver = "docker"

      config {
        image = "grafana/loki:latest"
        ports = ["loki"]

        args = [
          "-config.file=/etc/loki/local-config.yaml"
        ]
      }

      template {
        data = <<EOH
auth_enabled: false

server:
  http_listen_port: 3100

ingester:
  lifecycler:
    address: 127.0.0.1
    ring:
      kvstore:
        store: inmemory
      replication_factor: 1
    final_sleep: 0s
  chunk_idle_period: 5m
  chunk_retain_period: 30s

schema_config:
  configs:
    - from: 2020-05-15
      store: boltdb
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 168h

storage_config:
  boltdb:
    directory: /tmp/loki/index

  filesystem:
    directory: /tmp/loki/chunks

limits_config:
  enforce_metric_name: false
  reject_old_samples: true
  reject_old_samples_max_age: 168h
EOH

        destination = "local/config.yaml"
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }
  }
}