# Ceph Examples

This example includes agent configuration for a Ceph backed storage pool and jobs
demonstrating different ways of using the pool.

## Setup

Perform setup described [here](../../README.md).

### Ceph

The following example depends on a Ceph cluster with the following configuration:

* RBD pool named `nomad-pool`
* Client named `nomad`
  * Permissions: `mon "allow r" osd "allow *"`

#### Required Information

The Nomad configuration will require the collection of Ceph monitor addresses and the
credentials for the Ceph `nomad` user.

To gather the list of Ceph monitors, connect to the Ceph admin node and run:

``` shellsession
$ ceph mon dump -f json 2>/dev/null | jq '[.mons.[].public_addrs.addrvec.[] | select(.type == "v2").addr]'
[
  "10.162.122.246:3300",
  "10.162.122.160:3300",
  "10.162.122.64:3300",
  "10.162.122.251:3300"
]
```

To retreive the credentials for the `nomad` user, connect to the Ceph admin node and run:

``` shellsession
$ ceph auth get client.nomad
[client.nomad]
	key = AQD8bvtpvFsSHxAAetpPOmzr4VBwu1zk7YQB+w==
	caps mon = "allow r"
	caps osd = "allow *"
```

Now update the `./config.hcl` file with the information collected. The current configuration for the Ceph
storage pool looks like:

``` hcl
ceph "remote" {
  pool  = "nomad-pool"
  hosts = ["UPDATE_ME"]
  authentication {
    username = "nomad"
    secret   = "UPDATE_ME"
  }
}
```

Update the configuration using the information collected above:

``` hcl
ceph "remote" {
  pool = "nomad-pool"
  hosts = [
    "10.162.122.246:3300",
    "10.162.122.160:3300",
    "10.162.122.64:3300",
    "10.162.122.251:3300"
  ]
  authentication {
    username = "nomad"
    secret   = "AQD8bvtpvFsSHxAAetpPOmzr4VBwu1zk7YQB+w=="
  }
}
```

We will want to use the `rbd` command locally through these examples which requires
local configuration. First, connect to the Ceph admin node and generate a minimal
configuration file:

``` shellsession
$ ceph config generate-minimal-conf
# minimal ceph.conf for 05d9ecb5-4969-11f1-916d-10666a6c3ef5
[global]
	fsid = 05d9ecb5-4969-11f1-916d-10666a6c3ef5
	mon_host = [v2:10.162.122.246:3300/0,v1:10.162.122.246:6789/0] [v2:10.162.122.251:3300/0,v1:10.162.122.251:6789/0] [v2:10.162.122.160:3300/0,v1:10.162.122.160:6789/0] [v2:10.162.122.64:3300/0,v1:10.162.122.64:6789/0]
```

Copy the content and paste it into the local file: `/etc/ceph/ceph.conf`.

Next get the `admin` user credentials. Connect to the Ceph admin node and run:

``` shellsession
$ ceph auth get client.admin
[client.admin]
	key = AQA+bftpajAAMBAAIJZgzz2HUwS+rSI64DMhFQ==
	caps mds = "allow *"
	caps mgr = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
```

Copy the content and paste it into the local file: `/etc/ceph/ceph.client.admin.keyring`.

## Run

### Agent

In one terminal, start Nomad:

``` shellsession
$ nomad agent -dev -config ./config.hcl
```

### Jobs

In another terminal, verify that the storage pool is available:

``` shellsession
$ nomad node status -self -verbose | grep storage_pool.remote
driver.virt.storage_pool.remote                      = ceph
driver.virt.storage_pool.remote.provider.libvirt     = true
```

We can inspect the storage pool using the `virsh` tool:

``` shellsession
$ virsh pool-info remote
Name:           remote
UUID:           8e27e2cf-b1fc-4985-86f0-01f111c3e98b
State:          running
Persistent:     yes
Autostart:      yes
Capacity:       359.96 GiB
Allocation:     12.00 KiB
Available:      359.68 GiB
```

#### Source Image

* Job file: [./01-python-server.nomad.hcl](./01-python-server.nomad.hcl)

This job defines a disk that references an image to use:

``` hcl
disk {
  pool = "remote"
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

During the task creation process the source image will be inspected to determine
the format and the virtual size of the image. Ceph based volumes must be in `raw`
format. The source image in use is a `qcow2` formatted image, and it will be
automatically converted to `raw`.

When the task is created, a new volume will be created and populated with the image.

Run:

``` shellsession
$ nomad run ./01-python-server.nomad.hcl
```

After the job is health the pool can be inspected to find the new volume.

Run:

``` shellsession
$ virsh vol-list --pool remote
 Name                         Path
---------------------------------------------------------------------
 virt-task-3fd422cc_vda.img   nomad-pool/virt-task-3fd422cc_vda.img
```

Note that only a single volume is defined. The cloud-init ISO volume is created in
the default storage pool which is configured as the `local` storage pool. Inspecting
the `local` storage pool shows the cloud-init ISO volume:

``` shellsession
$ virsh vol-list --pool local
 Name                         Path
----------------------------------------------------------------------------------
 virt-task-3fd422cc_hda.img   /opt/nomad/virt/storage/virt-task-3fd422cc_hda.img
```

Using the `rbd` command we can get information about the storage pool directly
from Ceph which shows us that the pool contains the single expected image:

``` shellsession
$ rbd pool stats nomad-pool
Total Images: 1
Total Snapshots: 0
Provisioned Size: 2.2 GiB
$ rbd ls nomad-pool
virt-task-3fd422cc_vda.img
```

#### Chained Source Image

* Job file: [./02-python-server.nomad.hcl](./02-python-server.nomad.hcl) and [./03-python-server.nomad.hcl](./03-python-server.nomad.hcl)

This job defines a disk that references an image to use and sets the `chained` attribute:

``` hcl
disk {
  pool    = "remote"
  chained = true
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

Instead of writing the image directly to the virtual machine's volume, a new parent volume will be created
to hold the image. When the virtual machine's volume is created, it will be chained to the parent volume and
the virtual machine's volume will be copy on write.

Run:

``` shellsession
$ nomad run ./02-python-server.nomad.hcl
```

After the job is healthy the pool can be inspected to find the new volumes.

Run:

``` shellsession
$ virsh vol-list --pool remote
 Name                                                                              Path
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   nomad-pool/nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-task-3289119a_vda.img                                                        nomad-pool/virt-task-3289119a_vda.img
```

Inspecting the primary disk using `virsh` shows that the volume is fully allocated:

``` shellsession
$ virsh vol-info --pool remote virt-task-3289119a_vda.img
Name:           virt-task-3289119a_vda.img
Type:           network
Capacity:       2.20 GiB
Allocation:     2.20 GiB
```

However, this is just a peculiarity with Ceph based storage volumes. Inspecting the Ceph volume directly
using the `rbd` command shows that the volume's disk usage is expectedly low:

``` shellsession
$ rbd disk-usage nomad-pool/virt-task-3289119a_vda.img
NAME                        PROVISIONED  USED
virt-task-3289119a_vda.img      2.2 GiB  224 MiB
```

Fetching volume information shows that it is chained to the parent volume:

``` shellsession
$ rbd info nomad-pool/virt-task-3289119a_vda.img | grep parent
	parent: nomad-pool/nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img@libvirt-60683
```

Now add another task with the chained disk using the same source image and update the job:

``` shellsession
$ nomad run ./03-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool remote
 Name                                                                              Path
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   nomad-pool/nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-alt-task-23644322_vda.img                                                    nomad-pool/virt-alt-task-23644322_vda.img
 virt-task-e507480d_vda.img                                                        nomad-pool/virt-task-e507480d_vda.img
```

The pool now includes the original parent volume along with the volumes for the two tasks. Inspecting the
Ceph volumes directly shows they are chained to the same parent volume:

``` shellsession
$ rbd info nomad-pool/virt-alt-task-23644322_vda.img | grep parent
	parent: nomad-pool/nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img@libvirt-60683
$ rbd info nomad-pool/virt-task-e507480d_vda.img | grep parent
	parent: nomad-pool/nmdsrc-raw-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img@libvirt-60683
```

#### Sizing

* Job file: [./04-python-server.nomad.hcl](./04-python-server.nomad.hcl)

This job sets the size of the volume instead of allowing it to be automatically determined. It
expands the size of the volume past the virtual size of the image:

``` hcl
disk {
  pool = "remote"
  size = "40GiB"
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./04-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
virsh vol-list --pool remote
 Name                         Path
---------------------------------------------------------------------
 virt-task-744b78e1_vda.img   nomad-pool/virt-task-744b78e1_vda.img
```

Inspect the volume to validate the size:

``` shellsession
$ virsh vol-info --pool remote virt-task-744b78e1_vda.img
Name:           virt-task-744b78e1_vda.img
Type:           network
Capacity:       40.00 GiB
Allocation:     40.00 GiB
```

Direct inspection of the Ceph volume shows that the actual allocation is much less:

``` shellsession
$ rbd disk-usage nomad-pool/virt-task-744b78e1_vda.img
NAME                        PROVISIONED  USED
virt-task-744b78e1_vda.img       40 GiB  1.7 GiB
```

With the size of the volume now 40GiB, let's connect to the VM and see how this looks
from within the guest. List the running VMs:

``` shellsession
$ virsh list
 Id   Name                 State
------------------------------------
 55   virt-task-744b78e1   running
```

Now connect to the VM and login. The username is the default user for Ubuntu which
is `ubuntu` and the password is the default password assigned in the job file `password`.
After login you will be required to update the password:

``` shellsession
virsh console 55
Connected to domain 'virt-task-744b78e1'
Escape character is ^] (Ctrl + ])

nomad-virt-task-744b78e1 login: ubuntu
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

ubuntu@nomad-virt-task-744b78e1:~$
```

The Ubuntu cloud images are configured to automatically expand the primary partition
to fill the disk. Run:

``` shellsession
$ df -h | grep vda
/dev/vda1        39G  1.5G   38G   4% /
/dev/vda15      105M  6.1M   99M   6% /boot/efi
```

The root partition is expanded to fill the available space in the volume.

#### Volume Cloning

* Job file: [./05-python-server.nomad.hcl](./05-python-server.nomad.hcl)

Volumes can also be cloned from existing volumes within the pool. To demonstrate this
we will first add a source image to the pool. Start with download the image:

``` shellsession
$ curl -OL http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100  617M  100  617M    0     0  1773M      0 --:--:-- --:--:-- --:--:-- 1775M
```

Next, convert the image to raw format:

``` shellsession
$ qemu-img convert --source-format qcow2 --target-format raw ./focal-server-cloudimg-amd64.img ./focal.img
```

Now, upload the image to the Ceph cluster:

``` shellsession
$ rbd import ./focal.img nomad-pool/focal.img
Importing image: 100% complete...done.
```

Inspect the pool to see the new volume exists. Due to the volume being created in Ceph
directly the local storage pool will need to be refreshed to see the new volume:

``` shellsession
$ virsh pool-refresh remote
Pool remote refreshed

$ virsh vol-list --pool remote
 Name        Path
-----------------------------------
 focal.img   nomad-pool/focal.img
```

In the job, the disk definition is updated to reference the new volume. This will result
in a full clone of the volume being created for the new volume:

``` hcl
disk {
  pool = "remote"
  source {
    volume = "focal.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./05-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool remote
 Name                         Path
---------------------------------------------------------------------
 focal.img                    nomad-pool/focal.img
 virt-task-d456f51c_vda.img   nomad-pool/virt-task-d456f51c_vda.img
```

The task volume is a full clone so we expect its size to be roughly the same size as
the source `focal.img` volume:

``` shellsession
$ rbd disk-usage nomad-pool/focal.img
NAME       PROVISIONED  USED
focal.img      2.2 GiB  1.6 GiB
$ rbd disk-usage nomad-pool/virt-task-d456f51c_vda.img
NAME                        PROVISIONED  USED
virt-task-d456f51c_vda.img      2.2 GiB  1.6 GiB
```

#### Chained Volume

* Job file: [./06-python-server.nomad.hcl](./06-python-server.nomad.hcl)

This job creates a lightweight clone by chaining the new volume to the source volume.

``` hcl
disk {
  pool    = "remote"
  chained = true
  source {
    volume = "focal.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./06-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list remote
 Name                         Path
---------------------------------------------------------------------
 focal.img                    nomad-pool/focal.img
 virt-task-b43d6541_vda.img   nomad-pool/virt-task-b43d6541_vda.img
```

The task volume is chained so we expect its size to be very small compared to the
size of the source `focal.img` volume:

``` shellsession
$ rbd disk-usage nomad-pool/focal.img
NAME                     PROVISIONED  USED
focal.img@libvirt-31117      2.2 GiB  1.6 GiB
focal.img                    2.2 GiB      0 B
<TOTAL>                      2.2 GiB  1.6 GiB
$ rbd disk-usage nomad-pool/virt-task-b43d6541_vda.img
NAME                        PROVISIONED  USED
virt-task-b43d6541_vda.img      2.2 GiB  204 MiB
```

Inspecting the volume in Ceph directly show it is backed by the source volume:

``` shellsession
$ rbd info nomad-pool/virt-task-b43d6541_vda.img | grep parent
	parent: nomad-pool/focal.img@libvirt-31117
```
