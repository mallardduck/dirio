package clients_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func findAvailablePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}
