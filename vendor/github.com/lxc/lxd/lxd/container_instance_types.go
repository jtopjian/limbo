package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/lxc/lxd/lxd/util"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/logger"
	"github.com/lxc/lxd/shared/version"
)

type instanceType struct {
	// Amount of CPUs (can be a fraction)
	CPU float32 `yaml:"cpu"`

	// Amount of memory in GB
	Memory float32 `yaml:"mem"`
}

var instanceTypes map[string]map[string]*instanceType

func instanceSaveCache() error {
	if instanceTypes == nil {
		return nil
	}

	data, err := yaml.Marshal(&instanceTypes)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(shared.CachePath("instance_types.yaml"), data, 0600)
	if err != nil {
		return err
	}

	return nil
}

func instanceLoadCache() error {
	if !shared.PathExists(shared.CachePath("instance_types.yaml")) {
		return nil
	}

	content, err := ioutil.ReadFile(shared.CachePath("instance_types.yaml"))
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(content, &instanceTypes)
	if err != nil {
		return err
	}

	return nil
}

func instanceRefreshTypes(d *Daemon) error {
	logger.Info("Updating instance types")

	// Attempt to download the new definitions
	downloadParse := func(filename string, target interface{}) error {
		url := fmt.Sprintf("https://images.linuxcontainers.org/meta/instance-types/%s", filename)

		httpClient, err := util.HTTPClient("", d.proxy)
		if err != nil {
			return err
		}

		httpReq, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}

		httpReq.Header.Set("User-Agent", version.UserAgent)

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Failed to get %s", url)
		}

		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(content, target)
		if err != nil {
			return err
		}

		return nil
	}

	// Set an initial value from the cache
	if instanceTypes == nil {
		instanceLoadCache()
	}

	// Get the list of instance type sources
	sources := map[string]string{}
	err := downloadParse(".yaml", &sources)
	if err != nil {
		logger.Warnf("Failed to update instance types: %v", err)
		return err
	}

	// Parse the individual files
	newInstanceTypes := map[string]map[string]*instanceType{}
	for name, filename := range sources {
		types := map[string]*instanceType{}
		err = downloadParse(filename, &types)
		if err != nil {
			logger.Warnf("Failed to update instance types: %v", err)
			return err
		}

		newInstanceTypes[name] = types
	}

	// Update the global map
	instanceTypes = newInstanceTypes

	// And save in the cache
	err = instanceSaveCache()
	if err != nil {
		logger.Warnf("Failed to update instance types cache: %v", err)
		return err
	}

	logger.Infof("Done updating instance types")
	return nil
}

func instanceParseType(value string) (map[string]string, error) {
	sourceName := ""
	sourceType := ""
	fields := strings.SplitN(value, ":", 2)

	// Check if the name of the source was provided
	if len(fields) != 2 {
		sourceType = value
	} else {
		sourceName = fields[0]
		sourceType = fields[1]
	}

	// If not, lets go look for a match
	if instanceTypes != nil && sourceName == "" {
		for name, types := range instanceTypes {
			_, ok := types[sourceType]
			if ok {
				if sourceName != "" {
					return nil, fmt.Errorf("Ambiguous instance type provided: %s", value)
				}

				sourceName = name
			}
		}
	}

	// Check if we have a limit for the provided value
	limits, ok := instanceTypes[sourceName][sourceType]
	if !ok {
		// Check if it's maybe just a resource limit
		if sourceName == "" && value != "" {
			newLimits := instanceType{}
			fields := strings.Split(value, "-")
			for _, field := range fields {
				if len(field) < 2 || (field[0] != 'c' && field[0] != 'm') {
					return nil, fmt.Errorf("Bad instance type: %s", value)
				}

				value, err := strconv.ParseFloat(field[1:], 32)
				if err != nil {
					return nil, err
				}

				if field[0] == 'c' {
					newLimits.CPU = float32(value)
				} else if field[0] == 'm' {
					newLimits.Memory = float32(value)
				}
			}

			limits = &newLimits
		}

		if limits == nil {
			return nil, fmt.Errorf("Provided instance type doesn't exist: %s", value)
		}
	}
	out := map[string]string{}

	// Handle CPU
	if limits.CPU > 0 {
		cpuCores := int(limits.CPU)
		if float32(cpuCores) < limits.CPU {
			cpuCores++
		}
		cpuTime := int(limits.CPU / float32(cpuCores) * 100.0)

		out["limits.cpu"] = fmt.Sprintf("%d", cpuCores)
		if cpuTime < 100 {
			out["limits.cpu.allowance"] = fmt.Sprintf("%d%%", cpuTime)
		}
	}

	// Handle memory
	if limits.Memory > 0 {
		rawLimit := int64(limits.Memory * 1024)
		out["limits.memory"] = fmt.Sprintf("%dMB", rawLimit)
	}

	return out, nil
}
