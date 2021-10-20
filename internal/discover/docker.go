package discover

import (
	"fmt"

	"github.com/docker/docker/client"
)

func NewDockerPodman(host string) (*client.Client, error) {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	if host != "" {
		opts = append(opts, client.WithHost(host))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating docker client: %w", err)
	}

	return cli, nil
}
