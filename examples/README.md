# Examples 

This directory contains various examples demonstrating the features and functionality of
the nomad-driver-virt plugin. All of the examples rely on the same basic setup which is
described below.

## Prerequisites

### Install Packages

Install the required packages.

Ubuntu:

``` shell-session
apt-get install libvirt-daemon libvirt-clients libvirt-daemon-driver-storage-rbd jq curl 
```

### Update Libvirt Configuration 

Adjust the libvirt qemu configuration to set the user and group to root:

``` shell-session
printf 'user = "root"\ngroup = "root"\n' >> /etc/libvirt/qemu.conf
systemctl restart libvirtd
```

### Update AppArmor Configuration

Adjust the AppArmor configuration to allow access to the `/opt/nomad` paths used in the examples:

``` shell-session
printf "/opt/nomad rw,\n/opt/nomad/** rwk,\n" > /etc/apparmor.d/abstractions/libvirt-qemu.d/nomad
systemctl restart apparmor
```

### Enable Port Forwards

Port forwards in the example are to localhost. Enabling routing of localnet packets is 
required for this to be function. Enable the routing:

``` shell-session
sysctl -w net.ipv4.conf.all.route_localnet=1
```

## Setup

Build and install the nomad-driver-virt plugin. Run these commands from the root of the repository:

``` shell-session
make dev 
mkdir -p /opt/nomad/plugins
cp ./build/nomad-driver-virt /opt/nomad/plugins/nomad-driver-virt
```
