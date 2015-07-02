# withebs

Withebs runs your command with the specified EBS volume attached to the currently running EC2 instance.

Usage:

    withebs --volume=$VOLUME_ID docker run -v /ebs/$VOLUME_ID:/data training/webapp

The volume is mounted at /ebs/$VOLUME_ID. If the volume does not contain a recognized filesystem, it is formatted with an ext4 filesystem. (You can change that with -mkfs-command.)


