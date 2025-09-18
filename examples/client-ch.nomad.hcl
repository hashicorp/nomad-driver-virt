# Nomad client configuration for Cloud Hypervisor driver
# Place this in /etc/nomad.d/client.hcl on the Nomad client node

datacenter = "dc1"
name       = "nomad-client-ch"
data_dir   = "/opt/nomad/data"

bind_addr = "0.0.0.0"

# Client configuration
client {
  enabled = true
  servers = ["nomad-server:4647"]

  # Node metadata
  meta {
    "node.class" = "microvm"
    "gpu.available" = "false"  # Set to true if GPU available
  }

  # Host volumes for shared data
  host_volume "shared-data" {
    path      = "/opt/shared"
    read_only = false
  }
}

# Plugin configuration for Cloud Hypervisor
plugin "nomad-driver-virt" {
  config {
    # Cloud Hypervisor binary paths
    cloud_hypervisor {
      bin           = "/usr/bin/cloud-hypervisor"
      remote_bin    = "/usr/bin/ch-remote"
      virtiofsd_bin = "/usr/lib/virtiofsd"

      # Default kernel/initramfs (can be overridden per task)
      default_kernel    = "/opt/kernels/vmlinux"
      default_initramfs = "/opt/kernels/initrd"

      # Optional firmware for UEFI boot
      # firmware = "/opt/firmware/CLOUDHV.fd"

      # Security settings
      seccomp = "true"

      # Log file (empty = stderr)
      log_file = "/var/log/cloud-hypervisor.log"
    }

    # Network configuration
    network {
      bridge        = "br0"
      subnet_cidr   = "194.31.143.0/24"
      gateway       = "194.31.143.1"
      ip_pool_start = "194.31.143.100"
      ip_pool_end   = "194.31.143.200"
      tap_prefix    = "tap-"
    }

    # VFIO/GPU configuration
    vfio {
      # Allowlist of PCI devices that can be passed through
      allowlist = [
        "0000:01:00.0",  # Example GPU
        "0000:02:00.0",  # Example NIC
      ]

      # IOMMU configuration
      iommu_address_width = 48
      pci_segments       = 1
    }

    # Data directory for VM working files
    data_dir = "/var/lib/nomad-ch"

    # Allowed paths for VM images
    image_paths = [
      "/var/lib/images",
      "/opt/vm-images",
      "/tmp/nomad-images"
    ]
  }
}

# Logging
log_level = "INFO"
log_json  = true
enable_debug = false