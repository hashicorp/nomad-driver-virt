Nomad Virt Driver
==================
The virt driver task plugin expands the types of workloads Nomad can run to add virtual machines.
Leveraging on the power of Libvirt, the Virt driver allows the user to define virtual machine tasks using the Nomad job spec.

> **_IMPORTANT:_** This plugin is in tech preview, still under active development, there might be breaking changes in future releases

**: This is an Alpha version still under development**

## Features

* Use the job's `task.config` to define the cloud image for your virtual machine
* Start/stop virtual machines
* [Nomad runtime environment](https://www.nomadproject.io/docs/runtime/environment.html) is populated
* Use Nomad alloc data in the virtual machine.
* Publish ports
* Monitor the memory consumption
* Monitor CPU usage
* Task config cpu value is used to populate virtual machine CpuShares
* The tasks `task`, `alloc`, and `secrets` directories are mounted within the VM at the filesystem
  root. These are currently mounted read-only (RO), so VMs do not write excessive amounts of data
  which results in the host filesystem filling. Once correct guardrails are in place, this
  restriction will be lifted. Please see the
  [filesystem concepts page](https://developer.hashicorp.com/nomad/docs/concepts/filesystem) for
  more detail about an allocations working directory.

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

      driver = "nomad-driver-virt"

      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      }

      config {
        image                 = "local/focal-server-cloudimg-amd64.img"
        primary_disk_size     = 10000
        use_thin_copy         = true
        default_user_password = "password"
        cmds                  = ["python3 -m http.server 8000"]

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

In order to be able to build the binary, the `libvirt-dev` module is necessary,
use any of the package managers to get it:

 ```
sudo apt install libvirt-dev
```

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

`Nomad` runs as root, add the user `root` and the group `root` to the [QEMU configuration](https://libvirt.org/drvqemu.html#posix-users-groups) to allow it to execute the workloads. Remember to start the libvirtd daemon if not started yet or to restarted after adding the qemu user/group configuration:

```
systemctl start libvirtd
```
or
```
systemctl restart libvirtd
```

Ensure that Nomad can find the plugin, see [plugin_dir](https://www.nomadproject.io/docs/configuration/index.html#plugin_dir)

## Driver Configuration

* **emulator block**
  * **uri** - Since libvirt supports many different kinds of virtualization (often referred to as "drivers" or "hypervisors"), it is necessary to use a `uri` to specify which one
  to use. It defaults to `"qemu:///system"`
  * **user** - User for the [connection authentication](https://libvirt.org/auth.html).
  * **password** - Password for the [connection authentication](https://libvirt.org/auth.html).

* **data_dir** - The plugin will create VM configuration files and intermediate files in
  this directory. If not defined it will default to `/var/lib/virt`.
* **image_paths** - Specifies the host paths the QEMU driver is allowed to load images from. If not defined, it defaults to the plugin data_dir directory and alloc directory.

```hcl
plugin "nomad-driver-virt" {
  config {
    emulator {
      uri = "qemu:///default"
    }
    data_dir    = "/opt/ubuntu/virt_temp"
    image_paths = ["/var/local/statics/images/"]
  }
}
```

## Task Configuration
* **image** - Path to .img cloud image to base the VM disk on, it should be located in an allowed path. It is very important that the cloud image includes cloud init, otherwise most features will not be available for teh task.
* **use_thin_copy** - Make a thin copy of the image using qemu, and use it as the backing cloud image for the VM.
* **hostname** - The hostname to assign which defaults to a short uuid that will be unique to every VM, to avoid clashes when there are multiple instances of the same task running. Since it's used as a network host name, it must be a valid DNS label according to RFC 1123.
* **os** - Guest configuration for a specific machine and architecture to emulate if they are to be different from the host. Both the architecture and machine have to be available for KVM. If not defined, libvirt will use the same ones as the host machine.
* **command** - List of commands to execute on the VM once it is running. They can provide the operator with a quick and easy way to start a process on the newly created VM, used in conjunction with the template, it can be a simple yet powerful start up tool.
* **default_user_password** - Initial password to be configured for the default user on the newly created VM, it will have to be updated on first connect.
* **default_user_authorized_ssh_key** - SSH public key that will be added to the SSH configuration for the default user of the cloud image distribution.
* **user_data** - Path to a cloud-init compliant user data file to be used as the user-data for the cloud-init configuration.
* **primary_disk_size** - Disk space to assign to the VM, bear in mind it will fit the
VM's OS.

Regarding the resources, currently the driver has support for cpuSets or cores and memory.
Every core will be treated as a vcpu.
Do not use `resources.cpus`, they will be ignored.

```sh
driver = "nomad-driver-virt"

artifact {
  source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
  destination = "local/focal-server-cloudimg-amd64.img"
  mode        = "file"
}

config {
  image                           = "local/focal-server-cloudimg-amd64.img"
  primary_disk_size               = 9000
  use_thin_copy                   = true
  default_user_password           = "password"
  cmds                            = ["touch /home/ubuntu/file.txt"]
  default_user_authorized_ssh_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC31v1..."
}
```

### Network Configuration
The following configuration options are available within the task's driver configuration block:
* **network_interface** - A list of network interfaces that should be attached to the VM. This
  currently only supports a single entry.
  * **bridge** - Attach the VM to a network of bridged type. `virbr0`, the default libvirt network
  is a bridged network.
    * **name** - The name of the bridge interface to use. This relates to the output seen from
    commands such as `virsh net-info`.
    * **ports** - A list of port labels which will be exposed on the host via mapping to the
    network interface. These labels must exist within the job specification
    [network block](https://developer.hashicorp.com/nomad/docs/job-specification/network).

The example below shows the network configuration and task configuration required to expose and map
ports `22` and `80`:
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
    driver = "nomad-driver-virt"
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

Sometimes things dont go as plan and extra tools are necessary to find the problem.
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
mount the VM's disk to inspect it. If the `use_thin_image` option is used, the driver will create 
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
