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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
)

func TestDeviceName(t *testing.T) {
	ok, name := DeviceName(v1.ResourceName("NotADeviceName"))
	require.False(t, ok)

	ok, name = DeviceName(v1.ResourceNvidiaGPU)
	require.True(t, ok)
	require.Equal(t, name, "nvidia-gpu")

	ok, name = DeviceName(v1.ResourceName(v1.ResourceOpaqueIntPrefix + "foo"))
	require.True(t, ok)
	require.Equal(t, name, "foo")
}
