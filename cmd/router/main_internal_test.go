package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:funlen
func Test_getAddressAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		remoteHosts []string
		parsedHosts []string
		err         error
	}{
		{
			name:        "localhost",
			remoteHosts: []string{"http://localhost", "http://[::1]", "http://127.0.0.1"},
			parsedHosts: []string{"localhost", "[::1]", "127.0.0.1"},
			err:         nil,
		},
		{
			name:        "localhost with port",
			remoteHosts: []string{"http://localhost:123", "http://[::1]:123", "http://127.0.0.1:123"},
			parsedHosts: []string{"localhost:123", "[::1]:123", "127.0.0.1:123", "localhost", "[::1]", "127.0.0.1"},
			err:         nil,
		},
		{
			name:        "other host",
			remoteHosts: []string{"http://host"},
			parsedHosts: []string{"host"},
			err:         nil,
		},
		{
			name:        "other host with port",
			remoteHosts: []string{"http://host:123"},
			parsedHosts: []string{"host:123", "host"},
			err:         nil,
		},
		{
			name:        "other ip address",
			remoteHosts: []string{"http://18.12.41.128"},
			parsedHosts: []string{"18.12.41.128"},
			err:         nil,
		},
		{
			name:        "other ip address with host",
			remoteHosts: []string{"http://18.12.41.128:123"},
			parsedHosts: []string{"18.12.41.128:123", "18.12.41.128"},
			err:         nil,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, host := range tt.remoteHosts {
				host := host

				t.Run(host, func(t *testing.T) {
					t.Parallel()

					got, err := getAddressAliases(host)

					assert.ElementsMatch(t, tt.parsedHosts, got)
					assert.ElementsMatch(t, tt.err, err)
				})
			}
		})
	}
}
