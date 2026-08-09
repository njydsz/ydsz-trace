//go:build !windows

package source

import (
	"context"
	"net"
)

func winNetDial(ctx context.Context, pipeName string) (net.Conn, error) {
	return net.Dial("unix", pipeName)
}
