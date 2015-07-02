package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	oldaws "github.com/crowdmob/goamz/aws"
)

var verbose = flag.Bool("verbose", false, "Print progress messages")
var volumeID = flag.String("volume", "", "The volume ID to mount")
var mountPoint = flag.String("mountpoint", "", "Where to mount the volume")
var fsType = flag.String("fs", "ext4",
	"Which filesystem to create on the volume if one does not already exist")
var attachTimeout = flag.Duration("attach-timeout", 90*time.Second,
	"how long to wait for the EBS volume to successfully attach to the instance")
var mountOnly = flag.Bool("mount", false, "mount the volume and exit")
var unmountOnly = flag.Bool("unmount", false, "unmount the volume and exit")
var instanceID string
var log io.Writer

func main() {
	flag.Parse()
	if err := Main(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func Unmount() error {
	err := exec.Command("umount", *mountPoint).Run()
	if err != nil {
		if err.Error() == "exit status 32" {
			return nil
		}
		return fmt.Errorf("failed to unmount %s: %s\n", *mountPoint, err)
	}
	return nil
}

func Detach(ec2Conn *ec2.EC2) error {
	fmt.Fprintf(log, "detaching %s\n", *volumeID)
	_, err := ec2Conn.DetachVolume(&ec2.DetachVolumeInput{
		InstanceID: &instanceID,
		VolumeID:   volumeID,
	})
	if err != nil {
		return fmt.Errorf("failed to detach %s: %s\n", *volumeID, err)
	}
	return nil
}

func Main() error {
	log = os.Stderr
	if !*verbose {
		log = ioutil.Discard
	}

	if *mountPoint == "" {
		*mountPoint = "/ebs/" + *volumeID
	}

	instanceID = oldaws.InstanceId()
	if instanceID == "unknown" {
		return fmt.Errorf("cannot determine AWS instance ID. not running in EC2?")
	}
	region := oldaws.InstanceRegion()
	ec2Conn := ec2.New(&aws.Config{Region: region})

	if *unmountOnly {
		err1 := Unmount()
		err2 := Detach(ec2Conn)
		if err1 != nil {
			return err1
		}
		return err2
	}

	var err error

	linuxDeviceName := ""
	awsDeviceName := ""
	for i := 'a'; true; i++ {
		awsDeviceName = fmt.Sprintf("/dev/sd%c", i)
		linuxDeviceName = fmt.Sprintf("/dev/xvd%c", i)
		_, err = os.Stat(linuxDeviceName)
		if err != nil && os.IsNotExist(err) {
			fmt.Fprintf(log, "found device %s\n", linuxDeviceName)
			break
		}
		if err != nil {
			fmt.Fprintf(log, "%s: %s\n", linuxDeviceName, err)
		}
		if i == 'z' {
			return fmt.Errorf("Cannot locate an available device to mount")
		}
	}

	fmt.Fprintf(log, "attaching %s to %s\n", *volumeID, awsDeviceName)
	_, err = ec2Conn.AttachVolume(&ec2.AttachVolumeInput{
		Device:     &awsDeviceName,
		InstanceID: &instanceID,
		VolumeID:   volumeID,
	})
	if err != nil {
		return fmt.Errorf("failed to attach %s at %s: %s",
			*volumeID, awsDeviceName, err)
	}
	defer func() {
		fmt.Fprintf(log, "detaching %s from %s\n", *volumeID, awsDeviceName)
		if !*mountOnly || err != nil {
			if cleanupErr := Detach(ec2Conn); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "failed to detach %s: %s\n", *volumeID, cleanupErr)
			}
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
		return fmt.Errorf("failed to attach %s: %s\n", linuxDeviceName, err)
	}

	// Use blkid to tell if we need to create a filesystem
	_, err = exec.Command("blkid", linuxDeviceName).Output()
	if err != nil && err.Error() == "exit status 2" {
		// blkid told us we have no filesystem, so create one
		fmt.Fprintf(log, "Creating filesystem on %s\n", *volumeID)
		err = exec.Command(fmt.Sprintf("mkfs.%s", *fsType), linuxDeviceName).Run()
		if err != nil {
			return err
		}
	}

	// Mount the file system
	fmt.Fprintf(log, "mounting %s on %s\n", linuxDeviceName, *mountPoint)
	os.MkdirAll(*mountPoint, 0777)
	err = exec.Command("mount", linuxDeviceName, *mountPoint).Run()
	if err != nil {
		return fmt.Errorf("cannot mount %s: %s", *volumeID, err)
	}
	defer func() {
		if !*mountOnly || err != nil {
			fmt.Fprintf(log, "unmounting %s from %s\n", linuxDeviceName, *mountPoint)
			if cleanupErr := Unmount(); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "failed to unmount %s: %s\n", *volumeID, cleanupErr)
			}
		}
	}()

	if *mountOnly {
		return nil
	}

	// Invoke the command
	fmt.Fprintf(log, "invoking %s %#v\n", flag.Arg(0), flag.Args()[1:])
	cmd := exec.Command(flag.Arg(0), flag.Args()[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Catch shutdown signals and make sure we cleanup
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, os.Kill)

	// Run the command. When finished, close the signalCh to wake up the main
	// "thread".
	var cmdErr error
	go func() {
		cmdErr = cmd.Run()
		close(signalCh)
	}()

	// Wait for the command to stop or a signal to arrive
	_ = <-signalCh

	return cmdErr
}
