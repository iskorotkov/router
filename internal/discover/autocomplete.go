package discover

import (
	"context"
	"fmt"
	"log"
	"os/user"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

const elementsPerSource = 200

var dockerPodmanSockets = []string{ //nolint:gochecknoglobals
	"unix:///var/run/docker.sock",
	"unix:///var/run/user/{userID}/docker.sock",
	"unix:///var/run/podman/podman.sock",
	"unix:///var/run/user/{userID}/podman/podman.sock",
}

type Autocomplete struct {
	dockerPodmanClients []*client.Client
}

func NewAutocomplete() (Autocomplete, error) {
	u, err := user.Current()
	if err != nil {
		return Autocomplete{}, fmt.Errorf("error getting current user: %w", err)
	}

	var dockerPodmanClients []*client.Client //nolint:prealloc

	for _, host := range dockerPodmanSockets {
		if u != nil {
			host = strings.ReplaceAll(host, "{userID}", u.Uid)
		}

		dockerPodman, err := NewDockerPodman(host)
		if err != nil {
			log.Printf("error creating docker/podman client with host %q: %v", host, err)

			continue
		}

		if _, err := dockerPodman.Info(context.Background()); err != nil {
			log.Printf("error getting docker/podman server info with host %q: %v", host, err)

			continue
		}

		dockerPodmanClients = append(dockerPodmanClients, dockerPodman)
	}

	return Autocomplete{
		dockerPodmanClients: dockerPodmanClients,
	}, nil
}

func (s Autocomplete) Hosts() []string {
	containersNames := []string{"localhost"}

	var wg sync.WaitGroup

	wg.Add(len(s.dockerPodmanClients))

	for _, dockerPodman := range s.dockerPodmanClients {
		dockerPodman := dockerPodman

		go func() {
			defer wg.Done()

			items, err := s.dockerPodmanContainers(dockerPodman)
			if err != nil {
				log.Printf("error autocompleting docker/podman containers: %v", err)

				return
			}

			containersNames = append(containersNames, items...)
		}()
	}

	wg.Wait()

	return containersNames
}

func (s Autocomplete) dockerPodmanContainers(c *client.Client) ([]string, error) {
	var containersNames []string

	containers, err := c.ContainerList(context.Background(), types.ContainerListOptions{
		Quiet:   true,
		Size:    false,
		All:     false,
		Latest:  false,
		Since:   "",
		Before:  "",
		Limit:   elementsPerSource,
		Filters: filters.Args{},
	})
	if err != nil {
		return nil, fmt.Errorf("error listing docker/podman containers: %w", err)
	}

	for _, container := range containers {
		for _, name := range container.Names {
			for _, port := range container.Ports {
				name := name[1:] // Trim '/' at the beginning.
				containersNames = append(containersNames, fmt.Sprintf("%s:%d", name, port.PublicPort))
			}
		}
	}

	services, err := c.ServiceList(context.Background(), types.ServiceListOptions{
		Filters: filters.Args{},
		Status:  false,
	})
	if err != nil && !errdefs.IsNotFound(err) {
		return nil, fmt.Errorf("error listing docker/podman services: %w", err)
	}

	for _, service := range services {
		for _, port := range service.Endpoint.Ports {
			containersNames = append(containersNames, fmt.Sprintf("%s:%d", service.Spec.Name, port.PublishedPort))
		}
	}

	return containersNames, nil
}
