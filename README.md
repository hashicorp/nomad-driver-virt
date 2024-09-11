Nomad Virt Driver
==================

## Features

* Use the job's `task.config` to define the cloud image for your virtual machine
* Start/stop virtual machines
* [Nomad runtime environment](https://www.nomadproject.io/docs/runtime/environment.html) is populated
* Use Nomad alloc data in the virtual machine.
* Publish ports
* Monitor the memory consumption
* Monitor CPU usage
* Task config cpu value is used to populate virtual machine CpuShares

## Ubuntu Example job

Here is a simple python server on ubuntu Example:

```hcl
job "python-server" {
  datacenters = ["dc1"]

  group "virt-group" {
    count = 1

    task "virt-task" {

      driver = "nomad-driver-virt"

      config {
        image                           = "/var/local/statics/images/focal-server-cloudimg-amd64.img"
        primary_disk_size               = 10000
        use_thin_copy                   = true
        default_user_password           = "password"
        cmds                            = ["python -m http.server 8000"]
      }

      resources {
        cpu    = 40
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

This project has a `go.mod` definition. So you can clone it to whatever directory you want.
It is not necessary to setup a go path at all.
Ensure that you use go 1.22 or newer.

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

Make sure the node where the client will run supports virtualization.

`Nomad` runs as root, add the [user](https://github.com/virtualopensystems/libvirt/blob/4fbfac851e05670cb7cb378ef1c3560b82a05473/src/qemu/qemu.conf#L214) `root` and the 
[group](https://github.com/virtualopensystems/libvirt/blob/4fbfac851e05670cb7cb378ef1c3560b82a05473/src/qemu/qemu.conf#L231) `root` to the QEMU configuration
to allow it to execute the workloads. Remember to start the libvirtd daemon if not started yet or to restarted after adding the qemu user/group configuration: 

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
  * **user** - User for OpenAuth
  * **password** - Password for OpenAuth

* **data_dir** - In the creation of the vm we will have to create intermediate files, this would be the directory to do so. And to store any configuration files related to the plugin. If not defined
it will default to `/var/lib/virt`
* **image_paths** - Specifies the host paths the QEMU driver is allowed to load images from. If not defined, it defaults to the plugin data_dir directory and alloc directory.

```hcl
plugin "nomad-driver-virt" {
  emulator {
    uri = "qemu:///default"
  }
  data_dir    = "/opt/ubuntu/virt_temp"
  image_paths = ["/var/local/statics/images/"]
}
```

## Task Configuration
* **image** - Path to .img cloud image to base the VM disk on, it should be located in an allowed path.
* **use_thin_copy** - Make a thin copy of the image using qemu, and use it as the backing cloud image for the VM. 
* **hostname** - The hostname to assign which defaults to a short uuid that will be unique to every VM, to avoid clashes when there are multiple instances of the same task running. Since it's used as a network host name, it must be a valid DNS label according to RFC 1123.
* **os** - Guest configuration for a specific machine and architecture to emulate if they are to be different from the host. Both the architecture and machine have to be available for KVM. If not defined, libvirt will use the same ones as the host machine.
* **command** - List of commands to execute on the VM once it is running. They can provide the operator with a quick and easy way to start a process on the newly created VM, used in conjunction with the template, it can be a simple yet powerful start up tool.
* **default_user_password** - Initial password to be configured for the default user on the newly created VM, it will have to be updated on first connect.
* **default_user_authorized_ssh_key** - SSH public key that will be added to the ssh configuration for the default user of the cloud image distribution.
* **user_data** - Path to a cloud-init compliant user data file to be used as the user-data for the cloud-init configuration.
* **primary_disk_size** - Disk space to assign to the VM, bear in mind it will fit the
VM's OS.

```sh
  driver = "nomad-driver-virt"
      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "focal-server-cloudimg-amd64.img"
        mode        = "file"
      } 

      config {
        image                           = "focal-server-cloudimg-amd64.img"
        primary_disk_size               = 9000
        use_thin_copy                   = true
        default_user_password           = "password"
        cmds                            = ["touch /home/ubuntu/file.txt"]
        default_user_authorized_ssh_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC31v1...
      }

```

## Network Configuration

## Local Development

Make sure the node suports virtualization.

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
