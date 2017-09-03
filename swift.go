package main

import (
	"github.com/gophercloud/gophercloud"
	"github.com/jtopjian/limbo/lib"

	"github.com/urfave/cli"
)

var swiftFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "storage-container",
		Usage: "Destination Swift Container.",
		Value: "limbo",
	},
	cli.BoolFlag{
		Name:  "create-storage-container",
		Usage: "Create storage container if it does not exist.",
	},
	cli.BoolFlag{
		Name:  "archive",
		Usage: "Enable archiving",
	},
	cli.StringFlag{
		Name:  "object-name",
		Usage: "Object name of the exported image.",
	},
}

func newSwiftClient(ctx *cli.Context) (*gophercloud.ServiceClient, error) {
	authOpts := lib.SwiftAuthOpts{
		DomainID:         ctx.String("os-domain-id"),
		DomainName:       ctx.String("os-domain-name"),
		IdentityEndpoint: ctx.String("os-auth-url"),
		Password:         ctx.String("os-password"),
		TenantID:         ctx.String("os-project-id"),
		TenantName:       ctx.String("os-project-name"),
		TokenID:          ctx.String("os-token"),
		Username:         ctx.String("os-username"),
		UserID:           ctx.String("os-user-id"),
		RegionName:       ctx.String("os-region-name"),
		CACert:           ctx.String("os-cacert"),
		Insecure:         ctx.Bool("os-insecure"),
		Swauth:           ctx.Bool("os-swauth"),
	}

	return lib.GetSwiftClient(authOpts)
}
