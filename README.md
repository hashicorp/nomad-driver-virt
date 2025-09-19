# Nomad Cloud Hypervisor Driver

**Production-ready Nomad task driver for Intel Cloud Hypervisor virtual machines**

[![Build Status](https://badge.buildkite.com/your-build-badge)](https://buildkite.com/hypr/nomad-driver-ch)
[![License](https://img.shields.io/badge/license-MPL%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/ccheshirecat/nomad-driver-ch)](https://goreportcard.com/report/github.com/ccheshirecat/nomad-driver-ch)

The `nomad-driver-ch` is a task driver for [HashiCorp Nomad](https://www.nomadproject.io/) that enables orchestration of [Intel Cloud Hypervisor](https://www.cloudhypervisor.org/) virtual machines. This driver provides a modern, lightweight alternative to traditional hypervisor solutions while maintaining full compatibility with Nomad's scheduling and resource management capabilities.

## 🚀 Key Features

- **🏃‍♂️ Lightweight Virtualization**: Leverages Intel Cloud Hypervisor for minimal overhead VM orchestration
- **🔧 Dynamic Resource Management**: CPU, memory, and disk allocation with Nomad's resource constraints
- **🌐 Advanced Networking**: Bridge networking with static IP support and dynamic configuration
- **☁️ Cloud-Init Integration**: Automatic VM provisioning with user data, SSH keys, and custom scripts
- **💾 Flexible Storage**: Virtio-fs shared filesystems and disk image management with thin provisioning
- **🚧 PCIe Device Passthrough**: VFIO support coming very soon - high priority feature in active development
- **🔒 Security Isolation**: Secure VM boundaries with configurable seccomp filtering
- **📊 Resource Monitoring**: Real-time VM statistics and health monitoring
- **🔄 Lifecycle Management**: Complete VM lifecycle with start, stop, restart, and recovery capabilities

## 📋 Table of Contents

- [Quick Start](#-quick-start)
- [Installation](#-installation)
- [Configuration](#️-configuration)
- [Task Examples](#-task-examples)
- [Networking](#-networking)
- [Cloud-Init](#️-cloud-init)
- [Storage](#-storage)
- [Device Passthrough](#-device-passthrough)
- [Monitoring](#-monitoring)
- [Troubleshooting](#-troubleshooting)
- [API Reference](#-api-reference)
- [Development](#-development)
- [Contributing](#-contributing)

## 🚀 Quick Start

### Prerequisites

- **Nomad** v1.4.0 or later
- **Cloud Hypervisor** v48.0.0 or later
- **Linux kernel** with KVM support
- **Bridge networking** configured on host

### Basic Example

```hcl
job "web-server" {
  datacenters = ["dc1"]
  type = "service"

  group "web" {
    task "nginx" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/alpine-nginx.img"

        network_interface {
          bridge {
            name = "br0"
            static_ip = "192.168.1.100"
          }
        }
      }

      resources {
        cpu    = 1000
        memory = 512
      }
    }
  }
}
```## 📦 Installation

### 1. Install Dependencies

**Cloud Hypervisor:**
```bash
# Download and install Cloud Hypervisor v48+
wget https://github.com/cloud-hypervisor/cloud-hypervisor/releases/download/v48.0.0/cloud-hypervisor-static
sudo mv cloud-hypervisor-static /usr/bin/cloud-hypervisor
sudo chmod +x /usr/bin/cloud-hypervisor

# Install ch-remote for VM management
wget https://github.com/cloud-hypervisor/cloud-hypervisor/releases/download/v48.0.0/ch-remote-static
sudo mv ch-remote-static /usr/bin/ch-remote
sudo chmod +x /usr/bin/ch-remote
```

**VirtioFS daemon:**
```bash
# Install virtiofsd for filesystem sharing
sudo apt-get install virtiofsd  # Ubuntu/Debian
# or
sudo yum install virtiofsd      # RHEL/CentOS
```

### 2. Configure Bridge Networking

```bash
# Create bridge interface
sudo ip link add br0 type bridge
sudo ip addr add 192.168.1.1/24 dev br0
sudo ip link set br0 up

# Configure bridge persistence (systemd-networkd)
cat > /etc/systemd/network/br0.netdev << EOF
[NetDev]
Name=br0
Kind=bridge
EOF

cat > /etc/systemd/network/br0.network << EOF
[Match]
Name=br0

[Network]
IPForward=yes
Address=192.168.1.1/24
EOF

sudo systemctl restart systemd-networkd
```

### 3. Install Driver Plugin

**Option A: Download Release**
```bash
# Download latest release
wget https://github.com/ccheshirecat/nomad-driver-ch/releases/latest/download/nomad-driver-ch
sudo mv nomad-driver-ch /opt/nomad/plugins/
sudo chmod +x /opt/nomad/plugins/nomad-driver-ch
```

**Option B: Build from Source**
```bash
git clone https://github.com/ccheshirecat/nomad-driver-ch.git
cd nomad-driver-ch
go build -o nomad-driver-ch .
sudo mv nomad-driver-ch /opt/nomad/plugins/
```

### 4. Configure Nomad

**Client Configuration:**
```hcl
# /etc/nomad.d/client.hcl
client {
  enabled = true

  plugin "nomad-driver-ch" {
    config {
      # Cloud Hypervisor configuration
      cloud_hypervisor {
        bin = "/usr/bin/cloud-hypervisor"
        remote_bin = "/usr/bin/ch-remote"
        virtiofsd_bin = "/usr/bin/virtiofsd"
        default_kernel = "/boot/vmlinuz"
        default_initramfs = "/boot/initramfs.img"
      }

      # Network configuration
      network {
        bridge = "br0"
        subnet_cidr = "192.168.1.0/24"
        gateway = "192.168.1.1"
        ip_pool_start = "192.168.1.100"
        ip_pool_end = "192.168.1.200"
      }

      # Allowed image paths for security
      image_paths = ["/var/lib/images", "/opt/vm-images"]
    }
  }
}
```

### 5. Start Nomad

```bash
sudo systemctl restart nomad
```

Verify the driver is loaded:
```bash
nomad node status -self | grep nomad-driver-ch
```

For detailed installation instructions, see [docs/INSTALLATION.md](docs/INSTALLATION.md).

## ⚙️ Configuration

### Driver Configuration

The driver configuration is specified in the Nomad client configuration file:

```hcl
plugin "nomad-driver-ch" {
  config {
    # Cloud Hypervisor binaries
    cloud_hypervisor {
      bin = "/usr/bin/cloud-hypervisor"           # Cloud Hypervisor binary path
      remote_bin = "/usr/bin/ch-remote"           # ch-remote binary path
      virtiofsd_bin = "/usr/bin/virtiofsd"        # virtiofsd binary path
      default_kernel = "/boot/vmlinuz"            # Default kernel for VMs
      default_initramfs = "/boot/initramfs.img"   # Default initramfs for VMs
      firmware = "/usr/share/qemu/OVMF.fd"        # UEFI firmware (optional)
      seccomp = "true"                            # Enable seccomp filtering
      log_file = "/var/log/cloud-hypervisor.log" # VM log file path
    }

    # Network configuration
    network {
      bridge = "br0"                      # Bridge interface name
      subnet_cidr = "192.168.1.0/24"      # Subnet for VMs
      gateway = "192.168.1.1"             # Gateway IP address
      ip_pool_start = "192.168.1.100"     # IP pool start range
      ip_pool_end = "192.168.1.200"       # IP pool end range
      tap_prefix = "tap"                  # TAP interface prefix
    }

    # VFIO device passthrough (not yet implemented)
    # vfio {
    #   allowlist = ["10de:*", "8086:0d26"]  # PCI device allowlist
    #   iommu_address_width = 48              # IOMMU address width
    #   pci_segments = 1                      # Number of PCI segments
    # }

    # Security and paths
    data_dir = "/opt/nomad/data"              # Nomad data directory
    image_paths = [                           # Allowed image paths
      "/var/lib/images",
      "/opt/vm-images",
      "/mnt/shared-storage"
    ]
  }
}
```

### Configuration Reference

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cloud_hypervisor.bin` | string | `/usr/bin/cloud-hypervisor` | Path to Cloud Hypervisor binary |
| `cloud_hypervisor.remote_bin` | string | `/usr/bin/ch-remote` | Path to ch-remote binary |
| `cloud_hypervisor.virtiofsd_bin` | string | `/usr/bin/virtiofsd` | Path to virtiofsd binary |
| `cloud_hypervisor.default_kernel` | string | - | Default kernel path for VMs |
| `cloud_hypervisor.default_initramfs` | string | - | Default initramfs path for VMs |
| `cloud_hypervisor.firmware` | string | - | UEFI firmware path (optional) |
| `cloud_hypervisor.seccomp` | string | `"true"` | Enable seccomp filtering |
| `cloud_hypervisor.log_file` | string | - | VM log file path |
| `network.bridge` | string | `"br0"` | Bridge interface name |
| `network.subnet_cidr` | string | `"192.168.1.0/24"` | VM subnet CIDR |
| `network.gateway` | string | `"192.168.1.1"` | Network gateway |
| `network.ip_pool_start` | string | `"192.168.1.100"` | IP allocation pool start |
| `network.ip_pool_end` | string | `"192.168.1.200"` | IP allocation pool end |
| `network.tap_prefix` | string | `"tap"` | TAP interface name prefix |
| `vfio.allowlist` | []string | - | ⚠️ Not implemented yet |
| `vfio.iommu_address_width` | number | - | ⚠️ Not implemented yet |
| `vfio.pci_segments` | number | - | ⚠️ Not implemented yet |
| `data_dir` | string | - | Nomad data directory |
| `image_paths` | []string | - | Allowed VM image paths |

For complete configuration details, see [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## 📝 Task Examples

### Basic VM Task

```hcl
job "basic-vm" {
  datacenters = ["dc1"]

  group "app" {
    task "vm" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/ubuntu-22.04.img"
        hostname = "app-server"

        # Custom kernel and initramfs
        kernel = "/boot/vmlinuz-5.15.0"
        initramfs = "/boot/initramfs-5.15.0.img"
        cmdline = "console=ttyS0 root=/dev/vda1"
      }

      resources {
        cpu    = 2000  # 2 CPU cores
        memory = 2048  # 2GB RAM
      }
    }
  }
}
```

### VM with Custom User Data

```hcl
job "custom-vm" {
  datacenters = ["dc1"]

  group "web" {
    task "nginx" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/alpine.img"
        hostname = "nginx-server"

        # Cloud-init user data
        user_data = "/etc/cloud-init/nginx-setup.yml"

        # Default user configuration
        default_user_password = "secure123"
        default_user_authorized_ssh_key = "ssh-rsa AAAAB3NzaC1yc2E..."

        # Custom commands to run
        cmds = [
          "apk add --no-cache nginx",
          "rc-service nginx start",
          "rc-update add nginx default"
        ]
      }

      resources {
        cpu    = 1000
        memory = 512
      }
    }
  }
}
```

### VM with Storage and Networking

```hcl
job "database" {
  datacenters = ["dc1"]

  group "db" {
    task "postgres" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/postgres-14.img"
        hostname = "postgres-primary"

        # Enable thin copy for faster startup
        use_thin_copy = true
        primary_disk_size = 20480  # 20GB

        # Network configuration with static IP
        network_interface {
          bridge {
            name = "br0"
            static_ip = "192.168.1.50"
            gateway = "192.168.1.1"
            netmask = "24"
            dns = ["8.8.8.8", "1.1.1.1"]
          }
        }

        # Custom timezone
        timezone = "America/New_York"
      }

      resources {
        cpu    = 4000  # 4 CPU cores
        memory = 8192  # 8GB RAM
      }

      # Mount shared storage
      volume_mount {
        volume      = "postgres-data"
        destination = "/var/lib/postgresql"
      }
    }
  }

  volume "postgres-data" {
    type      = "host"
    source    = "postgres-data"
    read_only = false
  }
}
```

### GPU-Accelerated VM

```hcl
job "ml-workload" {
  datacenters = ["dc1"]

  group "gpu" {
    task "training" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/cuda-ubuntu.img"
        hostname = "ml-trainer"

        # VFIO GPU passthrough (not yet implemented)
        # vfio_devices = ["10de:2204"]  # NVIDIA RTX 3080
      }

      resources {
        cpu      = 8000  # 8 CPU cores
        memory   = 16384 # 16GB RAM
        device "nvidia/gpu" {
          count = 1
        }
      }
    }
  }
}
```

For more examples, see [docs/EXAMPLES.md](docs/EXAMPLES.md).

## 🌐 Networking

### Bridge Networking

The driver supports bridge networking with automatic IP allocation or static IP assignment:

#### Automatic IP Allocation

```hcl
config {
  network_interface {
    bridge {
      name = "br0"
      # IP will be allocated from pool automatically
    }
  }
}
```

#### Static IP Assignment

```hcl
config {
  network_interface {
    bridge {
      name = "br0"
      static_ip = "192.168.1.100"
      gateway = "192.168.1.1"
      netmask = "24"
      dns = ["8.8.8.8", "1.1.1.1"]
    }
  }
}
```

### Network Configuration Priority

The driver uses a hierarchical configuration approach:

1. **Task-Level Configuration** (highest priority)
   - `static_ip`, `gateway`, `netmask`, `dns` from task config

2. **Driver-Level Configuration** (medium priority)
   - IP pool allocation, default gateway, subnet settings

3. **DHCP Fallback** (lowest priority)
   - When no static configuration is provided

### Port Mapping

Map container ports to host ports:

```hcl
config {
  network_interface {
    bridge {
      name = "br0"
      ports = ["web", "api"]  # Reference port labels from network block
    }
  }
}

network {
  port "web" {
    static = 80
  }
  port "api" {
    static = 8080
  }
}
```

## ☁️ Cloud-Init

The driver integrates with cloud-init for automated VM provisioning and configuration.

### User Data Sources

#### File-Based User Data

```hcl
config {
  user_data = "/etc/cloud-init/web-server.yml"
}
```

Example user data file (`/etc/cloud-init/web-server.yml`):
```yaml
#cloud-config
packages:
  - nginx
  - curl
  - htop

runcmd:
  - systemctl enable nginx
  - systemctl start nginx
  - ufw allow 80
  - ufw --force enable

write_files:
  - path: /var/www/html/index.html
    content: |
      <!DOCTYPE html>
      <html>
        <head><title>Hello from Nomad VM</title></head>
        <body><h1>VM deployed via Nomad Cloud Hypervisor driver!</h1></body>
      </html>
    permissions: '0644'
```

#### Inline User Data

```hcl
config {
  user_data = <<EOF
#cloud-config
package_update: true
packages:
  - docker.io
runcmd:
  - systemctl enable docker
  - systemctl start docker
  - docker run -d -p 80:80 nginx:alpine
EOF
}
```

### Built-in Cloud-Init Features

#### User Authentication

```hcl
config {
  default_user_password = "secure-password"
  default_user_authorized_ssh_key = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB..."
}
```

#### Custom Commands

```hcl
config {
  # Commands run during boot process
  cmds = [
    "apt-get update",
    "apt-get install -y docker.io",
    "systemctl enable docker"
  ]
}
```

#### Network Configuration

Cloud-init automatically generates network configuration based on:
- Static IP settings from task configuration
- Driver network configuration
- DHCP fallback for dynamic assignment

## 💾 Storage

### Disk Images

#### Supported Formats
- **Raw** (`.img`)
- **QCOW2** (`.qcow2`)
- **VHD** (`.vhd`)
- **VMDK** (`.vmdk`)

### Thin Provisioning

Enable thin copy for faster VM startup:

```hcl
config {
  image = "/var/lib/images/base-ubuntu.img"
  use_thin_copy = true
  primary_disk_size = 10240  # 10GB allocated space
}
```

### Shared Filesystems

Mount host directories into VMs using VirtioFS:

```hcl
job "shared-storage" {
  group "app" {
    volume "shared-data" {
      type   = "host"
      source = "app-data"
    }

    task "processor" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/data-processor.img"
      }

      volume_mount {
        volume      = "shared-data"
        destination = "/app/data"
        read_only   = false
      }

      resources {
        cpu    = 2000
        memory = 4096
      }
    }
  }
}
```

## 🚧 Device Passthrough (Coming Very Soon!)

### VFIO Configuration

⚠️ **VFIO passthrough is a high-priority feature in active development and will be available very soon.** This is one of Cloud Hypervisor's most valuable capabilities and is being prioritized for the next release.

The configuration would look like this when implemented:

```hcl
# This feature is not yet available
# config {
#   vfio {
#     allowlist = [
#       "10de:*",      # All NVIDIA devices
#       "8086:0d26",   # Intel specific device
#       "1002:67df"    # AMD Radeon RX 480
#     ]
#     iommu_address_width = 48
#     pci_segments = 1
#   }
# }
```

### GPU Passthrough Example

```hcl
job "ai-training" {
  datacenters = ["dc1"]

  constraint {
    attribute = "${node.unique.name}"
    value     = "gpu-node-1"
  }

  group "training" {
    task "model-training" {
      driver = "nomad-driver-ch"

      config {
        image = "/var/lib/images/cuda-pytorch.img"

        # Pass through NVIDIA RTX 3080 (not yet implemented)
        # vfio_devices = ["10de:2204", "10de:1aef"]  # GPU + Audio controller
      }

      resources {
        cpu    = 8000
        memory = 32768
        device "nvidia/gpu" {
          count = 1
        }
      }
    }
  }
}
```

## 📊 Monitoring

### Resource Statistics

The driver provides real-time VM resource statistics:

```bash
# View allocation statistics
nomad alloc status <alloc-id>

# Monitor resource usage
nomad alloc logs -f <alloc-id> <task-name>
```

### VM Health Checks

Configure health checks for VM services:

```hcl
task "web-server" {
  driver = "virt"

  config {
    image = "/var/lib/images/nginx.img"
    network_interface {
      bridge {
        name = "br0"
        static_ip = "192.168.1.100"
      }
    }
  }

  service {
    name = "web"
    port = "http"

    check {
      type     = "http"
      path     = "/"
      interval = "30s"
      timeout  = "5s"
      address_mode = "alloc"
    }
  }
}
```

## 🔧 Troubleshooting

### Common Issues

#### VM Fails to Start

**Symptoms:**
- Task fails during startup
- Error: "Failed to parse disk image format"

**Solutions:**
```bash
# 1. Verify image format
qemu-img info /path/to/image.img

# 2. Check image paths configuration
nomad agent-info | grep -A 10 virt

# 3. Validate kernel/initramfs paths
ls -la /boot/vmlinuz* /boot/initramfs*

# 4. Test Cloud Hypervisor directly
cloud-hypervisor --kernel /boot/vmlinuz --disk path=/path/to/image.img
```

#### Network Connectivity Issues

**Symptoms:**
- VM has no network access
- Cannot reach VM from host

**Solutions:**
```bash
# 1. Check bridge configuration
ip link show br0
brctl show br0

# 2. Verify TAP interface creation
ip link show | grep tap

# 3. Test bridge connectivity
ping 192.168.1.1  # Gateway IP

# 4. Check iptables rules
iptables -L -v -n
```

### Debugging Steps

#### 1. Enable Debug Logging

**Nomad Client:**
```hcl
log_level = "DEBUG"
enable_debug = true
```

#### 2. Inspect VM State

```bash
# Check Cloud Hypervisor processes
ps aux | grep cloud-hypervisor

# Inspect VM via ch-remote
ch-remote --api-socket /path/to/api.sock info

# Monitor VM console output
tail -f /opt/nomad/data/alloc/<alloc-id>/<task>/serial.log
```

## 📚 API Reference

### Task Configuration Specification

Complete HCL task configuration reference:

```hcl
config {
  # Required: VM disk image path
  image = "/path/to/vm-image.img"

  # Optional: VM hostname
  hostname = "my-vm-host"

  # Optional: Operating system variant
  os {
    arch    = "x86_64"        # CPU architecture
    machine = "q35"           # Machine type
    variant = "ubuntu20.04"   # OS variant
  }

  # Optional: Cloud-init user data
  user_data = "/path/to/user-data.yml"  # File path
  # OR
  user_data = <<EOF           # Inline YAML
  #cloud-config
  packages:
    - nginx
  EOF

  # Optional: Timezone configuration
  timezone = "America/New_York"

  # Optional: Custom commands to run
  cmds = [
    "apt-get update",
    "systemctl enable nginx"
  ]

  # Optional: Default user configuration
  default_user_authorized_ssh_key = "ssh-rsa AAAAB3..."
  default_user_password = "secure-password"

  # Optional: Storage configuration
  use_thin_copy = true        # Enable thin provisioning
  primary_disk_size = 20480   # Primary disk size in MB

  # Optional: Cloud Hypervisor specific
  kernel = "/boot/custom-kernel"      # Custom kernel path
  initramfs = "/boot/custom-initrd"   # Custom initramfs path
  cmdline = "console=ttyS0 quiet"     # Kernel command line

  # Optional: Network interface configuration
  network_interface {
    bridge {
      name = "br0"                    # Bridge name (required)
      ports = ["web", "api"]          # Port labels to expose
      static_ip = "192.168.1.100"     # Static IP address
      gateway = "192.168.1.1"         # Custom gateway
      netmask = "24"                  # Subnet mask (CIDR)
      dns = ["8.8.8.8", "1.1.1.1"]   # Custom DNS servers
    }
  }

  # Optional: VFIO device passthrough (coming very soon!)
  # vfio_devices = ["10de:2204"]  # PCI device IDs

  # Optional: USB device passthrough
  usb_devices = ["046d:c52b"]   # USB vendor:product IDs
}
```

### Resource Configuration

```hcl
resources {
  cpu    = 2000    # CPU shares (1 core = 1000)
  memory = 2048    # Memory in MB

  # Optional: GPU devices
  device "nvidia/gpu" {
    count = 1
    constraint {
      attribute = "${device.attr.compute_capability}"
      operator  = ">="
      value     = "6.0"
    }
  }
}
```

## 🛠 Development

### Building from Source

**Prerequisites:**
- Go 1.19 or later
- Git

**Build Steps:**
```bash
# Clone repository
git clone https://github.com/ccheshirecat/nomad-driver-ch.git
cd nomad-driver-ch

# Install dependencies
go mod download

# Run tests
go test ./...

# Build binary
go build -o nomad-driver-ch .

# Install plugin
sudo cp nomad-driver-ch /opt/nomad/plugins/
```

### Testing

**Unit Tests:**
```bash
go test ./...
```

**Integration Tests:**
```bash
# Requires Cloud Hypervisor installation
sudo go test -v ./virt/... -run Integration
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Quick Contribution Checklist

- [ ] Fork the repository
- [ ] Create a feature branch (`git checkout -b feature/amazing-feature`)
- [ ] Write tests for your changes
- [ ] Ensure all tests pass (`go test ./...`)
- [ ] Run linting (`golangci-lint run`)
- [ ] Commit with clear messages
- [ ] Push to your fork
- [ ] Create a Pull Request

### Development Guidelines

1. **Code Style**: Follow Go conventions and use `gofmt`
2. **Testing**: Maintain >80% test coverage
3. **Documentation**: Update docs for user-facing changes
4. **Compatibility**: Maintain backward compatibility
5. **Security**: Never commit secrets or credentials

## 📄 License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [HashiCorp Nomad](https://www.nomadproject.io/) team for the excellent orchestration platform
- [Intel Cloud Hypervisor](https://www.cloudhypervisor.org/) team for the lightweight VMM
- [Cloud-init](https://cloud-init.io/) project for VM initialization
- All contributors who help improve this driver

---

**Made with ❤️ for the cloud-native community**