/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deviceplugin

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

func TestInvalidRegistry(t *testing.T) {
	_, err := newRegistry("")
	require.Error(t, err)

	_, err = newRegistry("./test.sock")
	require.Error(t, err)
}

func TestInvalidRegister(t *testing.T) {
	mgr, err := NewManager(kubeletSocket, nil, nil, monitorCallback)
	require.NoError(t, err)

	m, err := StartMockDevicePluginServer(kubeletSocket, "fooVendor", deviceKind,
		nDevices, time.Millisecond*500)
	require.NoError(t, err)

	c, err := grpc.Dial(m.kubeletSocket, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)
	require.NoError(t, err)

	// Version match
	client := pluginapi.NewPluginRegistrationClient(c)
	resp, err := client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    "Not an exact version match",
		Unixsocket: deviceSock,
		Vendor:     m.vendor,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Error)

	// Invalid vendor
	resp, err = client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    pluginapi.Version,
		Unixsocket: deviceSock,
		Vendor:     "An invalid vendor/-...",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Error)

	// Have two device plugins on the same socket
	resp, err = client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    pluginapi.Version,
		Unixsocket: deviceSock,
		Vendor:     m.vendor,
	})
	require.NoError(t, err)
	require.Empty(t, resp.Error)

	resp, err = client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    pluginapi.Version,
		Unixsocket: deviceSock,
		Vendor:     "ADifferentVendor",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Error)

	c.Close()
	m.Stop()
	mgr.Stop()
}

func TestInvalidHeartbeat(t *testing.T) {
	r, err := newRegistry(kubeletSocket)
	require.NoError(t, err)

	resp, err := r.Heartbeat(context.Background(), &pluginapi.HeartbeatRequest{
		Vendor: "An invalid vendor/-...",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Error)

	resp, err = r.Heartbeat(context.Background(), &pluginapi.HeartbeatRequest{
		Vendor: "Vendor Does not exist",
	})
	require.NoError(t, err)
	require.Equal(t, resp.Response, pluginapi.HeartbeatKo)
}
