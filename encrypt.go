package main

import (
	"github.com/urfave/cli"
)

var cryptFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "encrypt",
		Usage: "encrypt/decrypt the image",
	},
	cli.StringFlag{
		Name:   "pass",
		Usage:  "passphrase for encryption/decryption",
		EnvVar: "LIMBO_PASS,PASS",
	},
}
