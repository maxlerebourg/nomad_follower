job "log-shipping" {
  datacenters = ["dc1"]
  type        = "system"
  namespace   = "logs"
  update {
    max_parallel      = 1
    min_healthy_time  = "10s"
    healthy_deadline  = "3m"
    progress_deadline = "10m"
    auto_revert       = false
  }

  group "log-shipping" {
    count = 1

    network {
      port "promtail-healthcheck" {
        to = 3000
      }
    }

    restart {
      attempts = 2
      interval = "30m"
      delay    = "15s"
      mode     = "fail"
    }

    ephemeral_disk {
      size = 300
    }


    task "nomad-forwarder" {
      driver = "docker"

      env {
        VERBOSE    = 4
        LOG_META    = "logging"
        LOG_FILE   = "${NOMAD_ALLOC_DIR}/nomad-logs.log"
        NOMAD_ADDR = "http://172.17.0.1:4646"
      }

      config {
        image = "sofixa/nomad_follower:latest"
      }

      resources {
        cpu    = 100
        memory = 512
      }
    }
  }
}
