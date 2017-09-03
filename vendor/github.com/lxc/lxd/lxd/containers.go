package main

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/lxc/lxd/lxd/db"
	"github.com/lxc/lxd/lxd/state"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/logger"

	log "gopkg.in/inconshreveable/log15.v2"
)

var containersCmd = Command{
	name: "containers",
	get:  containersGet,
	post: containersPost,
}

var containerCmd = Command{
	name:   "containers/{name}",
	get:    containerGet,
	put:    containerPut,
	delete: containerDelete,
	post:   containerPost,
	patch:  containerPatch,
}

var containerStateCmd = Command{
	name: "containers/{name}/state",
	get:  containerState,
	put:  containerStatePut,
}

var containerFileCmd = Command{
	name:   "containers/{name}/files",
	get:    containerFileHandler,
	post:   containerFileHandler,
	delete: containerFileHandler,
}

var containerSnapshotsCmd = Command{
	name: "containers/{name}/snapshots",
	get:  containerSnapshotsGet,
	post: containerSnapshotsPost,
}

var containerSnapshotCmd = Command{
	name:   "containers/{name}/snapshots/{snapshotName}",
	get:    snapshotHandler,
	post:   snapshotHandler,
	delete: snapshotHandler,
}

var containerExecCmd = Command{
	name: "containers/{name}/exec",
	post: containerExecPost,
}

var containerMetadataCmd = Command{
	name: "containers/{name}/metadata",
	get:  containerMetadataGet,
	put:  containerMetadataPut,
}

var containerMetadataTemplatesCmd = Command{
	name:   "containers/{name}/metadata/templates",
	get:    containerMetadataTemplatesGet,
	post:   containerMetadataTemplatesPostPut,
	put:    containerMetadataTemplatesPostPut,
	delete: containerMetadataTemplatesDelete,
}

type containerAutostartList []container

func (slice containerAutostartList) Len() int {
	return len(slice)
}

func (slice containerAutostartList) Less(i, j int) bool {
	iOrder := slice[i].ExpandedConfig()["boot.autostart.priority"]
	jOrder := slice[j].ExpandedConfig()["boot.autostart.priority"]

	if iOrder != jOrder {
		iOrderInt, _ := strconv.Atoi(iOrder)
		jOrderInt, _ := strconv.Atoi(jOrder)
		return iOrderInt > jOrderInt
	}

	return slice[i].Name() < slice[j].Name()
}

func (slice containerAutostartList) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func containersRestart(s *state.State) error {
	// Get all the containers
	result, err := db.ContainersList(s.DB, db.CTypeRegular)
	if err != nil {
		return err
	}

	containers := []container{}

	for _, name := range result {
		c, err := containerLoadByName(s, name)
		if err != nil {
			return err
		}

		containers = append(containers, c)
	}

	sort.Sort(containerAutostartList(containers))

	// Restart the containers
	for _, c := range containers {
		config := c.ExpandedConfig()
		lastState := config["volatile.last_state.power"]

		autoStart := config["boot.autostart"]
		autoStartDelay := config["boot.autostart.delay"]

		if shared.IsTrue(autoStart) || (autoStart == "" && lastState == "RUNNING") {
			if c.IsRunning() {
				continue
			}

			c.Start(false)

			autoStartDelayInt, err := strconv.Atoi(autoStartDelay)
			if err == nil {
				time.Sleep(time.Duration(autoStartDelayInt) * time.Second)
			}
		}
	}

	return nil
}

func containersShutdown(s *state.State) error {
	var wg sync.WaitGroup

	// Get all the containers
	results, err := db.ContainersList(s.DB, db.CTypeRegular)
	if err != nil {
		return err
	}

	// Reset all container states
	_, err = db.Exec(s.DB, "DELETE FROM containers_config WHERE key='volatile.last_state.power'")
	if err != nil {
		return err
	}

	for _, r := range results {
		// Load the container
		c, err := containerLoadByName(s, r)
		if err != nil {
			return err
		}

		// Record the current state
		lastState := c.State()

		// Stop the container
		if c.IsRunning() {
			// Determinate how long to wait for the container to shutdown cleanly
			var timeoutSeconds int
			value, ok := c.ExpandedConfig()["boot.host_shutdown_timeout"]
			if ok {
				timeoutSeconds, _ = strconv.Atoi(value)
			} else {
				timeoutSeconds = 30
			}

			// Stop the container
			wg.Add(1)
			go func() {
				c.Shutdown(time.Second * time.Duration(timeoutSeconds))
				c.Stop(false)
				c.ConfigKeySet("volatile.last_state.power", lastState)

				wg.Done()
			}()
		} else {
			c.ConfigKeySet("volatile.last_state.power", lastState)
		}
	}
	wg.Wait()

	return nil
}

func containerDeleteSnapshots(s *state.State, cname string) error {
	logger.Debug("containerDeleteSnapshots",
		log.Ctx{"container": cname})

	results, err := db.ContainerGetSnapshots(s.DB, cname)
	if err != nil {
		return err
	}

	for _, sname := range results {
		sc, err := containerLoadByName(s, sname)
		if err != nil {
			logger.Error(
				"containerDeleteSnapshots: Failed to load the snapshotcontainer",
				log.Ctx{"container": cname, "snapshot": sname})

			continue
		}

		if err := sc.Delete(); err != nil {
			logger.Error(
				"containerDeleteSnapshots: Failed to delete a snapshotcontainer",
				log.Ctx{"container": cname, "snapshot": sname, "err": err})
		}
	}

	return nil
}
