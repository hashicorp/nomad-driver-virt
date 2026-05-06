# Dynamic Host Volume Examples

This example includes a dynamic host volume script, agent configuration, and jobs
demonstrating different ways of using dynamic host volumes.

## Setup

Perform setup described [here](../../README.md). After that is complete, create the
expected directory for the volume plugin and copy it into the new directory:

``` shellsession
$ mkdir /opt/nomad/volume-plugins
$ cp ./volume-plugins/mkblock /opt/nomad/volume-plugins/mkblock
```

## Run

### Agent

In one terminal, start Nomad:

``` shellsession
$ nomad agent -dev -config ./config.hcl
```

### Jobs

In another terminal, verify that the host volume plugins are registered:

``` shellsession
$ nomad node status -self -verbose | grep plugins.host_volume
plugins.host_volume.mkblock.version = 0.0.1
plugins.host_volume.mkdir.version   = 0.0.1
```

#### Mount Volume

_TODO_: link to mkdir plugin docs

This job will use a dynamic host volume created with the [mkdir plugin][mkdir-plugin] to store
the `index.html` which will be served by the python server. Start by creating
the volume:

``` shellsession
$ nomad volume create ./volumes/virt-data-dir.hcl
==> Created host volume virt-data-dir with ID b85b6a19-6b5c-e8d4-1965-6f0bae4566f2
  ✓ Host volume "b85b6a19" ready

    2026-05-05T23:03:35Z
    ID        = b85b6a19-6b5c-e8d4-1965-6f0bae4566f2
    Name      = virt-data-dir
    Namespace = default
    Plugin ID = mkdir
    Node ID   = a3f6e7ad-eb1c-a1a1-252e-af554e15cba4
    Node Pool = default
    Capacity  = 0 B
    State     = ready
    Host Path = /opt/nomad/virt/host-volumes/b85b6a19-6b5c-e8d4-1965-6f0bae4566f2
```

The job includes the volume configuration:

``` hcl
volume "virt-data" {
  type   = "host"
  source = "virt-data-dir"
}
```

And the volume mount configuration within the task:

``` hcl
volume_mount {
  volume      = "virt-data"
  destination = "/mnt/data"
}
```

The volume will be mounted within the virtual machine task at `/mnt/data` and the
`index.html` file will be copied into the mount. The python web server will then
serve the `/mnt/data` directory.

Run:

``` shellsession
$ nomad run ./01-python-server.nomad.hcl
```

After the service is healthy, fetch the dynamic port. First get the allocation ID for the python-server job:

``` shell-session
$ ALLOC_ID="$(nomad job allocs -json python-server | jq -r '.[].ID')"
```

Next, get the dynamic port from the allocation using the allocation ID:

``` shell-session
$ PORT="$(nomad alloc status -json $ALLOC_ID | jq -r '.Resources.Networks[0].DynamicPorts[0].Value')"
```

Use the port to make a request to the service:

``` shell-session
$ curl 127.0.0.1:$PORT
<pre>
Guest System

\o/
</pre>
```

Using the volume information we can verify the `index.html` file has been stored in the volume. Run:

``` shellsession
$ ls -l "$(nomad volume status -json virt-data-dir | jq -r '.HostPath')"
total 4
-rw-r--r-- 1 root root 31 May  5 23:04 index.html
```

#### Block Volume (secondary)

Here we will create a block storage volume and attach it to the virtual machine as
a secondary disk. Start by creating the volume:

``` shellsession
$ nomad volume create ./volumes/virt-secondary-volume.hcl
==> Created host volume virt-secondary-volume with ID 65a825ac-54e5-bdb4-f288-959bfe62fd77
  ✓ Host volume "65a825ac" ready

    2026-05-05T23:29:27Z
    ID        = 65a825ac-54e5-bdb4-f288-959bfe62fd77
    Name      = virt-secondary-volume
    Namespace = default
    Plugin ID = mkblock
    Node ID   = a3f6e7ad-eb1c-a1a1-252e-af554e15cba4
    Node Pool = default
    Capacity  = 500 MiB
    State     = ready
    Host Path = /dev/loop1
```

The job includes volume configuration:

``` hcl
volume "virt-secondary" {
  type            = "host"
  source          = "virt-secondary-volume"
  attachment_mode = "block-device"
}
```

And volume mount configuration:

``` hcl
volume_mount {
  volume      = "virt-secondary"
  destination = "/dev/vdb"
}
```

Note the destination path of `/dev/vdb` within the volume mount block. This signals
to the plugin that the volume is a block device and specifies the device name that
should be used within the virtual machine.

Finally, a disk entry is added to the task configuration which references the destination
defined within the volume mount configuration:

``` hcl
disk {
  volume = "/dev/vdb"
}
```

Run:

``` shellsession
$ nomad run ./02-python-server.nomad.hcl
```

After the job is healthy, let's connect to the VM and inspect the storage devices. List
the running VMs:

``` shellsession
$ virsh list
 Id   Name                 State
------------------------------------
 47   virt-task-b42ec407   running
```

Now connect to the VM and login. The username is the default user for Ubuntu which
is `ubuntu` and the password is the default password assigned in the job file `password`.
After login you will be required to update the password:

``` shellsession
$ virsh console 47
Connected to domain 'virt-task-b42ec407'
Escape character is ^] (Ctrl + ])

nomad-virt-task-b42ec407 login: ubuntu
Password:
You are required to change your password immediately (administrator enforced)
Changing password for ubuntu.
Current password:
New password:
Retype new password:
Welcome to Ubuntu 20.04.6 LTS (GNU/Linux 5.4.0-216-generic x86_64)
...
To run a command as administrator (user "root"), use "sudo <command>".
See "man sudo_root" for details.

ubuntu@nomad-virt-task-b42ec407:~$
```

Using `fdisk` we can verify the volume is attached to the virtual machine:

``` shellsession
$ sudo fdisk -l | grep vda
Disk /dev/vda: 500 MiB, 524288000 bytes, 1024000 sectors
```

#### Block Volume (primary)

In this example, we will create a block storage volume and attach it to the virtual
machine as the primary disk. Start by creating the volume:

``` shellsession
$ nomad volume create ./volumes/virt-primary-volume.hcl
==> Created host volume virt-primary-volume with ID b651dc5d-3bfe-d332-7727-810f98840f7b
  ✓ Host volume "b651dc5d" ready

    2026-05-06T00:22:07Z
    ID        = b651dc5d-3bfe-d332-7727-810f98840f7b
    Name      = virt-primary-volume
    Namespace = default
    Plugin ID = mkblock
    Node ID   = b6f43cf0-1654-bd4f-c8dc-df1332980ce4
    Node Pool = default
    Capacity  = 5.0 GiB
    State     = ready
    Host Path = /dev/loop3
```

The job includes volume configuration:

``` hcl
volume "virt-primary" {
  type            = "host"
  source          = "virt-primary-volume"
  attachment_mode = "block-device"
}
```

And volume mount configuration:

``` hcl
volume_mount {
  volume      = "virt-primary"
  destination = "/dev/vda"
}
```

The task configuration includes a single disk entry which is backed by the volume:

``` hcl
disk {
  volume = "/dev/vda"
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

The task is configured to copy the `index.html` file to the root directory and
then start the python web server serving the root directory.

Run:

``` shellsession
$ nomad run ./03-python-server.nomad.hcl
```

After the job is healthy, find the allocation ID:

``` shellsession
$ nomad job allocs -json python-server | jq -r '.[].ID'
e9f2b953-5f5d-7f7c-4814-3ca23e599a63
```

Now inspect the volume and validate the python-server allocation is using the volume:

``` shellsession
$ nomad volume status -json virt-primary-volume | jq -r '.Allocations.[].ID'
e9f2b953-5f5d-7f7c-4814-3ca23e599a63
```

Confirm that the python web server is serving the expected content:

``` shellsession
$ ALLOC_ID="$(nomad job allocs -json python-server | jq -r '.[].ID')"
$ PORT="$(nomad alloc status -json $ALLOC_ID | jq -r '.Resources.Networks[0].DynamicPorts[0].Value')"
$ curl http://127.0.0.1:$PORT
<pre>
Guest System

\o/
</pre>
```

Stop and purge the job:

``` shellsession
$ nomad job stop -purge python-server
```

Now we will reuse the existing volume for the next job. Notice that job does not
include an artifact block to download the Ubuntu image and the disk configuration
block now only includes the volume:

``` hcl
disk {
  volume = "/dev/vda"
}
```

Similarly, the `cmds` defined in the task now only contain the command to start
the python web server serving the root directory. Since the primary disk is now
a durable volume, changes are persisted and we expect the same out from the
python web server.

Run:

``` shellsession
$ nomad run ./04-python-server.nomad.hcl
```

After the job is healthy, verify the allocation is using the volume:

``` shellsession
$ nomad job allocs -json python-server | jq -r '.[].ID'
f00a44ef-f0e0-8eed-f3d5-48dfc99668ad
$nomad volume status -json virt-primary-volume | jq -r '.Allocations.[].ID'
f00a44ef-f0e0-8eed-f3d5-48dfc99668ad
```

Finally, confirm that the web server is serving the expected content:

``` shellsession
$ ALLOC_ID="$(nomad job allocs -json python-server | jq -r '.[].ID')"
$ PORT="$(nomad alloc status -json $ALLOC_ID | jq -r '.Resources.Networks[0].DynamicPorts[0].Value')"
$ curl http://127.0.0.1:$PORT
<pre>
Guest System

\o/
</pre>
```

[mkdir-plugin]: https://developer.hashicorp.com/nomad/docs/other-specifications/volume/host#mkdir-plugin
