job "cadvisor" {
  datacenters = ["dc1"]
  type = "system"
  group "cadvisor" {
    count = 1
    network {
      port "metrics" {
        to = 8080
      }
    }
    task "cadvisor" {
      driver = "docker"
      config {
        image = "gcr.io/cadvisor/cadvisor:v0.47.0"
        ports = ["metrics"]
        volumes = [
          "/:/rootfs:ro",
          "/var/run:/var/run:ro",
          "/sys:/sys:ro",
          "/var/lib/docker/:/var/lib/docker:ro",
          "/dev/disk/:/dev/disk:ro",
          "/dev/kmsg:/dev/kmsg",
          "/etc/machine-id:/etc/machine-id:ro"
        ]
        devices = [
          {
            host_path = "/dev/kmsg"
            container_path = "/dev/kmsg"
          }
        ]
        ulimit {
          nofile = "262144:262144"
        }
        privileged = true
        args = [
          "--machine_id_file=/etc/machine-id",
          "--docker_only=true",
          "--housekeeping_interval=10s"
        ]
      }
      template {
        data = <<EOH
#!/bin/sh
if [ ! -f /etc/machine-id ]; then
  echo "Generating a temporary machine-id"
  echo "temporary-machine-id" > /etc/machine-id
fi
exec /usr/bin/cadvisor $@
EOH
        destination = "local/wrapper.sh"
        perms = "755"
      }
      resources {
        cpu    = 200
        memory = 256
      }
      service {
        name = "cadvisor"
        port = "metrics"
        check {
          type     = "http"
          path     = "/healthz"
          interval = "10s"
          timeout  = "2s"
          port     = "metrics"
        }
      }
    }
  }
}