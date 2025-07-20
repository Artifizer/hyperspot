package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

var sysCapSwDocker = syscap.SysCap{Category: syscap.CategorySoftware, Name: "docker", DisplayName: "Docker", CacheTTLMsec: 3000, Detector: detectDocker}

// registerDockerSysCaps registers Docker capability detectors
func registerDockerSysCaps() {
	syscap.RegisterSysCap(&sysCapSwDocker)
}

// detectDocker detects if Docker is installed and running
// Utilizes functionality from executor_docker.go for Docker client creation
func detectDocker(c *syscap.SysCap) error {
	c.Present = false

	// Try to create a Docker client using the same method as executor
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		// Docker client creation failed, Docker not available
		c.SetDetails("Docker client creation failed: " + err.Error())
		return err
	}
	defer cli.Close()

	// Try to ping Docker daemon with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ping, err := cli.Ping(ctx)
	if err != nil {
		// Docker daemon not running or not accessible
		c.SetDetails("Docker daemon not accessible: " + err.Error())
		return err
	}

	// Docker is available
	c.Present = true
	c.SetVersion(ping.APIVersion)

	// Get additional Docker info
	info, err := cli.Info(ctx)
	if err != nil {
		c.SetDetails("Docker version: " + ping.APIVersion)
	} else {
		details := fmt.Sprintf("Docker version: %s, Server version: %s, Operating system: %s, Architecture: %s",
			ping.APIVersion, info.ServerVersion, info.OperatingSystem, info.Architecture)

		if info.MemTotal > 0 {
			memGB := float64(info.MemTotal) / (1024 * 1024 * 1024)
			details += fmt.Sprintf(", Available memory: %.2f GB", memGB)
		}

		if info.NCPU > 0 {
			details += fmt.Sprintf(", CPUs: %d", info.NCPU)
		}

		// Get additional container and image information
		containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
		if err == nil {
			details += fmt.Sprintf(", Containers: %d", len(containers))
		}

		images, err := cli.ImageList(ctx, types.ImageListOptions{})
		if err == nil {
			details += fmt.Sprintf(", Images: %d", len(images))
		}

		c.SetDetails(details)
	}

	return nil
}
