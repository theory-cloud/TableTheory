package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLocalListenAddrIsLoopback pins finding 3b3b4aea's local-exposure half: the
// local example server must only listen on loopback, never on every interface.
func TestLocalListenAddrIsLoopback(t *testing.T) {
	host, port, err := net.SplitHostPort(localListenAddr)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", host, "the local example server must bind to loopback")
	require.NotEmpty(t, port)
}
