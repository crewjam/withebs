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

    # ./withebs -verbose -volume=vol-7265549c touch foo ; ./withebs -verbose -volume=vol-7265549c touch foo
    attaching vol-7265549c to /dev/sdc
    mounting /dev/xvdc on /ebs/vol-7265549c
    invoking touch []string{"foo"}
    unmounting /dev/xvdc from /ebs/vol-7265549c
    detaching vol-7265549c from /dev/sdc
    attaching vol-7265549c to /dev/sdd
    failed to attach vol-7265549c at /dev/sdd: VolumeInUse: vol-7265549c is already attached to an instance
        status code: 400, request id: []
