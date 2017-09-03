package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/jtopjian/limbo/lib"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// cmdImportSwift defines a cli command to import an LXD resource from Swift.
var cmdImportSwift = cli.Command{
	Name:     "swift",
	Usage:    "Swift Driver",
	Action:   actionImportSwift,
	Category: "import",
}

func init() {
	cmdImportSwift.Flags = append(cmdImportSwift.Flags, lxdFlags...)
	cmdImportSwift.Flags = append(cmdImportSwift.Flags, swiftFlags...)
	cmdImportSwift.Flags = append(cmdImportSwift.Flags, openStackFlags...)
	cmdImportSwift.Flags = append(cmdImportSwift.Flags, cryptFlags...)
}

// actionImportSwift implements the actions to import an LXD resource
// from Swift.
func actionImportSwift(ctx *cli.Context) error {
	log := logrus.New()
	if ctx.GlobalBool("debug") {
		log.Level = logrus.DebugLevel
	}

	// A name is required.
	objectName := ctx.String("object-name")
	if objectName == "" {
		return fmt.Errorf("must specify --object-name")
	}
	log.Debugf("Source name is: %s", objectName)

	// A storage container name is required.
	storageContainerName := ctx.String("storage-container")
	if storageContainerName == "" {
		return fmt.Errorf("must specify --storage-container")
	}
	log.Debugf("Storage container name is: %s", storageContainerName)

	// Set some variables.
	localTmpDir := ctx.String("tmpdir")
	log.Debugf("Local tmpdir is: %s", localTmpDir)

	lxdConfigDirectory := ctx.String("lxd-config-directory")
	log.Debugf("LXD config directory is: %s", lxdConfigDirectory)

	// Because the downloaded image might be large, save it locally temporarily
	// instead of in memory.
	log.Debugf("Creating tmpdir %s", localTmpDir)
	tmpDir, err := ioutil.TempDir(localTmpDir, "limbo")
	if err != nil {
		return fmt.Errorf("Unable to create temp directory: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	// If --name is specified, use it. If not, use --object-name.
	lxdContainerName := objectName
	if v := ctx.String("name"); v != "" {
		lxdContainerName = v
	}

	// Get a Swift client.
	log.Debug("Creating swift client")
	swiftClient, err := newSwiftClient(ctx)
	if err != nil {
		return fmt.Errorf("Unable to create swift client: %s", err)
	}

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

	// Download the image from Swift.

	// First download the meta file.
	metaFilename := tmpDir + "/" + objectName
	downloadOpts := lib.SwiftDownloadOpts{
		Filename:         metaFilename,
		ObjectName:       objectName,
		StorageContainer: storageContainerName,
	}
	log.Debugf("Swift downloadOpts: %#v", downloadOpts)

	log.Infof("Downloading %s from Swift container %s as %s",
		objectName, storageContainerName, metaFilename)

	downloadResult, err := lib.SwiftDownloadObject(swiftClient, downloadOpts)
	if err != nil {
		return fmt.Errorf("Unable to download meta file to swift: %s", err)
	}
	log.Debugf("Download result headers: %#v", downloadResult.Headers)

	// Then download the rootfs file, if it exists.
	rootfsObjectName := objectName + ".root"
	rootfsFilename := metaFilename + ".root"
	downloadOpts.ObjectName = rootfsObjectName
	downloadOpts.Filename = rootfsFilename

	rootfsObjectExists := true
	downloadResult, err = lib.SwiftDownloadObject(swiftClient, downloadOpts)
	if err != nil {
		// If the error was a 404/does not exist
		if _, ok := err.(lib.ErrObjectDoesNotExist); !ok {
			return err
		}

		rootfsObjectExists = false
	}

	if rootfsObjectExists {
		log.Infof("Downloaded %s from Swift container %s as %s",
			rootfsObjectName, storageContainerName, rootfsFilename)
		log.Debugf("Download result headers: %#v", downloadResult.Headers)
	}

	// If the object is encrypted, decrypt it.
	if ctx.Bool("encrypt") {
		log.Infof("Decrypting %s", objectName)
		err = lib.Decrypt(metaFilename, ctx.String("pass"))
		if err != nil {
			return err
		}

		if rootfsObjectExists {
			log.Infof("Decrypting %s", rootfsObjectName)
			err = lib.Decrypt(rootfsFilename, ctx.String("pass"))
			if err != nil {
				return err
			}
		}
	}

	// Import the image into LXD.
	importOpts := lib.LXDImportOpts{
		Name:         ctName,
		MetaFilename: metaFilename,
		TmpDir:       tmpDir,
	}

	if rootfsObjectExists {
		importOpts.RootfsFilename = metaFilename + ".root"
	}

	if len(ctx.StringSlice("alias")) > 0 {
		aliases := []string{}
		for _, v := range ctx.StringSlice("alias") {
			aliases = append(aliases, v)
		}
		importOpts.Aliases = aliases
	}

	log.Infof("Importing %s", ctName)
	log.Debugf("LXD importOpts: %#v", importOpts)
	importResult, err := lib.LXDImportImage(lxdConfig, importOpts)
	if err != nil {
		return fmt.Errorf("Unable to import image %s: %s", ctName, err)
	}
	log.Debugf("LXD importResult: %#v", importResult)

	log.Infof("Successfully imported %s", ctName)
	return nil
}
