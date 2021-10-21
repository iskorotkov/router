package discover

import (
	"context"
	"fmt"
	"log"
	"os/user"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	kubernetesClient    *kubernetes.Clientset
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
			log.Printf("skipping docker/podman client with host %q: %v", host, err)

			continue
		}

		if _, err := dockerPodman.Info(context.Background()); err != nil {
			log.Printf("skipping docker/podman server info with host %q: %v", host, err)

			continue
		}

		dockerPodmanClients = append(dockerPodmanClients, dockerPodman)
	}

	kubernetesClient, err := NewKubernetes()
	if err != nil {
		log.Printf("skipping k8s client: %v", err)
	}

	return Autocomplete{
		dockerPodmanClients: dockerPodmanClients,
		kubernetesClient:    kubernetesClient,
	}, nil
}

func (s Autocomplete) Hosts() []string {
	containersNames := []string{"localhost"}
	containersNamesQueue := make(chan string)

	go func() {
		for name := range containersNamesQueue {
			containersNames = append(containersNames, name)
		}

		log.Printf("all container names have been set")
	}()

	var wg sync.WaitGroup

	wg.Add(len(s.dockerPodmanClients) + 1)

	go func() {
		defer wg.Done()

		s.kubernetesNames(containersNamesQueue)
	}()

	for _, dockerPodman := range s.dockerPodmanClients {
		dockerPodman := dockerPodman

		go func() {
			defer wg.Done()

			s.dockerPodmanNames(dockerPodman, containersNamesQueue)
		}()
	}

	wg.Wait()

	close(containersNamesQueue)

	return containersNames
}

func (s Autocomplete) kubernetesNames(containersNamesQueue chan string) {
	if s.kubernetesClient == nil {
		return
	}

	namespaces, err := s.kubernetesClient.
		CoreV1().
		Namespaces().
		List(context.Background(), v1.ListOptions{Limit: elementsPerSource}) //nolint:exhaustivestruct
	if err != nil {
		log.Printf("skipping k8s: error getting list of namespaces: %v", err)
	}

	var wg sync.WaitGroup

	wg.Add(len(namespaces.Items))

	for _, ns := range namespaces.Items {
		ns := ns

		go func() {
			defer wg.Done()

			services, err := s.kubernetesClient.
				CoreV1().
				Services(ns.Name).
				List(context.Background(), v1.ListOptions{Limit: elementsPerSource}) //nolint:exhaustivestruct
			if err != nil {
				log.Printf("skipping namespace %q: error getting list of services: %v", ns.Name, err)

				return
			}

			for _, service := range services.Items {
				for _, port := range service.Spec.Ports {
					containersNamesQueue <- fmt.Sprintf("%s.%s.svc:%d", service.Name, ns.Name, port.Port)
				}
			}
		}()
	}

	wg.Wait()
}

func (s Autocomplete) dockerPodmanNames(c *client.Client, containersNamesQueue chan string) {
	containers, err := c.ContainerList(context.Background(),
		types.ContainerListOptions{Limit: elementsPerSource}) //nolint:exhaustivestruct
	if err != nil {
		log.Printf("skipping docker/podman containers: %v", err)
	} else {
		for _, container := range containers {
			for _, name := range container.Names {
				for _, port := range container.Ports {
					name := name[1:] // Trim '/' at the beginning.
					containersNamesQueue <- fmt.Sprintf("%s:%d", name, port.PublicPort)
				}
			}
		}
	}

	services, err := c.ServiceList(context.Background(), types.ServiceListOptions{}) //nolint:exhaustivestruct
	if err != nil && !errdefs.IsNotFound(err) {
		log.Printf("skipping docker/podman services: %v", err)
	} else {
		for _, service := range services {
			for _, port := range service.Endpoint.Ports {
				containersNamesQueue <- fmt.Sprintf("%s:%d", service.Spec.Name, port.PublishedPort)
			}
		}
	}
}
