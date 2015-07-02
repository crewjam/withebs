package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	oldaws "github.com/crowdmob/goamz/aws"
)

var volumeID = flag.String("volume", "", "The volume ID to mount")
var mountPoint = flag.String("mount", "", "Where to mount the volume")
var fsType = flag.String("fs", "ext4",
	"Which filesystem to create on the volume if one does not already exist")
var attachTimeout = flag.Duration("attach-timeout", 90*time.Second,
	"how long to wait for the EBS volume to successfully attach to the instance")

func main() {
	flag.Parse()
	if err := Main(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func Main() error {
	instanceID := oldaws.InstanceId()
	if instanceID == "unknown" {
		return fmt.Errorf("cannot determine AWS instance ID. not running in EC2?")
	}

	region := oldaws.InstanceRegion()

	linuxDeviceName := ""
	awsDeviceName := ""
	for i := 'a'; i < 'z'; i++ {
		if _, err := os.Stat(fmt.Sprintf("/dev/xvd%s", i)); err != nil {
			if os.IsNotExist(err) {
				awsDeviceName = fmt.Sprintf("/dev/sd%c", i)
				linuxDeviceName = fmt.Sprintf("/dev/xvd%c", i)
				break
			}
		}
	}

	ec2Conn := ec2.New(&aws.Config{Region: region})
	_, err := ec2Conn.AttachVolume(&ec2.AttachVolumeInput{
		Device:     &awsDeviceName,
		InstanceID: &instanceID,
		VolumeID:   volumeID,
	})
	if err != nil {
		return fmt.Errorf("failed to attach %s at %s: %s",
			*volumeID, awsDeviceName, err)
	}
	defer func() {
		if _, err := ec2Conn.DetachVolume(&ec2.DetachVolumeInput{
			Device:     &awsDeviceName,
			InstanceID: &instanceID,
			VolumeID:   volumeID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to detach %s: %s\n", *volumeID, err)
		}
	}()

	// Wait for the volume to attach
	for i := time.Duration(0); i < *attachTimeout; i += time.Second {
		_, err = os.Stat(linuxDeviceName)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		fmt.Printf("failed to attach %s: %s\n", linuxDeviceName, err)
	}

	// Use blkid to tell if we need to create a filesystem
	_, err = exec.Command("blkid", linuxDeviceName).Output()
	if err != nil && err.Error() == "exit status 2" {
		// blkid told us we have no filesystem, so create one
		fmt.Printf("Creating filesystem on %s", *volumeID)
		err = exec.Command(fmt.Sprintf("mkfs.%s", *fsType), linuxDeviceName).Run()
		if err != nil {
			return err
		}
	}

	// Mount the file system
	if *mountPoint == "" {
		*mountPoint = "/ebs/" + *volumeID
	}
	fmt.Printf("mounting %s on %s\n", linuxDeviceName, *mountPoint)
	os.MkdirAll(*mountPoint, 0777)
	err = exec.Command("mount", linuxDeviceName, *mountPoint).Run()
	if err != nil {
		return fmt.Errorf("cannot mount %s: %s", *volumeID, err)
	}
	defer func() {
		fmt.Printf("unmounting %s from %s\n", linuxDeviceName, *mountPoint)
		err = exec.Command("umount", linuxDeviceName, *mountPoint).Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to unmount %s: %s\n", *mountPoint, err)
		}
	}()

	// Invoke the command
	err = exec.Command(flag.Arg(0), flag.Args()[1:]...).Run()
	if err != nil {
		return fmt.Errorf("%s: %s", flag.Arg(0), err)
	}

	return nil
}
