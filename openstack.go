package main

import (
	"github.com/urfave/cli"
)

var openStackFlags = []cli.Flag{
	cli.StringFlag{
		Name:   "os-username",
		Usage:  "OpenStack username.",
		EnvVar: "OS_USERNAME",
	},
	cli.StringFlag{
		Name:   "os-user-id",
		Usage:  "OpenStack user ID.",
		EnvVar: "OS_USER_ID",
	},
	cli.StringFlag{
		Name:   "os-password",
		Usage:  "OpenStack password.",
		EnvVar: "OS_PASSWORD",
	},
	cli.StringFlag{
		Name:   "os-project-name",
		Usage:  "OpenStack Tenant/Project name.",
		EnvVar: "OS_TENANT_NAME,OS_PROJECT_NAME",
	},
	cli.StringFlag{
		Name:   "os-project-id",
		Usage:  "OpenStack Tenant/Project ID.",
		EnvVar: "OS_TENANT_ID,OS_PROJECT_ID",
	},
	cli.StringFlag{
		Name:   "os-domain-name",
		Usage:  "OpenStack Domain Name.",
		EnvVar: "OS_DOMAIN_NAME",
	},
	cli.StringFlag{
		Name:   "os-domain-id",
		Usage:  "OpenStack Domain ID.",
		EnvVar: "OS_DOMAIN_ID",
	},
	cli.StringFlag{
		Name:   "os-token",
		Usage:  "OpenStack Token.",
		EnvVar: "OS_TOKEN",
	},
	cli.StringFlag{
		Name:   "os-auth-url",
		Usage:  "OpenStack Auth URL.",
		EnvVar: "OS_AUTH_URL",
	},
	cli.StringFlag{
		Name:   "os-region-name",
		Usage:  "OpenStack Region Name.",
		EnvVar: "OS_REGION_NAME",
	},
	cli.StringFlag{
		Name:   "os-cacert",
		Usage:  "OpenStack CA certificate.",
		EnvVar: "OS_CACERT",
	},
	cli.StringFlag{
		Name:   "os-insecure",
		Usage:  "Disable SSL verification.",
		EnvVar: "OS_INSECURE",
	},
	cli.BoolFlag{
		Name:   "os-swauth",
		Usage:  "Usage native Swift authentication.",
		EnvVar: "OS_SWAUTH",
	},
}
