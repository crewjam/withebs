# withebs

Withebs runs a command with the specified EBS volume attached to the currently running EC2 instance.

Usage:

    withebs --volume=$VOLUME_ID docker run -v /ebs/$VOLUME_ID:/data training/webapp

The volume is mounted at `/ebs/$VOLUME_ID`. If the volume does not contain a recognized filesystem, it is formatted with *mkfs* before mounting.

Options:

- `-volume` - which volume to mount.
- `-attach-timeout` - how long to wait for the EBS volume to successfully attach to the instance. (Default: 1m30s)
- `-fs` - Which filesystem to create on the volume if one does not already exist. (Default: `ext4`)
- `-mount` - Where to mount the volume. (Default: `/ebs/` *volume_id*)
