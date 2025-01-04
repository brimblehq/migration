job "prometheus" {
  datacenters = ["dc1"]
  type        = "service"

  group "prometheus" {
    count = 1

    network {
      port "prometheus_ui" {
        static = 9090
      }
    }

    service {
      name = "prometheus"
      port = "prometheus_ui"
      
      tags = ["prometheus", "metrics"]

      check {
        name     = "prometheus_ui port alive"
        type     = "tcp"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "prometheus" {
      driver = "docker"

      env {
        CONSUL_HTTP_TOKEN = "14f1f8a7-08cb-20d5-7ee3-621624225a68"
        CONSUL_HTTP_SERVER = "100.64.13.94:8500"
      }

      config {
        image = "prom/prometheus:latest"
        ports = ["prometheus_ui"]

        args = [
          "--config.file=/etc/prometheus/prometheus.yml",
          "--storage.tsdb.path=/prometheus",
          "--storage.tsdb.retention.time=15d",
          "--web.console.libraries=/usr/share/prometheus/console_libraries",
          "--web.console.templates=/usr/share/prometheus/consoles"
        ]

        volumes = [
          "local/prometheus.yml:/etc/prometheus/prometheus.yml"
        ]
      }

      template {
        data = <<EOH
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nomad_metrics'
    metrics_path: '/v1/metrics'
    params:
      format: ['prometheus']
    consul_sd_configs:
      - server: "{{ env "CONSUL_HTTP_SERVER" }}"
        token: "{{ env "CONSUL_HTTP_TOKEN" }}"
        services: ['nomad-client', 'nomad']
    relabel_configs:
      - source_labels: ['__meta_consul_tags']
        regex: '(.*)http(.*)'
        action: keep

  - job_name: 'node-exporter'
    metrics_path: '/metrics'
    scheme: http
    consul_sd_configs:
      - server: "{{ env "CONSUL_HTTP_SERVER" }}"
        token: "{{ env "CONSUL_HTTP_TOKEN" }}"
        services: ['node-exporter']
    relabel_configs:
      - source_labels: [__meta_consul_tags]
        regex: .*
        action: keep
      - source_labels: [__meta_consul_node]
        target_label: node
      - source_labels: [__meta_consul_service_metadata_external_source]
        regex: nomad
        action: keep

  - job_name: 'cadvisor'
    metrics_path: '/metrics'
    scheme: http
    consul_sd_configs:
      - server: "{{ env "CONSUL_HTTP_SERVER" }}"
        token: "{{ env "CONSUL_HTTP_TOKEN" }}"
        services: ['cadvisor']
    relabel_configs:
      - source_labels: [__meta_consul_tags]
        regex: .*
        action: keep
      - source_labels: [__meta_consul_service_metadata_external_source]
        regex: nomad
        action: keep
      - source_labels: [__meta_consul_node]
        target_label: node
      - source_labels: [__meta_consul_address]
        target_label: address
        regex: (.+)
        replacement: ${1}:8889
EOH

        destination = "local/prometheus.yml"
      }

      resources {
        cpu    = 500
        memory = 512
      }
    }
  }
}