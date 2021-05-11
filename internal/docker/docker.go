package docker

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/vontikov/pgcluster/internal/logging"
)

const (
	loggerName = "docker"
)

type Client struct {
	cli    *client.Client
	logger logging.Logger
}

func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &Client{
		cli:    cli,
		logger: logging.NewLogger(loggerName),
	}, nil
}

// ContainerOptions contains the options to create the container with.
type ContainerOptions struct {
	Image    string
	Env      []string
	HostIP   string
	Ports    [][]int
	Volumes  []string
	Hostname string
	Command  string
	Name     string
	Network  string
}

// ImagePull pulls the Docker image.
func (c *Client) ImagePull(tag string) (err error) {
	c.logger.Debug("pulling image", "tag", tag)
	defer func() {
		if err != nil {
			c.logger.Error("pull error", "message", err)
		}
	}()

	out, err := c.cli.ImagePull(context.Background(), tag, types.ImagePullOptions{})
	if err != nil {
		return
	}
	_, err = io.Copy(os.Stdout, out)
	return
}

// ContainerCreate creates and starts the Docker container.
func (c *Client) ContainerCreate(options ContainerOptions) (id string, err error) {
	c.logger.Debug("creating container", "options", options)
	defer func() {
		if err != nil {
			c.logger.Error("create error", "message", err)
		} else {
			c.logger.Debug("container created", "id", id)
		}
	}()

	pwd, err := os.Getwd()
	if err != nil {
		return
	}

	exposedPorts := make(nat.PortSet)
	portBindings := make(map[nat.Port][]nat.PortBinding)
	for _, p := range options.Ports {
		published := strconv.Itoa(p[0])
		target := nat.Port(strconv.Itoa(p[1]))
		exposedPorts[target] = struct{}{}
		portBindings[target] = []nat.PortBinding{{HostIP: options.HostIP, HostPort: published}}
	}

	var cmd []string
	if len(strings.TrimSpace(options.Command)) > 0 {
		cmd = strings.Split(options.Command, " ")
	}

	config := &container.Config{
		Hostname:     options.Hostname,
		Cmd:          strslice.StrSlice(cmd),
		Env:          options.Env,
		ExposedPorts: exposedPorts,
		Image:        options.Image,
	}

	var mounts []mount.Mount
	for _, v := range options.Volumes {
		a := strings.Split(v, ":")
		source := a[0]
		target := a[1]
		if strings.HasPrefix(source, "./") {
			source = strings.Replace(source, ".", pwd, 1)
		}

		mounts = append(mounts,
			mount.Mount{
				Type:   mount.TypeBind,
				Source: source,
				Target: target,
			})
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Mounts:       mounts,
	}

	endpointsConfig := make(map[string]*network.EndpointSettings)
	endpointsConfig[options.Network] = &network.EndpointSettings{}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: endpointsConfig,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, options.Name)
	if err != nil {
		return
	}
	return resp.ID, nil
}

// ContainerStart starts the container with the id.
func (c *Client) ContainerStart(id string) (err error) {
	c.logger.Debug("starting container", "id", id)
	defer func() {
		if err != nil {
			c.logger.Error("start error", "message", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.cli.ContainerStart(ctx, id, types.ContainerStartOptions{})
}

// ContainerStop stops the Docker containter.
func (c *Client) ContainerStop(id string) (err error) {
	c.logger.Debug("stopping container", "id", id)
	defer func() {
		if err != nil {
			c.logger.Error("stop error", "message", err)
		}
	}()

	err = c.cli.ContainerStop(context.Background(), id, nil)
	return
}

// ContainerRemove removes the Docker containter.
func (c *Client) ContainerRemove(id string) (err error) {
	c.logger.Debug("removing container", "id", id)
	defer func() {
		if err != nil {
			c.logger.Error("remove error", "message", err)
		}
	}()

	opts := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}
	err = c.cli.ContainerRemove(context.Background(), id, opts)
	return
}

// ContainerKill kills the Docker container.
func (c *Client) ContainerKill(id string) (err error) {
	c.logger.Debug("killing container", "id", id)
	defer func() {
		if err != nil {
			c.logger.Error("kill error", "message", err)
		}
	}()
	err = c.cli.ContainerKill(context.Background(), id, "9")
	return
}

// NetworkCreate creates Docker network.
func (c *Client) NetworkCreate(name string) (id string, err error) {
	c.logger.Debug("creating network", "name", name)
	defer func() {
		if err != nil {
			c.logger.Error("create error", "message", err)
		} else {
			c.logger.Debug("network created", "id", id)
		}
	}()

	options := types.NetworkCreate{
		Driver:         "bridge",
		CheckDuplicate: true,
	}
	r, err := c.cli.NetworkCreate(context.Background(), name, options)
	if err != nil {
		return
	}
	return r.ID, nil
}

// NetworkRemove removes Docker network.
func (c *Client) NetworkRemove(id string) (err error) {
	c.logger.Debug("removing network", "id", id)
	defer func() {
		if err != nil {
			c.logger.Error("remove error", "message", err)
		}
	}()
	err = c.cli.NetworkRemove(context.Background(), id)
	return
}
