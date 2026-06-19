package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/fabienpiette/schedule-containers/internal/models"
)

type Client struct {
	cli *client.Client
}

func NewClient(dockerHost string) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithHost(dockerHost))
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

func (c *Client) ListContainers(ctx context.Context) ([]models.Container, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return transformContainers(containers), nil
}

func (c *Client) StartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStart(ctx, name, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStop(ctx, name, container.StopOptions{})
}

func (c *Client) IsRunning(ctx context.Context, name string) (bool, error) {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, fmt.Errorf("failed to inspect container: %w", err)
	}
	return inspect.State.Running, nil
}

func transformContainers(containers []container.Summary) []models.Container {
	var result []models.Container
	for _, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = ctr.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}
		stackName := ""
		if project, ok := ctr.Labels["com.docker.compose.project"]; ok {
			stackName = project
		}
		result = append(result, models.Container{
			ID:        ctr.ID[:12],
			Name:      name,
			Image:     ctr.Image,
			State:     ctr.State,
			Status:    ctr.Status,
			StackName: stackName,
		})
	}
	return result
}

func findContainer(containers []models.Container, name string) (*models.Container, error) {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i], nil
		}
	}
	return nil, fmt.Errorf("container %s not found", name)
}

func (c *Client) GetContainer(ctx context.Context, name string) (*models.Container, error) {
	containers, err := c.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	return findContainer(containers, name)
}