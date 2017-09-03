package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/jtopjian/limbo/lib"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// cmdExportSwift defines a cli command to export an LXD resource to Swift.
var cmdExportSwift = cli.Command{
	Name:     "swift",
	Usage:    "Swift Driver",
	Action:   actionExportSwift,
	Category: "export",
}

func init() {
	cmdExportSwift.Flags = append(cmdExportSwift.Flags, lxdFlags...)
	cmdExportSwift.Flags = append(cmdExportSwift.Flags, swiftFlags...)
	cmdExportSwift.Flags = append(cmdExportSwift.Flags, openStackFlags...)
	cmdExportSwift.Flags = append(cmdExportSwift.Flags, cryptFlags...)
}

// actionExportSwift implements the actions to export an LXD resource
// and upload it to Swift.
func actionExportSwift(ctx *cli.Context) error {
	log := logrus.New()
	if ctx.GlobalBool("debug") {
		log.Level = logrus.DebugLevel
	}

	// A name is required.
	lxdContainerName := ctx.String("name")
	if lxdContainerName == "" {
		return fmt.Errorf("must specify --name")
	}
	log.Debugf("Source name is: %s", lxdContainerName)

	// A storage container name is required.
	storageContainerName := ctx.String("storage-container")
	if storageContainerName == "" {
		return fmt.Errorf("must specify --storage-container")
	}
	log.Debugf("Storage container name is: %s", storageContainerName)

	// Set some other variables.
	lxdResourceType := ctx.String("type")
	log.Debugf("LXD resource type is: %s", lxdResourceType)

	localTmpDir := ctx.String("tmpdir")
	log.Debugf("Local tmpdir is: %s", localTmpDir)

	lxdConfigDirectory := ctx.String("lxd-config-directory")
	log.Debugf("LXD config directory is: %s", lxdConfigDirectory)

	stopLXDContainer := ctx.Bool("stop")
	log.Debugf("Stop container if it's running: %t", stopLXDContainer)

	createStorageContainer := ctx.Bool("create-storage-container")
	log.Debugf("Create storage container if it doesn't exist: %t", createStorageContainer)

	archive := ctx.Bool("archive")
	log.Debugf("Images will be archived: %t", archive)

	// Because the exported image might be large, save it locally temporarily
	// instead of in memory.
	log.Debugf("Creating tmpdir %s", localTmpDir)
	tmpDir, err := ioutil.TempDir(localTmpDir, "limbo")
	if err != nil {
		return fmt.Errorf("Unable to create temp directory: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an LXD client.
	lxdConfig, err := newLXDConfig(lxdConfigDirectory)
	if err != nil {
		return fmt.Errorf("Unable to get LXD configuration: %s", err)
	}

	// Determine the LXD remote name and resource name.
	remote, ctName, err := lxdConfig.Config.ParseRemote(lxdContainerName)
	if err != nil {
		return fmt.Errorf("Unable to parse LXD remote: %s", err)
	}
	lxdConfig.RemoteName = remote
	log.Debugf("LXD Remote: %s", remote)
	log.Debugf("LXD Container name: %s", ctName)

	// Create a connection to the LXD container server API.
	log.Debugf("Creating connection to LXD container server %s", remote)
	lxdServer, err := lxdConfig.GetContainerServer()
	if err != nil {
		return fmt.Errorf("Unable to connect to LXD Server: %s", err)
	}

	// Get a Swift client.
	log.Debug("Creating swift client")
	swiftClient, err := newSwiftClient(ctx)
	if err != nil {
		return fmt.Errorf("Unable to create swift client: %s", err)
	}

	// See if the destination storage container exists.
	// Configure the container to archive, if requested.
	log.Debug("Configuring Swift container")
	err = lib.SwiftCreateContainer(swiftClient, storageContainerName, createStorageContainer, archive)
	if err != nil {
		return err
	}

	var lxdFingerprint string
	if lxdResourceType == "container" {
		// First publish the LXD container.
		// This converts a container to an image.
		publishOpts := lib.LXDPublishOpts{
			Name:                 ctName,
			Stop:                 stopLXDContainer,
			CompressionAlgorithm: ctx.String("compression"),
		}

		if len(ctx.StringSlice("property")) > 0 {
			properties := map[string]string{}
			for _, v := range ctx.StringSlice("property") {
				if strings.Contains(v, "=") {
					v := strings.Split(v, "=")
					properties[v[0]] = v[1]
				}
			}

			publishOpts.Properties = &properties
		}
		log.Debugf("LXD PublishOpts: %#v", publishOpts)

		log.Infof("Creating an image from %s", ctName)
		publishResult, err := lib.LXDPublishImage(lxdConfig, publishOpts)
		if err != nil {
			return fmt.Errorf("Unable to publish image: %s", err)
		}
		log.Debugf("LXD PublishResult: %#v", publishResult)

		lxdFingerprint = publishResult.Fingerprint

		// Delete the newly created image when the job is done.
		defer func() {
			op, err := lxdServer.DeleteImage(publishResult.Fingerprint)
			if err != nil {
				panic(err)
			}

			if err := op.Wait(); err != nil {
				panic(err)
			}
		}()
	}

	// If lxdFingerprint doesn't have a value, that means an existing container
	// wasn't exported and we might be dealing with an image. Try to get the
	// fingerprint of the image.
	if lxdFingerprint == "" {
		lxdImage, err := lxdConfig.GetImageServer()
		if err != nil {
			return fmt.Errorf("Unable to connect to LXD image API: %s", err)
		}

		result, _, err := lxdImage.GetImageAlias(ctName)
		if result == nil {
			lxdFingerprint = ctName
		}

		lxdFingerprint = result.Target
	}

	// Download the image locally.
	downloadOpts := lib.LXDDownloadOpts{
		Name:        ctName,
		TmpDir:      tmpDir,
		Fingerprint: lxdFingerprint,
	}
	log.Debugf("LXD downloadOpts: %#v", downloadOpts)

	log.Infof("Downloading image of %s", ctName)
	downloadResult, err := lib.LXDDownloadImage(lxdConfig, downloadOpts)
	if err != nil {
		return fmt.Errorf("Unable to download lxd image: %s", err)
	}
	log.Debugf("LXD downloadResult: %#v", downloadResult)

	// Encrypt the file, if requested.
	if ctx.Bool("encrypt") {
		log.Infof("Encrypting %s", downloadResult.MetaFilename)
		err = lib.Encrypt(downloadResult.MetaFilename, ctx.String("pass"))
		if err != nil {
			return fmt.Errorf("Unable to encrypt %s: %s", downloadResult.MetaFilename, err)
		}
	}

	// Upload the image to Swift.
	objectName := ctx.String("object-name")
	if objectName == "" {
		objectName = ctName
	}

	// First upload the meta file.
	uploadOpts := lib.SwiftUploadOpts{
		ObjectName:       objectName,
		SourceName:       downloadResult.MetaFilename,
		StorageContainer: storageContainerName,
	}
	log.Debugf("Swift uploadOpts: %#v", uploadOpts)

	log.Infof("Uploading %s to Swift container %s as %s",
		ctName, uploadOpts.StorageContainer, uploadOpts.ObjectName)

	uploadResult, err := lib.SwiftUploadObject(swiftClient, uploadOpts)
	if err != nil {
		return fmt.Errorf("Unable to upload meta file to swift: %s", err)
	}
	log.Debugf("Upload result headers: %#v", uploadResult.Headers)

	// Then upload the rootfs file, if it exists.
	if downloadResult.RootfsFilename != "" {
		// Encrypt the file, if requested.
		if ctx.Bool("encrypt") {
			log.Infof("Encrypting %s", downloadResult.RootfsFilename)
			err = lib.Encrypt(downloadResult.RootfsFilename, ctx.String("pass"))
			if err != nil {
				return fmt.Errorf("Unable to encrypt %s: %s", downloadResult.RootfsFilename, err)
			}
		}

		uploadOpts.SourceName = downloadResult.RootfsFilename
		uploadOpts.ObjectName = objectName + ".root"
		log.Debugf("Swift uploadOpts: %#v", uploadOpts)

		log.Infof("Uploading %s rootfs to Swift container %s as %s",
			ctName, uploadOpts.StorageContainer, uploadOpts.ObjectName)

		uploadResult, err = lib.SwiftUploadObject(swiftClient, uploadOpts)
		if err != nil {
			return fmt.Errorf("Unable to upload meta file to swift: %s", err)
		}
		log.Debugf("Upload result headers: %#v", uploadResult.Headers)
	}

	log.Infof("Successfully exported %s", ctName)
	return nil
}
