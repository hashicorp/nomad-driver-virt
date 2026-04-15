Nomad Virt Driver
==================

The virt driver task plugin expands the types of workloads Nomad can run to add virtual machines. Currently
leveraging the power of libvirt, the virt driver allows users to define virtual machine tasks using the
Nomad job spec.

> **_IMPORTANT:_** This plugin is in tech preview, still under active development, there might be breaking changes in future releases

**: This is an Alpha version still under development**

## Features

* Use the job's `task.config` to define the virtual machine (VM).
* Start/stop virtual machines.
* [Nomad runtime environment](https://www.nomadproject.io/docs/runtime/environment.html) is populated.
* Use Nomad alloc data in the virtual machine.
* Publish ports.
* Monitor the memory consumption.
* Monitor CPU usage.
* Task config cpu value is used to populate virtual machine CpuShares.
* The tasks `task`, `alloc`, and `secrets` directories are mounted within the VM at the filesystem
  root. These are currently mounted read-only to prevent excessive amounts of data being written to
  the host filesystem. Please see the [filesystem concepts page](https://developer.hashicorp.com/nomad/docs/concepts/filesystem)
  for more detail about an allocations working directory.

## Ubuntu Example job

Here is a simple Python server on Ubuntu example:

```hcl
job "python-server" {

  group "virt-group" {
    count = 1

    network {
      mode = "host"
      port "http" {
        to = 8000
      }
    }

    task "virt-task" {

      driver = "virt"

      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      }

      config {
        default_user_password = "password"
        cmds                  = ["python3 -m http.server 8000"]

        disk {
          size = "10GiB"
          source {
            image = "local/focal-server-cloudimg-amd64.img"
          }
        }

        network_interface {
          bridge {
            name  = "virbr0"
            ports = ["http"]
          }
        }
      }

      resources {
        cores  = 2
        memory = 4000
      }
    }
  }
}
```

```sh
$ nomad job run examples/python.nomad.hcl

==> 2024-09-10T13:01:22+02:00: Monitoring evaluation "c0424142"
    2024-09-10T13:01:22+02:00: Evaluation triggered by job "python-server"
    2024-09-10T13:01:23+02:00: Evaluation within deployment: "d546f16e"
    2024-09-10T13:01:23+02:00: Allocation "db146826" created: node "c20ee15a", group "virt-group"
    2024-09-10T13:01:23+02:00: Evaluation status changed: "pending" -> "complete"
==> 2024-09-10T13:01:23+02:00: Evaluation "c0424142" finished with status "complete"

$ virsh list

 Id   Name                 State
------------------------------------
 4    virt-task-5a6e215e   running

```

## Building The Driver from source

In order to build the plugin binary some development libraries are required:

* libvirt
* librbd

For Debian/Ubuntu based systems:

``` shell-session
apt install libvirt-dev librbd-dev
```

For RHEL based systems:

``` shellsession
dnf install libvirt-devel librbd-devel
```

To build the plugin:

```shell-session
git clone git@github.com:hashicorp/nomad-driver-virt
cd nomad-driver-virt
make dev
```

The compiled binary will be located at `./build/nomad-driver-virt`.

## Runtime dependencies

* [Nomad](https://www.nomadproject.io/downloads.html) 1.9.0+
* [libvirt-daemon-system](https://pkgs.org/download/libvirt-daemon-system)
* [qemu-utils](https://pkgs.org/download/qemu-utils)

Make sure the node where the client will run supports virtualization, in Linux you can do it in a couple of ways:
1. Reading the CPU flags:
```sh
egrep -o '(vmx|svm)' /proc/cpuinfo
```

2. Reading the kernel modules and looking for the virtualization ones:
```sh
lsmod | grep -E '(kvm_intel|kvm_amd)'
```

If the result is empty for either call, the machine does not support virtualization and the nomad client wont be able to run any virtualization workload.

3. Verify permissions:
`Nomad` runs as root, add the user `root` and the group `root` to the [QEMU configuration](https://libvirt.org/drvqemu.html#posix-users-groups) to allow it to execute the workloads. Remember to start the libvirtd daemon if not started yet or to restart it after adding the qemu user/group configuration:

```
systemctl start libvirtd
```
or
```
systemctl restart libvirtd
```

Ensure that Nomad can find the plugin, see [plugin_dir](https://www.nomadproject.io/docs/configuration/index.html#plugin_dir)

## Driver Configuration

* **image_paths** - Host paths containing image files allowed to be used by tasks.
* **provider** - Named block containing provider configuration. Defaults to libvirt.
* **storage_pools** - Block containing storage pool configuration.

### Provider - libvirt

* **password** - The libvirt password to use for authentication.
* **uri** - The libvirt driver to use. Defaults to `qemu:///system`.
* **user** - The libvirt user to use for authentication.

### Storage pools

Storage pools contain volumes which are created for, and attached to, task VMs. Two
types of storage pools are supported by the driver: directory and Ceph. Directory
storage pools are host local storage pools with volumes stored at a specified path.
Ceph storage pools are RBD based volumes stored in Ceph.

A default storage pool must be assigned. If the configuration only
defines a single storage pool, that storage pool is automatically the default.

* **ceph** - Named block containing Ceph based storage pool configuration.
* **default** - Name of the default storage pool. If only one storage pool is defined, it is automatically the default.
* **directory** - Named block containing directory based storage pool configuration.

#### Storage pool - directory

* **path** - Host path to contain the pool volumes.

#### Storage pool - ceph

* **authentication** - Block containing authentication configuration.
  * **username** - Ceph client name .
  * **secret** - Ceph client key (base64 encoded).
* **hosts** - List of Ceph monitors.
* **pool** - Name of the Ceph pool.

### Examples

Minimal configuration defining a directory storage pool on the host and defining
a directory for image files which tasks may reference:

``` hcl
plugin "nomad-driver-virt" {
  config {
    image_paths = ["/var/lib/virt/images"]
    storage_pools {
      directory "local-storage" {
        path = "/var/lib/virt/storage"
      }
    }
  }
}
```

This full libvirt configuration example has a username and password and allows
multiple host directories for image files. It defines two storage pools, one
directory and one Ceph, and the directory storage pool is marked as the default:

``` hcl
plugin "nomad-driver-virt" {
  config {
    provider "libvirt" {
      uri      = "qemu:///system"
      user     = "libvirt-username"
      password = "libvirt-pass"
    }

    image_paths = [
      "/var/lib/virt/images",
      "/opt/custom-images",
    ]

    storage_pools {
      default = "local-storage"
      directory "local-storage" {
        path = "/var/lib/virt/storage"
      }

      ceph "remote-storage" {
        pool  = "nomad-pool"
        hosts = [
          "10.0.0.2:3300",
          "10.0.0.12:3300",
          "10.0.0.99:3300",
        ]
        authentication {
          username = "nomad"
          secret   = "AQCzNMxpb6aWIxAA7YrNMSg8z5TxEvB0jsuibQ=="
        }
      }
    }
  }
}
```

## Task Configuration

* **cmds** - List of commands to execute on the VM once it is running.
* **default_user_authorized_ssh_key** - SSH public key added to the SSH configuration for the default user of the cloud image distribution.
* **default_user_password** - Initial password configured for the default user of the cloud image distribution.
* **disk** - A list of disk configurations for volumes to be attached to the VM.
* **hostname** - Hostname assigned. Must be a valid DNS label according to RFC 1123. Defaults to a name based on the task name.
* **network_interface** A list of network interfaces to be attached to the VM. Currently only a single entry is supported.
* **os** - Configuration for specific machine and architecture to emulate. Default to match host machine.
* **user_data** - Path to a cloud-init compliant user data file to be used as the user-data for the cloud-init configuration.

_Note_: The driver currently has support for cpuSets or cores and memory. Every core will be treated as a vcpu. Do not use `resources.cpus`, they will be ignored.

### Disk

A disk describes a volume to be attached to the task VM. Multiple disks can be defined within a task's configuration,
with one disk required to be identified as the `primary` disk. A disk can provide a volume that is an empty block device,
a clone of an existing volume within the storage pool, or formatted with a supplied image.

* **bus_type** - Bus type for the disk. Defaults to `virtio`.
* **chained** - Disk is an overlay on the source.
* **devname** - Device name used within the VM. Auto-generated by default.
* **driver** - Driver to use for the disk. Usage and default value is provider specific.
* **format** - Format of the disk. Default is provider specific.
* **kind** - Kind of disk defined. Defaults to `disk`.
* **pool** - Storage pool to place volume created from this definition. Defaults to the default storage pool.
* **primary** - Disk is the primary to boot the VM.
* **read_only** - Disk is read only.
* **size** - Size of the disk as bytes, or string (example: `20GB` or `15GiB`)
* **sparse** - Disk should be sparsely populated.
* **source** - Block containing disk source configuration.
  * **format** - Format of the image. Auto-detected if unset.
  * **image** - Image to write to the disk. Overwrites any existing information on disk.
  * **volume** - Volume in storage pool to clone.
* **volume** - Nomad volume to back the disk.

#### Example

The example below shows the task and disk configuration to define a primary disk in the directory storage pool and a secondary
empty disk in the Ceph storage pool:

```hcl
job "python-server" {
  group "virt-group" {
    task "virt-task" {
      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      }

      driver = "virt"

      config {
        disk {
          size    = "10GiB"
          pool    = "local-storage"
          primary = true
          source {
            image = "local/focal-server-cloudimg-amd64.img"
          }
        }

        disk {
          size = "20GB"
          pool = "ceph-storage"
        }
      }
    }
  }
}

```

If the storage pool already contains a volume with the focal server image, it can be cloned to remove the need of
downloading and applying the image. Once the volume is cloned, it will be automatically resized to the requested
size:

```hcl
job "python-server" {
  group "virt-group" {
    task "virt-task" {
      driver = "virt"

      config {
        disk {
          size   = "10GiB"
          pool   = "local-storage"
          source {
            volume = "focal-server-cloudimg-amd64.img"
          }
        }
      }
    }
  }
}

```

Instead of making a full clone of the source volume, a chained copy may be created which overlays the new volume
on the source volume creating a copy-on-write volume:

```hcl
job "python-server" {
  group "virt-group" {
    task "virt-task" {
      driver = "virt"

      config {
        disk {
          size    = "10GiB"
          pool    = "local-storage"
          chained = true
          source {
            volume = "focal-server-cloudimg-amd64.img"
          }
        }
      }
    }
  }
}

```

Chained copies may also be used when providing a source image. A new volume will be created for the image and
any tasks that define a chained disk with that source image will be chained to that volume:

```hcl
job "python-server" {
  group "virt-group" {
    task "virt-task" {
      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      }

      driver = "virt"

      config {
        disk {
          size    = "10GiB"
          pool    = "local-storage"
          chained = true
          source {
            image = "local/focal-server-cloudimg-amd64.img"
          }
        }
      }
    }
  }
}

```

### Network Configuration

The following configuration options are available within the task's driver configuration block:

* **bridge** - Block configuration for connecting to a bridged network.
  * **name** - Name of the bridge interface to use. The default libvirt network, `virbr0`, is a bridged network.
  * **ports** - A list of port labels exposed on the host via mapping to the network interface. Labels must exist within the job specification [network block](https://developer.hashicorp.com/nomad/docs/job-specification/network).

#### Example

The example below shows the network configuration and task configuration required to expose and map ports `22` and `80`:

```hcl
group "virt-group" {

  network {
    mode = "host"
    port "ssh" {
      to = 22
    }
    port "http" {
      to = 80
    }
  }

  task "virt-task" {
    driver = "virt"
    config {
      network_interface {
        bridge {
          name  = "virbr0"
          ports = ["ssh", "http"]
        }
      }
    }
  }
}
```

Exposed ports and services can make use of the existing
[service block](https://developer.hashicorp.com/nomad/docs/job-specification/service),
so that registrations can be performed using the specified backend provider.

## Local Development

Make sure the node supports virtualization.

```
# Build the task driver plugin
make dev

# Copy the build nomad-driver-plugin executable to the plugin dir
cp ./build/nomad-driver-virt - /opt/nomad/plugins

# Start Nomad
nomad agent -config=examples/server.nomad.hcl 2>&1 > server.log &

# Run the client as sudo
sudo nomad agent -config=examples/client.nomad.hcl 2>&1 > client.log &

# Run a job
nomad job run examples/job.nomad.hcl

# Verify
nomad job status virt-example

virsh list
```
## Debugging a VM

### Before starting

If running a job for the first time, you run into errors, remember to verify the runtime [Runtime dependencies](#runtime-dependencies).

It is important to know that to protect the host machine from guests overusing the disk, managed vm don't have write access to the Nomad filesystem.

If Nomad is not running as root, the permissions for the directories used by both Nomad and the virt driver need to be adjusted.

Once the vm is running things still don't go as plan and extra tools are necessary to find the problem.
Here are some strategies to debug a failing VM:

### Connecting to a VM

By default, cloud images are password protected, by adding a `default_user_password`
a new password is assigned to the default user of the used distribution (for example,
`ubuntu` for ubuntu `fedora` for fedora, or `root` for alpine)
By running `virsh console [vm-name]`, a terminal is started inside the VM that will allow an internal inspection of the VM.

```
$ virsh list
 Id   Name                 State
------------------------------------
 1    virt-task-8bc0a63f   running

$ virsh console virt-task-8bc0a63f
Connected to domain 'virt-task-8bc0a63f'
Escape character is ^] (Ctrl + ])

nomad-virt-task-8bc0a63f login: ubuntu
Password:
```

If no login prompt shows up, it can mean the virtual machine is not booting and
adding  some extra  space to the disk may solve the problem. Remember the disk
has to fit the root image plus any other process running in the VM.

The virt driver heavily relies on `cloud-init` to execute the virtual machine's
configuration. Once you have managed to connect to the terminal, the results of
cloud init can be found in two different places:
  * `/var/log/cloud-init.log`
  * `/var/log/cloud-init-output.log`

Looking into these files can give a better understanding of any possible execution
errors.

If connecting to the terminal is not an option, it is possible to stop the job and
mount the VM's disk to inspect it. If the `use_thin_copy` option is used, the driver will create
the disk image in the directory `${plugin_config.data_dir}/virt/vm-name.img`:

```
# Find the virtual machine disk image
$ ls /var/lib/virt
virt-task-8bc0a63f.img

# Enable Network Block Devices on the Host
modprobe nbd max_part=8

# Connect the disk as network block device
qemu-nbd --connect=/dev/nbd0 '/var/lib/virt/virt-task-dc8187e3.img'

# Find The Virtual Machine Partitions
fdisk /dev/nbd0 -l

# Mount the partition from the VM
mount /dev/nbd0p1 /mnt/somepoint/
```

**Important** Don't forget to unmount the disk after finishing:

```
umount /mnt/somepoint/
qemu-nbd --disconnect /dev/nbd0
rmmod nbd
```

### Networking

For networking, the plugin leverages on the libvirt default network `default`:
```
$ virsh net-list
 Name      State    Autostart   Persistent
--------------------------------------------
 default   active   yes         yes
 ```

Under the hood, libvirt uses [dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html) to lease
IP addresses to the virtual machines, there are mutiple ways to find the IP assigned
to the nomad task.
Using virsh to find the leased IP:

```
$ virsh net-dhcp-leases default
 Expiry Time           MAC address         Protocol   IP address           Hostname                   Client ID or DUID
----------------------------------------------------------------------------------------------------------------------------------------------------------------
 2024-10-07 18:48:09   52:54:00:b5:0b:d4   ipv4       192.168.122.211/24   nomad-virt-task-dc8187e3   ff:08:24:45:0e:00:02:00:00:ab:11:63:3c:26:5b:b7:fe:b3:13
 ```

 or using the mac address to find the IP via ARP:

```
$ virsh dumpxml virt-task-8473ccfb  | grep "mac address" | awk -F\' '{ print $2}'
52:54:00:b5:0b:d4
$ arp -an | grep 52:54:00:b5:0b:d4
? (192.168.122.211) at 52:54:00:b5:0b:d4 [ether] on virbr0
```
