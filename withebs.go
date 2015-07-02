package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/ec2"
	docker "github.com/fsouza/go-dockerclient"
)

var volumeID = flag.String("volume", "", "the ARN of the load balancer")
var dockerEndpoint = flag.String("docker", "unix:///var/run/docker.sock",
	"The path to the Docker endpoint")

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
	dockerClient, err := docker.NewClient(*dockerEndpoint)
	if err != nil {
		return fmt.Errorf("cannot connect to docker: %s", err)
	}

	instanceID := aws.InstanceId()
	if instanceID == "unknown" {
		return fmt.Errorf("cannot determine AWS instance ID. not running in EC2?")
	}

	awsAuth, err := aws.GetAuth("", "", "", time.Time{})
	if err != nil {
		return fmt.Errorf("cannot get AWS auth: %s\n", err)
	}

	ec2Conn := ec2.New(awsAuth, aws.GetRegion(aws.InstanceRegion()))

	deviceName := ""
	for i := 'a'; i < 'z'; i++ {
		if _, err := os.Stat(fmt.Sprintf("/dev/xvd%s", i)); err != nil {
			if os.IsNotExist(err) {
				deviceName = fmt.Sprintf("/dev/sd%c", i)
				break
			}
		}
	}

	_, err = ec2.AttachVolume(*volumeID, instanceID, deviceName)
	if err != nil {
		return fmt.Errorf("failed to attach %s at %s: %s",
			*volumeID, deviceName, err)
	}
	defer func() {
		if _, err := ec2.DetachVolume(*volumeID); err != nil {
			fmt.Fprintf(os.Stderr, "failed to detach %s: %s\n", *volumeID, err)
		}
	}()

	// Wait for the volume to attach
	deviceName = strings.Replace(deviceName, "/dev/sd", "/dev/xvd")
	for i := time.Duration(0); i < *attachTimeout; i += time.Second {
		_, err = os.Stat(deviceName)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		fmt.Printf("failed to attach %s: %s\n", deviceName, err)
	}

	// Use blkid to tell if we need to create a filesystem
	output, err := exec.Command("blkid", deviceName).Output()
	if err != nil && err.Error() == "exit status 2" {
		// blkid told us we have no filesystem, so create one
		fmt.Printf("Creating filesystem on %s", *volumeID)
		err = exec.Command(fmt.Sprintf("mkfs.%s", *fsType), deviceName).Run()
		if err != nil {
			return err
		}
	}

	// Mount the file system
	if *mountPoint == "" {
		*mountPoint = "/ebs/" + *volumeID
	}
	fmt.Printf("mounting %s on %s\n", deviceName, *mountPoint)
	os.MkdirAll(*mountPoint, 0777)
	err = exec.Command("mount", deviceName, *mountPoint).Run()
	if err != nil {
		return fmt.Errorf("cannot mount %s: %s", *volumeID, err)
	}
	defer func() {
		fmt.Printf("unmounting %s from %s\n", deviceName, *mountPoint)
		err = exec.Command("umount", deviceName, *mountPoint).Run()
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
