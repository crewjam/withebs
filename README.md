# withebs

Withebs runs a command with the specified EBS volume attached to the currently running EC2 instance.

Usage:

    withebs --volume=$VOLUME_ID docker run -v /ebs/$VOLUME_ID:/data training/webapp

The volume is mounted at `/ebs/$VOLUME_ID`. If the volume does not contain a recognized filesystem, it is formatted with *mkfs* before mounting.

Options:

- `-volume` - which volume to mount.
- `-attach-timeout` - how long to wait for the EBS volume to successfully attach to the instance. (Default: 1m30s)
- `-fs` - Which filesystem to create on the volume if one does not already exist. (Default: `ext4`)
- `-mountpoint` - Where to mount the volume. (Default: `/ebs/` *volume_id*)
- `-mount` - Only mount the volume, don't run a command or unmount
- `-unmount` - Only unmount the volume

It takes about 6s to complete the full attach/run/detach cycle:

    # time ./withebs -volume=vol-12345678 touch /ebs/vol-12345678/foo
    real 0m6.878s
    user 0m0.065s
    sys  0m0.038s

Note that the program does *not* wait for the detach process to complete by default. In particular, you may not be able to attach the volume again immediately.

    # ./withebs -verbose -volume=vol-12345678 touch foo ; ./withebs -verbose -volume=vol-12345678 touch foo
    attaching vol-12345678 to /dev/sdc
    mounting /dev/xvdc on /ebs/vol-12345678
    invoking touch []string{"foo"}
    unmounting /dev/xvdc from /ebs/vol-12345678
    detaching vol-12345678 from /dev/sdc
    attaching vol-12345678 to /dev/sdd
    failed to attach vol-12345678 at /dev/sdd: VolumeInUse: vol-7265549c is already attached to an instance
        status code: 400, request id: []

Here is an example systemd unit file for using withebs and Docker together.

```
[Unit]
Description=Foo Daemon
After=docker.service

[Service]
TimeoutStartSec=0
ExecStartPre=-/usr/bin/docker rm -f foo
ExecStartPre=/usr/bin/docker pull crewjam/foo

ExecStartPre=/bin/sh -c 'set -ex \
  [ -e /opt/bin/withebs ] || exit 0; \
  [ -d /opt/bin ] || mkdir -p /opt/bin; \
  curl -sSL -o /opt/bin/withebs https://github.com/crewjam/withebs/releases/download/v1.1/withebs; \
  chmod +x /opt/bin/withebs'
ExecStartPre=/opt/bin/withebs -volume=vol-12345678 -mountpoint=/mnt/foo -mount
ExecStopPost=/opt/bin/withebs -volume=vol-12345678 -mountpoint=/mnt/foo -unmount

ExecStart=/usr/bin/docker run \
  -v /mnt/foo:/data \
  --name=foo \
  crewjam/foo
ExecStop=/usr/bin/docker stop foo
```
