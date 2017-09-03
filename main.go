package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "limbo"
	app.Usage = "LXD Image Management"
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug,d",
			Usage:  "debug mode",
			EnvVar: "LIMBO_DEBUG,DEBUG",
		},
	}

	app.Commands = []cli.Command{
		cli.Command{
			Name:  "export",
			Usage: "export an LXD resource",
			Subcommands: []cli.Command{
				cmdExportSwift,
			},
		},
		cli.Command{
			Name:  "import",
			Usage: "import an LXD image",
			Subcommands: []cli.Command{
				cmdImportSwift,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}
