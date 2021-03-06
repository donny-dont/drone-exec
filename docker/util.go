package docker

import (
	"errors"
	"os"
	// "strings"

	log "github.com/Sirupsen/logrus"
	"github.com/samalba/dockerclient"
)

var (
	ErrTimeout = errors.New("Timeout")
	ErrLogging = errors.New("Logs not available")
)

var (
	// options to fetch the stdout and stderr logs
	logOpts = &dockerclient.LogOptions{
		Stdout: true,
		Stderr: true,
	}

	// options to fetch the stdout and stderr logs
	// by tailing the output.
	logOptsTail = &dockerclient.LogOptions{
		Follow: true,
		Stdout: true,
		Stderr: true,
	}
)

func Run(client dockerclient.Client, conf *dockerclient.ContainerConfig, pull bool) (*dockerclient.ContainerInfo, error) {

	// fetches the container information.
	info, err := Start(client, conf, pull)
	if err != nil {
		return nil, err
	}

	// ensures the container is always stopped
	// and ready to be removed.
	defer func() {
		client.StopContainer(info.Id, 5)
		client.KillContainer(info.Id, "9")
	}()

	// channel listening for errors while the
	// container is running async.
	errc := make(chan error, 1)
	infoc := make(chan *dockerclient.ContainerInfo, 1)
	go func() {

		// blocks and waits for the container to finish
		// by streaming the logs (to /dev/null). Ideally
		// we could use the `wait` function instead
		rc, err := client.ContainerLogs(info.Id, logOptsTail)
		if err != nil {
			log.Errorf("Error tailing %s. %s\n", conf.Image, err)
			errc <- err
			return
		}
		defer rc.Close()
		StdCopy(os.Stdout, os.Stdout, rc)

		// fetches the container information
		info, err := client.InspectContainer(info.Id)
		if err != nil {
			log.Errorf("Error getting exit code for %s. %s\n", conf.Image, err)
			errc <- err
			return
		}
		infoc <- info
	}()

	select {
	case info := <-infoc:
		return info, nil
	case err := <-errc:
		return info, err
	}
}

func Start(client dockerclient.Client, conf *dockerclient.ContainerConfig, pull bool) (*dockerclient.ContainerInfo, error) {
	// force-pull the image if specified.
	if pull {
		log.Debugf("Pulling image %s", conf.Image)
		client.PullImage(conf.Image, nil)
	}

	// attempts to create the contianer
	id, err := client.CreateContainer(conf, "")
	if err != nil {
		// and pull the image and re-create if that fails
		err = client.PullImage(conf.Image, nil)
		if err != nil {
			log.Errorf("Error pulling %s. %s\n", conf.Image, err)
			return nil, err
		}
		id, err = client.CreateContainer(conf, "")
		if err != nil {
			log.Errorf("Error creating %s. %s\n", conf.Image, err)
			client.RemoveContainer(id, true, true)
			return nil, err
		}
	}

	// fetches the container information
	info, err := client.InspectContainer(id)
	if err != nil {
		log.Errorf("Error inspecting %s. %s\n", conf.Image, err)
		client.RemoveContainer(id, true, true)
		return nil, err
	}

	// starts the container
	err = client.StartContainer(id, &conf.HostConfig)
	if err != nil {
		log.Errorf("Error starting %s. %s\n", conf.Image, err)
	}
	return info, err
}
