package proxy

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
)

func TestIsReuseportUnsupported(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "EINVAL", err: syscall.EINVAL, want: true},
		{name: "wrapped EINVAL", err: fmt.Errorf("bind: %w", syscall.EINVAL), want: true},
		{name: "OpError EINVAL", err: &net.OpError{Op: "listen", Net: "udp4", Err: syscall.EINVAL}, want: true},
		{name: "string invalid argument", err: errors.New("listen udp4 :19132: invalid argument"), want: true},
		{name: "address in use", err: syscall.EADDRINUSE, want: false},
		{name: "OpError address in use", err: &net.OpError{Op: "listen", Net: "udp4", Err: syscall.EADDRINUSE}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isReuseportUnsupported(tc.err); got != tc.want {
				t.Fatalf("isReuseportUnsupported(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
