package lib

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	lxd "github.com/lxc/lxd/client"
	lxd_config "github.com/lxc/lxd/lxc/config"
	lxd_shared "github.com/lxc/lxd/shared"
	lxd_api "github.com/lxc/lxd/shared/api"
)

type LXDConfig struct {
	Config          *lxd_config.Config
	RemoteName      string
	ConfigDirectory string
}

func NewLXDConfig(c LXDConfig) (LXDConfig, error) {
	c.getConfig()
	return c, nil
}

func (r *LXDConfig) getConfig() {
	if conf, err := lxd_config.LoadConfig(r.ConfigDirectory); err != nil {
		r.Config = &lxd_config.DefaultConfig
		r.Config.ConfigDir = r.ConfigDirectory
	} else {
		r.Config = conf
	}
}

func (r LXDConfig) GetContainerServer() (lxd.ContainerServer, error) {
	if r.Config == nil {
		r.getConfig()
	}

	return r.Config.GetContainerServer(r.RemoteName)
}

func (r LXDConfig) GetImageServer() (lxd.ImageServer, error) {
	if r.Config == nil {
		r.getConfig()
	}

	return r.Config.GetImageServer(r.RemoteName)
}

type LXDPublishOpts struct {
	Name                 string
	Stop                 bool
	Properties           *map[string]string
	CompressionAlgorithm string
}

type LXDPublishResult struct {
	Fingerprint string
}

// The following is a loose re-implementation of `lxc publish`.
// https://github.com/lxc/lxd/blob/master/lxc/publish.go
// This will create an image out of a container.
// If the container is ephemeral, it will temporarily be made non-ephemeral.
// If the container is running, it will temporarily be stopped.
func LXDPublishImage(lxdConfig LXDConfig, opts LXDPublishOpts) (*LXDPublishResult, error) {
	lxdServer, err := lxdConfig.GetContainerServer()
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to LXD container server: %s", err)
	}

	if !lxd_shared.IsSnapshot(opts.Name) {
		// Attempt to get the LXD container
		lxdContainer, etag, err := lxdServer.GetContainer(opts.Name)
		if err != nil {
			return nil, fmt.Errorf("Unable to get container: %s", err)
		}

		wasRunning := lxdContainer.StatusCode != 0 && lxdContainer.StatusCode != lxd_api.Stopped
		wasEphemeral := lxdContainer.Ephemeral

		if wasRunning {
			if !opts.Stop {
				err := fmt.Errorf("LXD Container is running. Use --stop to stop the container")
				return nil, err
			}

			if lxdContainer.Ephemeral {
				lxdContainer.Ephemeral = false
				op, err := lxdServer.UpdateContainer(opts.Name, lxdContainer.Writable(), etag)
				if err != nil {
					return nil, fmt.Errorf("Unable to update container: %s", err)
				}

				if err := op.Wait(); err != nil {
					return nil, fmt.Errorf("Problem waiting for container to update: %s", err)
				}

				_, etag, err = lxdServer.GetContainer(opts.Name)
				if err != nil {
					return nil, fmt.Errorf("Unable to get container: %s", err)
				}
			}

			stopReq := lxd_api.ContainerStatePut{
				Action:  "stop",
				Timeout: -1,
				Force:   false,
			}

			op, err := lxdServer.UpdateContainerState(opts.Name, stopReq, "")
			if err != nil {
				return nil, err
			}

			if err := op.Wait(); err != nil {
				return nil, fmt.Errorf("Problem waiting for container to stop: %s", err)
			}
			defer func() {
				stopReq.Action = "start"
				op, err = lxdServer.UpdateContainerState(opts.Name, stopReq, "")
				if err != nil {
					panic(err)
				}

				if err := op.Wait(); err != nil {
					panic(err)
				}
			}()

			if wasEphemeral {
				lxdContainer.Ephemeral = true
				op, err := lxdServer.UpdateContainer(opts.Name, lxdContainer.Writable(), etag)
				if err != nil {
					return nil, fmt.Errorf("Unable to update container: %s", err)
				}

				if err := op.Wait(); err != nil {
					return nil, fmt.Errorf("Problem waiting for container to update: %s", err)
				}
			}
		}
	}

	// Create an image out of the container.
	imageCreateReq := lxd_api.ImagesPost{
		Source: &lxd_api.ImagesPostSource{
			Type: "container",
			Name: opts.Name,
		},
		CompressionAlgorithm: opts.CompressionAlgorithm,
	}

	imageCreateReq.Properties = nil
	if opts.Properties != nil {
		imageCreateReq.Properties = *opts.Properties
	}

	if lxd_shared.IsSnapshot(opts.Name) {
		imageCreateReq.Source.Type = "snapshot"
	}

	op, err := lxdServer.CreateImage(imageCreateReq, nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to create image from container: %s", err)
	}

	if err := op.Wait(); err != nil {
		return nil, fmt.Errorf("Problem waiting for image to create: %s", err)
	}

	fingerprint := op.Metadata["fingerprint"].(string)
	p := &LXDPublishResult{
		Fingerprint: fingerprint,
	}

	return p, nil
}

type LXDDownloadOpts struct {
	Name        string
	TmpDir      string
	Fingerprint string
}

type LXDDownloadResult struct {
	MetaFilename   string
	RootfsFilename string
}

// This is a loose re-implementation of lxc image export:
// https://github.com/lxc/lxd/blob/master/lxc/image.go
// This will download an image to a temporary directory.
func LXDDownloadImage(lxdConfig LXDConfig, opts LXDDownloadOpts) (*LXDDownloadResult, error) {
	lxdServer, err := lxdConfig.GetContainerServer()
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to LXD container server: %s", err)
	}

	targetMeta := filepath.Join(opts.TmpDir, opts.Name)
	destMeta, err := os.Create(targetMeta)
	if err != nil {
		return nil, fmt.Errorf("Unable to create temporary meta file: %s", err)
	}

	targetRootfs := filepath.Join(opts.TmpDir, opts.Name+".root")
	destRoot, err := os.Create(targetRootfs)
	if err != nil {
		return nil, fmt.Errorf("Unable to create temporary root file: %s", err)
	}

	imageDownloadReq := lxd.ImageFileRequest{
		MetaFile:   destMeta,
		RootfsFile: destRoot,
	}

	resp, err := lxdServer.GetImageFile(opts.Fingerprint, imageDownloadReq)
	if err != nil {
		os.Remove(targetMeta)
		os.Remove(targetRootfs)
		return nil, fmt.Errorf("Unable to download image: %s", err)
	}

	uploadRoot := true
	if resp.RootfsSize == 0 {
		uploadRoot = false
		err := os.Remove(targetRootfs)
		if err != nil {
			os.Remove(targetMeta)
			os.Remove(targetRootfs)
			return nil, fmt.Errorf("Unable to remove temporary root file: %s", err)
		}
	}

	if resp.MetaName != "" {
		newName := filepath.Join(opts.TmpDir, resp.MetaName)
		err := os.Rename(targetMeta, newName)
		if err != nil {
			os.Remove(targetMeta)
			os.Remove(targetRootfs)
			return nil, fmt.Errorf("Unable to rename meta file: %s", err)
		}
		targetMeta = newName
	}

	if resp.RootfsSize > 0 && resp.RootfsName != "" {
		newName := filepath.Join(opts.TmpDir, resp.RootfsName)
		err := os.Rename(targetRootfs, newName)
		if err != nil {
			os.Remove(targetMeta)
			os.Remove(targetRootfs)
			return nil, fmt.Errorf("Unable to rename root file: %s", err)
		}
		targetRootfs = newName
	}

	d := &LXDDownloadResult{
		MetaFilename: targetMeta,
	}

	if uploadRoot {
		d.RootfsFilename = targetRootfs
	}

	return d, nil
}

type LXDImportOpts struct {
	Aliases        []string
	Name           string
	MetaFilename   string
	RootfsFilename string
	TmpDir         string
}

type LXDImportResult struct {
	Fingerprint string
}

// This is a loose re-implementation of "lxc import".
// https://github.com/lxc/lxd/blob/master/lxc/image.go
func LXDImportImage(lxdConfig LXDConfig, opts LXDImportOpts) (*LXDImportResult, error) {
	lxdServer, err := lxdConfig.GetContainerServer()
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to LXD container server: %s", err)
	}

	image := lxd_api.ImagesPost{}

	var meta io.ReadCloser
	var rootfs io.ReadCloser

	meta, err = os.Open(opts.MetaFilename)
	if err != nil {
		return nil, fmt.Errorf("Unable to open meta file %s: %s", opts.MetaFilename, err)
	}
	defer meta.Close()

	if opts.RootfsFilename != "" {
		rootfs, err = os.Open(opts.RootfsFilename)
		if err != nil {
			return nil, fmt.Errorf("Unable to open rootfs file %s: %s", opts.RootfsFilename, err)
		}
		defer rootfs.Close()
	}

	args := &lxd.ImageCreateArgs{
		MetaFile:   meta,
		MetaName:   filepath.Base(opts.MetaFilename),
		RootfsFile: rootfs,
		RootfsName: filepath.Base(opts.RootfsFilename),
	}

	image.Filename = opts.Name

	op, err := lxdServer.CreateImage(image, args)
	if err != nil {
		return nil, fmt.Errorf("Unable to create image: %s", err)
	}

	err = op.Wait()
	if err != nil {
		return nil, fmt.Errorf("Error saving image: %s", err)
	}

	fingerprint := op.Metadata["fingerprint"].(string)

	// Set the name and aliases of the image
	opts.Aliases = append(opts.Aliases, opts.Name)
	for _, v := range opts.Aliases {
		aliasPost := lxd_api.ImageAliasesPost{}
		aliasPost.Name = v
		aliasPost.Target = fingerprint
		if err := lxdServer.CreateImageAlias(aliasPost); err != nil {
			return nil, fmt.Errorf("Unable to set alias %s on %s", v, opts.Name)
		}
	}

	r := &LXDImportResult{
		Fingerprint: fingerprint,
	}

	return r, nil
}
