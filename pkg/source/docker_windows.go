//go:build windows

package source

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Windows Docker named pipe dialing.
// Uses winio library for named pipe communication,
// falls back to TCP if pipe name is tcp://...
func winNetDial(ctx context.Context, pipeName string) (net.Conn, error) {
	if len(pipeName) > 6 && pipeName[:6] == "tcp://" {
		// TCP mode (Docker Desktop default)
		var d net.Dialer
		d.Timeout = 10 * time.Second
		return d.DialContext(ctx, "tcp", pipeName[6:])
	}

	// Named pipe mode would use github.com/Microsoft/go-winio
	return nil, fmt.Errorf(
		"Windows named pipe dialing not implemented; use tcp://host:2375 as pipe name or install go-winio",
	)
}
