package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

const maxLogLine = 64 * 1024

func (c *Client) RestartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerRestart(ctx, name, container.StopOptions{})
}

// FollowLogs streams new log lines (stdout+stderr) for a running container,
// starting at `since`. The returned channel closes on stream end or ctx cancel.
// Returns an error if the container is missing or not running so the caller
// can back off and retry.
func (c *Client) FollowLogs(ctx context.Context, name string, since time.Time) (<-chan string, error) {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}
	if !inspect.State.Running {
		return nil, fmt.Errorf("container %s is not running", name)
	}

	rc, err := c.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Since:      strconv.FormatInt(since.Unix(), 10),
		Timestamps: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open container logs: %w", err)
	}

	out := make(chan string, 16)
	go func() {
		defer close(out)
		defer rc.Close()

		if inspect.Config != nil && inspect.Config.Tty {
			// TTY streams are raw (no stdcopy multiplex header).
			scanLogLines(ctx, rc, out)
			return
		}
		// Non-TTY streams are multiplexed; demux stdout+stderr into one pipe.
		pr, pw := io.Pipe()
		defer pr.Close()
		go func() {
			_, cerr := stdcopy.StdCopy(pw, pw, rc)
			pw.CloseWithError(cerr)
		}()
		scanLogLines(ctx, pr, out)
	}()

	return out, nil
}

// scanLogLines reads lines from r and emits them on out, capping each line at
// maxLogLine bytes (longer lines are truncated, and the remainder discarded up
// to the next newline). Returns when r is exhausted or ctx is cancelled.
func scanLogLines(ctx context.Context, r io.Reader, out chan<- string) {
	br := bufio.NewReaderSize(r, 8192)
	for {
		line, err := readCappedLine(br)
		if line != "" || err == nil {
			select {
			case out <- line:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// readCappedLine reads up to the next '\n', returning at most maxLogLine bytes
// (extra bytes on an over-long line are consumed and dropped). The trailing
// '\n' (and '\r') is stripped. On EOF with no data it returns ("", io.EOF).
func readCappedLine(br *bufio.Reader) (string, error) {
	buf := make([]byte, 0, 256)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return trimCR(string(buf)), err
		}
		if b == '\n' {
			return trimCR(string(buf)), nil
		}
		if len(buf) < maxLogLine {
			buf = append(buf, b)
		}
		// else: drop the byte but keep reading until newline/EOF
	}
}

func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}
