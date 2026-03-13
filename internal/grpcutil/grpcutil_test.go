package grpcutil

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerOptions_ReturnsOptions(t *testing.T) {
	logger := slog.Default()
	opts := ServerOptions(logger)
	require.NotEmpty(t, opts)
	assert.Len(t, opts, 4, "expected 4 server options (keepalive params, enforcement, max streams, interceptor)")
}
