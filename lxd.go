package main

import (
	"os"

	"github.com/jtopjian/limbo/lib"
	"github.com/urfave/cli"
)

var lxdFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "lxd-config-directory",
		Usage: "LXD Config Directory.",
		Value: os.ExpandEnv("$HOME/.config/lxc"),
	},
	cli.StringFlag{
		Name:  "name",
		Usage: "Name of the LXD resource.",
	},
	cli.StringFlag{
		Name:  "type",
		Usage: "Type of LXD resource (container, image).",
		Value: "container",
	},
	cli.BoolFlag{
		Name:  "stop",
		Usage: "Stop the LXD container if it's running.",
	},
	cli.StringSliceFlag{
		Name:  "property",
		Usage: "key=value properties for the new image. Can be repeated.",
	},
	cli.StringSliceFlag{
		Name:  "alias",
		Usage: "aliases for the new image. Can be repeated.",
	},
	cli.StringFlag{
		Name:  "compression",
		Usage: "Type of compression to use (bzip2, gzip, lzma, xz, or none).",
		Value: "gzip",
	},
	cli.StringFlag{
		Name:  "tmpdir",
		Usage: "Local temporary directory to store LXD resources.",
		Value: "/tmp",
	},
}

func newLXDConfig(configDirectory string) (lib.LXDConfig, error) {
	lxdConfig := lib.LXDConfig{
		ConfigDirectory: configDirectory,
	}

	return lib.NewLXDConfig(lxdConfig)
}
