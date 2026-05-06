# Directory Examples

This example includes agent configuration for a host directory backed storage pool and jobs demonstrating different ways of using the pool.

## Setup

Perform setup described [here](../../README.md).

## Run

### Agent

In one terminal, start Nomad:

``` shellsession
$ nomad agent -dev -config ./config.hcl
```

In another terminal, run the following command to see the information about the new storage pool:

``` shellsession
$ virsh pool-info local
Name:           local
UUID:           3fd18ea7-6d88-4e6c-9df9-7d52c1cbe2c7
State:          running
Persistent:     yes
Autostart:      yes
Capacity:       24.06 GiB
Allocation:     5.93 GiB
Available:      18.14 GiB
```

### Jobs

#### Source Image

This job defines a disk that references an image to use:

``` hcl
disk {
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

During the task creation process the source image will be inspected to determine the format
and the virtual size of the image. These values are then automatically set into the disk
configuration. The disk image is a qcow2 format so the sparse option will be automatically
enabled as well.

When the task is created, a new volume will be created and populated with the image.

Run:

``` shellsession
$ nomad run ./01-python-server.nomad.hcl
```

After the job is healthy the pool can be inspected to find the new volumes.

Run:

``` shellsession
$ virsh vol-list --pool local
 Name                         Path
----------------------------------------------------------------------------------
 virt-task-2f91b3d2_hda.img   /opt/nomad/virt/storage/virt-task-2f91b3d2_hda.img
 virt-task-2f91b3d2_vda.img   /opt/nomad/virt/storage/virt-task-2f91b3d2_vda.img
 ```

Two volumes are now in the storage pool.

Inspect the first volume:

``` shellsession
$ virsh vol-info --pool local virt-task-2f91b3d2_hda.img
Name:           virt-task-2f91b3d2_hda.img
Type:           file
Capacity:       52.00 KiB
Allocation:     52.00 KiB
```

This first volume is attached to the virtual machine as a cdrom and provides the cloud-init data.

Inspect the second volume:

``` shellsession
$ virsh vol-info --pool local virt-task-2f91b3d2_vda.img
Name:           virt-task-2f91b3d2_vda.img
Type:           file
Capacity:       2.20 GiB
Allocation:     645.80 MiB
```

This second volume is attached to the virtual machine as the primary disk. The capacity of the volume is
set to the virtual capacity of the disk while the allocation is the space actually in use.

#### Chained Source Image

This job defines a disk that references an image to use and sets the `chained` attribute:

``` hcl
disk {
  chain = true
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
$ virsh vol-list local
 Name                                                                                Path
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   /opt/nomad/virt/storage/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-task-ee066ff4_hda.img                                                          /opt/nomad/virt/storage/virt-task-ee066ff4_hda.img
 virt-task-ee066ff4_vda.img                                                          /opt/nomad/virt/storage/virt-task-ee066ff4_vda.img
 ```

Inspecting the primary disk shows that very little space of the volume is currently being used:

``` shellsession
$ virsh vol-info --pool local virt-task-ee066ff4_vda.img
Name:           virt-task-ee066ff4_vda.img
Type:           file
Capacity:       2.20 GiB
Allocation:     29.70 MiB
```

The definition of the volume shows that it is chained to the parent volume:

``` shellsession
$ virsh vol-dumpxml --pool local --xpath '/volume/backingStore/path' virt-task-ee066ff4_vda.img
<path>/opt/nomad/virt/storage/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img</path>
```

Now add another task with a chained disk using the same source image and update the job:

``` shellsession
$ nomad run ./03-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool local
 Name                                                                                Path
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   /opt/nomad/virt/storage/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-alt-task-f665808b_hda.img                                                      /opt/nomad/virt/storage/virt-alt-task-f665808b_hda.img
 virt-alt-task-f665808b_vda.img                                                      /opt/nomad/virt/storage/virt-alt-task-f665808b_vda.img
 virt-task-a6dc4253_hda.img                                                          /opt/nomad/virt/storage/virt-task-a6dc4253_hda.img
 virt-task-a6dc4253_vda.img                                                          /opt/nomad/virt/storage/virt-task-a6dc4253_vda.img
```

The pool now includes the original parent volume along with the volumes for the two tasks. The definition
of both primary volumes shows they are chained to the same parent volume:

``` shellsession
$ virsh vol-dumpxml --pool local virt-alt-task-f665808b_vda.img --xpath '/volume/backingStore/path'
<path>/var/nomad-virt/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img</path>

$ virsh vol-dumpxml --pool local virt-task-a6dc4253_vda.img --xpath '/volume/backingStore/path'
<path>/var/nomad-virt/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img</path>
```

#### Volume Format

This job defines a disk that references a source image (in qcow2 format) with a `raw` format:

``` hcl
disk {
  format = "raw"
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

During the task creation the image will be inspected to determine its format (in this case `qcow2`). Since the
disk definition specifies a `raw` format the image will be converted before being written to the volume.

Run:

``` shellsession
$ nomad run ./04-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool local
 Name                                                                                Path
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   /opt/nomad/virt/storage/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-task-b4dd7018_hda.img                                                          /opt/nomad/virt/storage/virt-task-b4dd7018_hda.img
 virt-task-b4dd7018_vda.img                                                          /opt/nomad/virt/storage/virt-task-b4dd7018_vda.img
 ```

Inspecting the primary volume now shows the physical size matching that capacity:

``` shellsession
$ virsh vol-info --pool local virt-task-b4dd7018_vda.img
Name:           virt-task-b4dd7018_vda.img
Type:           file
Capacity:       2.20 GiB
Allocation:     2.20 GiB
```

Direct inspection of the volume file confirms the raw format:

``` shellsession
$ qemu-img info --output=json /opt/nomad/virt/storage/virt-task-b4dd7018_vda.img | jq '.format'
"raw"
```

#### Sparse Volume

Since a large portion of the primary volume created will consist of empty space it may be better to
not allocate that empty space immediately. This job retains the `raw` format but enables `sparse`:

``` hcl
disk {
  format = "raw"
  sparse = true
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./05-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool local
 Name                                                                                Path
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   /opt/nomad/virt/storage/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-task-0d1381bd_hda.img                                                          /opt/nomad/virt/storage/virt-task-0d1381bd_hda.img
 virt-task-0d1381bd_vda.img                                                          /opt/nomad/virt/storage/virt-task-0d1381bd_vda.img
```

Inspect the primary volume:

``` shellsession
$ virsh vol-info --pool local virt-task-0d1381bd_vda.img
Name:           virt-task-0d1381bd_vda.img
Type:           file
Capacity:       2.20 GiB
Allocation:     1.49 GiB
```

And confirm that the format is still raw:

``` shellsession
$ qemu-img info --output=json /opt/nomad/virt/storage/virt-task-0d1381bd_vda.img | jq '.format'
"raw"
```

Now we see that the capacity of the primary volume is the same as the previous task, but the
allocation is less. Only the currently used size of the volume is allocated and the allocation
size will grow as data is added to the volume until it reaches its capacity limit.

#### Sizing

This job sets the size of the volume instead of allowing it to be automatically determined. It
expands the size of the volume past the virtual size of the image:

``` hcl
disk {
  format = "raw"
  sparse = true
  size   = "40GiB"
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./06-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool local
 Name                                                                                Path
------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img   /opt/nomad/virt/storage/nmdsrc-qcow2-18f2977d77dfea1b74aee14533bd21c34f789139e949c57023b7364894b7e5e9.img
 virt-task-680a643c_hda.img                                                          /opt/nomad/virt/storage/virt-task-680a643c_hda.img
 virt-task-680a643c_vda.img                                                          /opt/nomad/virt/storage/virt-task-680a643c_vda.img
```

Inspect the primary volume:

``` shellsession
$ virsh vol-info --pool local virt-task-680a643c_vda.img
Name:           virt-task-680a643c_vda.img
Type:           file
Capacity:       40.00 GiB
Allocation:     2.66 GiB
```

The size of the primary volume is now 40GiB. Let's connect to the VM and see how this looks
from within the guest. List the running VMs:

``` shellsession
$ virsh list
 Id   Name                 State
------------------------------------
 39   virt-task-680a643c   running
```

Now connect to the VM and login. The username is the default user for Ubuntu which
is `ubuntu` and the password is the default password assigned in the job file `password`.
After login you will be required to update the password:

``` shellsession
$ virsh console 39
Connected to domain 'virt-task-680a643c'
Escape character is ^] (Ctrl + ])

nomad-virt-task-680a643c login: ubuntu
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

ubuntu@nomad-virt-task-680a643c:~$
```

The Ubuntu cloud images are configured to automatically expand the primary partition
to fill the disk. Run:

``` shellsession
$ ubuntu@nomad-virt-task-680a643c:~$ df -h | grep vda1
/dev/vda1        39G  1.5G   38G   4% /
/dev/vda15      105M  6.1M   99M   6% /boot/efi
```

The root partition is expanded to fill the available space in the volume.

#### Volume Cloning

Volumes can also be cloned from existing volumes within the pool. To demonstrate
this we will first add a source image to the pool. Start with downloading the
image:

``` shellsession
curl -OL http://10.162.122.1:3333/focal-server-cloudimg-amd64.img
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100  617M  100  617M    0     0  1773M      0 --:--:-- --:--:-- --:--:-- 1775M
```

Next we'll create an empty volume to upload the image into. Volume creation requires
an XML definition, which contains the following:

``` xml
<volume type="file">
  <name>focal.img</name>
  <capacity>0</capacity>
</volume>
```

Create the volume:

``` shellsession
$ virsh vol-create --pool local ./volume.xml
Vol focal.img created from ./volume.xml
```

And upload the image into the volume:

``` shellsession
$ virsh vol-upload --pool local focal.img ./focal-server-cloudimg-amd64.img
```

Inspecting the volume shows that the volume is now populated:

``` shellsession
$ virsh vol-info --pool local focal.img
Name:           focal.img
Type:           file
Capacity:       2.20 GiB
Allocation:     617.98 MiB
```

In the job, the disk definition is updated to reference the new volume:

``` hcl
disk {
  source {
    volume = "focal.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./07-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
virsh vol-list --pool local
 Name                         Path
----------------------------------------------------------------------------------
 focal.img                    /opt/nomad/virt/storage/focal.img
 virt-task-5ac45668_hda.img   /opt/nomad/virt/storage/virt-task-5ac45668_hda.img
 virt-task-5ac45668_vda.img   /opt/nomad/virt/storage/virt-task-5ac45668_vda.img
```

And inspect the new cloned volume:

``` shellsession
virsh vol-info --pool local virt-task-5ac45668_vda.img
Name:           virt-task-5ac45668_vda.img
Type:           file
Capacity:       2.20 GiB
Allocation:     1.50 GiB
```

#### Chained Volume

This job creates a lightweight clone by chaining the new volume to
the source volume.

``` hcl
disk {
  chained = true
  source {
    volume = "focal.img"
  }
}
```

Run:

``` shellsession
$ nomad run ./08-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool local
 Name                         Path
----------------------------------------------------------------------------------
 focal.img                    /opt/nomad/virt/storage/focal.img
 virt-task-fe288807_hda.img   /opt/nomad/virt/storage/virt-task-fe288807_hda.img
 virt-task-fe288807_vda.img   /opt/nomad/virt/storage/virt-task-fe288807_vda.img
```

Inspecting the volume shows the expected small allocation size:

``` shellsession
virsh vol-info --pool local virt-task-fe288807_vda.img
Name:           virt-task-fe288807_vda.img
Type:           file
Capacity:       2.20 GiB
Allocation:     25.70 MiB
```

And the volume definition shows it is backed by the source volume:

``` shellsession
$ virsh vol-dumpxml --pool local --xpath '/volume/backingStore/path' virt-task-fe288807_vda.img
<path>/opt/nomad/virt/storage/focal.img</path>
```

#### Multiple Volumes

Additional volumes can be created by adding `disk` blocks. Once more than one `disk`
is defined in a task, the primary disk must be set to identify which disk is responsible
for booting the VM.

``` hcl
disk {
  primary = true
  source {
    image = "local/focal-server-cloudimg-amd64.img"
  }
}

disk {
  size = "20GiB"
}
```

Run:

``` shellsession
$ nomad run ./09-python-server.nomad.hcl
```

After the job is healthy, inspect the pool:

``` shellsession
$ virsh vol-list --pool local
 Name                         Path
----------------------------------------------------------------------------------
 virt-task-35461227_hda.img   /opt/nomad/virt/storage/virt-task-35461227_hda.img
 virt-task-35461227_vda.img   /opt/nomad/virt/storage/virt-task-35461227_vda.img
 virt-task-35461227_vdb.img   /opt/nomad/virt/storage/virt-task-35461227_vdb.img
```

The extra volume defined in the task can be seen in the pool. Now let's have a look
on the VM:

List the running VMs:

``` shellsession
$ virsh list
 Id   Name                 State
------------------------------------
 42   virt-task-35461227   running
```

Connect and log into the VM:

``` shellsession
$ virsh console 42
Connected to domain 'virt-task-35461227'
Escape character is ^] (Ctrl + ])

nomad-virt-task-35461227 login: ubuntu
Password:
You are required to change your password immediately (administrator enforced)
Changing password for ubuntu.
Current password:
New password:
Retype new password:
Welcome to Ubuntu 20.04.6 LTS (GNU/Linux 5.4.0-216-generic x86_64)
...
ubuntu@nomad-virt-task-35461227:~$
```

Using `fdisk` we can see the volume is attached to the VM ready
for use:

``` shellsession
$ ubuntu@nomad-virt-task-35461227:~$ sudo fdisk -l | grep vdb
Disk /dev/vdb: 20 GiB, 21474836480 bytes, 41943040 sectors
```
