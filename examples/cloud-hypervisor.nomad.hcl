# Example Nomad job using Cloud Hypervisor driver
# This demonstrates the new features including:
# - Custom kernel/initramfs
# - GPU device passthrough
# - Virtio-fs mounts
# - Network port mapping
# - vsock communication

job "microvm-workload" {
  datacenters = ["dc1"]
  type        = "service"

  group "compute" {
    count = 1

    # Allocate resources
    task "vm" {
      driver = "virt"

      # Basic VM configuration
      config {
        # Root filesystem image
        image = "/root/headless-shell.qcow2"
        primary_disk_size = 10240  # 10GB

        # Use custom kernel and initramfs for faster boot
        kernel   = "/root/vmlinux-normal"
        initramfs = "/root/raiin-fc.cpio.gz"
        cmdline  = "console=hvc0 root=/dev/vda1 rw init=/sbin/init"

        # VM sizing and performance options
        max_vcpus         = 4
        memory_hugepages  = true
        memory_shared     = true   # Required for virtio-fs
        hotplug_method    = "virtio-mem"
        hotplug_size      = "4G"

        # VM hostname
        hostname = "microvm-worker"

        # Cloud-init user configuration
        default_user_authorized_ssh_key = "ssh-rsa AAAAB3NzaC1yc2E... user@example.com"
        default_user_password = "changeme"

        # Custom commands to run in VM
        cmds = [
          "systemctl enable docker",
          "systemctl start docker",
        ]

        # Network configuration
        network_interface {
          bridge {
            name  = "br0"
            ports = ["http", "metrics"]
          }
        }

        # GPU passthrough example (uncomment if GPU available)
        # devices {
        #   path  = "/sys/bus/pci/devices/0000:01:00.0"
        #   id    = "gpu0"
        #   iommu = true
        # }

        # Additional disk for data
        disks {
          path     = "/var/lib/nomad/data/disk.qcow2"
          readonly = false
          serial   = "data-disk"
        }

        # Virtio-fs shared directories
        fs_mounts {
          tag         = "shared"
          source      = "/opt/shared"
          destination = "/mnt/shared"
          num_queues  = 2
          queue_size  = 1024
        }

        # Vsock for host-guest communication
        vsock {
          cid    = 3
          socket = "/tmp/vsock.sock"
        }

        # Random number generator
        rng {
          src = "/dev/urandom"
        }

        # Platform configuration for GPU/IOMMU
        platform {
          num_pci_segments      = 2
          iommu_segments        = [0, 1]
          iommu_address_width   = 48
        }
      }

      # Resource allocation
      resources {
        cpu    = 2000  # 2 CPU cores
        memory = 4096  # 4GB RAM

        # Network ports
        network {
          port "http" {
            static = 8080
            to     = 80
          }
          port "metrics" {
            static = 9090
            to     = 9090
          }
        }
      }

      # Service registration
      service {
        name = "microvm-app"
        port = "http"

        check {
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "3s"
        }
      }

      # Environment variables passed to VM
      env {
        APP_ENV = "production"
        LOG_LEVEL = "info"
      }

      # Artifacts to download before starting
      artifact {
        source      = "https://releases.example.com/app.tar.gz"
        destination = "local/"
        mode        = "file"
      }
    }
  }
}