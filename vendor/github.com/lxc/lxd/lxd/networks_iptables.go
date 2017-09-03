package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lxc/lxd/shared"
)

func networkIptablesPrepend(protocol string, netName string, table string, chain string, rule ...string) error {
	cmd := "iptables"
	if protocol == "ipv6" {
		cmd = "ip6tables"
	}

	_, err := exec.LookPath(cmd)
	if err != nil {
		return fmt.Errorf("Asked to setup %s firewalling but %s can't be found", protocol, cmd)
	}

	baseArgs := []string{"-w"}
	if table == "" {
		table = "filter"
	}
	baseArgs = append(baseArgs, []string{"-t", table}...)

	// Check for an existing entry
	args := append(baseArgs, []string{"-C", chain}...)
	args = append(args, rule...)
	args = append(args, "-m", "comment", "--comment", fmt.Sprintf("generated for LXD network %s", netName))
	_, err = shared.RunCommand(cmd, args...)
	if err == nil {
		return nil
	}

	// Add the rule
	args = append(baseArgs, []string{"-I", chain}...)
	args = append(args, rule...)
	args = append(args, "-m", "comment", "--comment", fmt.Sprintf("generated for LXD network %s", netName))

	_, err = shared.RunCommand(cmd, args...)
	if err != nil {
		return err
	}

	return nil
}

func networkIptablesClear(protocol string, netName string, table string) error {
	// Detect kernels that lack IPv6 support
	if !shared.PathExists("/proc/sys/net/ipv6") && protocol == "ipv6" {
		return nil
	}

	cmd := "iptables"
	if protocol == "ipv6" {
		cmd = "ip6tables"
	}

	_, err := exec.LookPath(cmd)
	if err != nil {
		return nil
	}

	baseArgs := []string{"-w"}
	if table == "" {
		table = "filter"
	}
	baseArgs = append(baseArgs, []string{"-t", table}...)

	// List the rules
	args := append(baseArgs, "-S")
	output, err := shared.RunCommand(cmd, args...)
	if err != nil {
		return fmt.Errorf("Failed to list %s rules for %s (table %s)", protocol, netName, table)
	}

	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, fmt.Sprintf("generated for LXD network %s", netName)) {
			continue
		}

		// Remove the entry
		fields := strings.Fields(line)
		fields[0] = "-D"

		args = append(baseArgs, fields...)
		_, err = shared.RunCommand("sh", "-c", fmt.Sprintf("%s %s", cmd, strings.Join(args, " ")))
		if err != nil {
			return err
		}
	}

	return nil
}
