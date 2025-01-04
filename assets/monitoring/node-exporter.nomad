job "node-exporter" {
  datacenters = ["dc1"]
  type        = "system"

  group "node-exporter" {
    network {
      port "node_exporter" {
        static = 9100
      }
    }

    service {
      name = "node-exporter"
      port = "node_exporter"
      
      tags = ["node-exporter", "metrics"]

      check {
        name     = "node_exporter port alive"
        type     = "tcp"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "node-exporter" {
      driver = "docker"

      config {
        image = "prom/node-exporter:latest"
        ports = ["node_exporter"]

        args = [
          "--path.rootfs=/host"
        ]

        volumes = [
          "/:/host:ro,rslave"
        ]
      }

      resources {
        cpu    = 100
        memory = 128
      }
    }
  }
}