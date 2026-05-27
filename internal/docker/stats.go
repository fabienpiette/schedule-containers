package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
)

type ContainerHealth struct {
	Status             string
	ContainerIP        string   // first Docker bridge IP (reachable from host and other containers)
	Ports              []uint16 // container-internal ports
	HostPorts          []uint16 // host-published ports
	HealthCheckDefined bool
}

type StatsSnapshot struct {
	CPUPercent    float64
	NetworkRxBytes float64
	NetworkTxBytes float64
	Timestamp     time.Time
}

func parseHealthStatus(status string) string {
	switch status {
	case "healthy", "unhealthy", "starting":
		return status
	default:
		return ""
	}
}

func (c *Client) InspectContainer(ctx context.Context, name string) (*ContainerHealth, error) {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	h := &ContainerHealth{}

	if inspect.State != nil && inspect.State.Health != nil {
		h.Status = parseHealthStatus(inspect.State.Health.Status)
		h.HealthCheckDefined = true
	}

	h.Ports = collectTCPPorts(inspect.Config, inspect.NetworkSettings)
	h.HostPorts = collectHostPorts(inspect.NetworkSettings)
	h.ContainerIP = extractContainerIP(inspect.NetworkSettings)

	return h, nil
}

func extractContainerIP(netSettings *container.NetworkSettings) string {
	if netSettings == nil {
		return ""
	}
	for _, ep := range netSettings.Networks {
		if ep != nil && ep.IPAddress != "" {
			return ep.IPAddress
		}
	}
	return ""
}

func collectHostPorts(netSettings *container.NetworkSettings) []uint16 {
	if netSettings == nil {
		return nil
	}
	seen := make(map[uint16]bool)
	for p, bindings := range netSettings.Ports {
		if p.Proto() != "tcp" {
			continue
		}
		for _, b := range bindings {
			if b.HostPort != "" {
				if v, err := parseUint16(b.HostPort); err == nil {
					seen[v] = true
				}
			}
		}
	}
	result := make([]uint16, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	return result
}

func collectTCPPorts(config *container.Config, netSettings *container.NetworkSettings) []uint16 {
	seen := make(map[uint16]bool)

	if config != nil {
		for p := range config.ExposedPorts {
			if p.Proto() == "tcp" {
				if v, err := parseUint16(p.Port()); err == nil {
					seen[v] = true
				}
			}
		}
	}

	if netSettings != nil {
		for p := range netSettings.Ports {
			if p.Proto() == "tcp" {
				if v, err := parseUint16(p.Port()); err == nil {
					seen[v] = true
				}
			}
		}
	}

	result := make([]uint16, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	return result
}

func parseUint16(s string) (uint16, error) {
	var v uint16
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func (c *Client) ContainerStats(ctx context.Context, name string) (<-chan StatsSnapshot, error) {
	stream, err := c.cli.ContainerStats(ctx, name, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats stream: %w", err)
	}

	ch := make(chan StatsSnapshot, 1)
	go func() {
		defer close(ch)
		defer stream.Body.Close()

		dec := json.NewDecoder(stream.Body)
		var prev *container.StatsResponse

		for {
			var stats container.StatsResponse
			if err := dec.Decode(&stats); err != nil {
				if ctx.Err() != nil {
					return
				}
				if err == io.EOF {
					return
				}
				return
			}

			if prev != nil {
				cpuPercent := computeCPUPercent(&stats, prev)
				rxBytes, txBytes := computeNetworkBytes(&stats, prev)
				ch <- StatsSnapshot{
					CPUPercent:    cpuPercent,
					NetworkRxBytes: rxBytes,
					NetworkTxBytes: txBytes,
					Timestamp:     stats.Read,
				}
			}
			prev = &stats
		}
	}()

	return ch, nil
}

func computeCPUPercent(current, previous *container.StatsResponse) float64 {
	cpuDelta := float64(current.CPUStats.CPUUsage.TotalUsage - previous.CPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(current.CPUStats.SystemUsage - previous.CPUStats.SystemUsage)

	if systemDelta == 0 {
		return 0
	}

	numCPU := uint64(len(current.CPUStats.CPUUsage.PercpuUsage))
	if numCPU == 0 {
		numCPU = uint64(current.CPUStats.OnlineCPUs)
	}
	if numCPU == 0 {
		numCPU = 1
	}

	return (cpuDelta / systemDelta) * float64(numCPU) * 100.0
}

func computeNetworkBytes(current, previous *container.StatsResponse) (rxBytes, txBytes float64) {
	for iface, cur := range current.Networks {
		if prev, ok := previous.Networks[iface]; ok {
			rxBytes += float64(cur.RxBytes - prev.RxBytes)
			txBytes += float64(cur.TxBytes - prev.TxBytes)
		}
	}
	return rxBytes, txBytes
}

