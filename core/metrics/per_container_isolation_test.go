// Copyright 2026 The HuaTuo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"os"
	"path/filepath"
	"testing"

	"huatuo-bamai/internal/cgroups"
	"huatuo-bamai/internal/pod"
	"huatuo-bamai/internal/procfs"
	"huatuo-bamai/pkg/metric"
)

// withFixtureProcfs points /proc at a temp tree and returns its root.
func withFixtureProcfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	originalPrefix := filepath.Dir(procfs.DefaultPath())
	procfs.RootPrefix(root)
	t.Cleanup(func() { procfs.RootPrefix(originalPrefix) })
	return root
}

// withFakeContainers overrides the container listing seam.
func withFakeContainers(t *testing.T, containers map[string]*pod.Container) {
	t.Helper()
	original := normalContainers
	normalContainers = func() (map[string]*pod.Container, error) {
		return containers, nil
	}
	t.Cleanup(func() { normalContainers = original })
}

// findSeries returns the first data point with the given name and value.
func findSeries(data []*metric.Data, name string, value float64) bool {
	for _, d := range data {
		if d.Name() == name && d.Value == value {
			return true
		}
	}
	return false
}

// A container with a broken /proc/<pid> file must be skipped, not abort the
// whole scrape: the healthy container's metrics must still be exported.
func TestSockstatCollectorSkipsBrokenContainer(t *testing.T) {
	root := withFixtureProcfs(t)

	for pid, content := range map[int]string{
		1111: "sockets: used 7\nTCP: inuse 1 orphan 0 tw 0 alloc 1 mem 1\n",
		2222: "garbage\n",
	} {
		dir := filepath.Join(root, "proc", itoa(pid), "net")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll() error=%v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sockstat"), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile() error=%v", err)
		}
	}

	withFakeContainers(t, map[string]*pod.Container{
		"a": {Name: "healthy", InitPid: 1111, Labels: map[string]any{"HostNamespace": "default"}},
		"b": {Name: "broken", InitPid: 2222, Labels: map[string]any{"HostNamespace": "default"}},
	})

	collector := &sockstatCollector{}
	data, err := collector.Update()
	if err != nil {
		t.Fatalf("Update() error=%v, want broken container skipped", err)
	}
	if !findSeries(data, "container_sockets_used", 7) {
		t.Errorf("healthy container's sockets_used=7 missing from %d series", len(data))
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

// netdev_stats must keep the healthy container's series when another
// container's /proc/<pid>/net/dev is unreadable.
func TestNetdevCollectorSkipsBrokenContainer(t *testing.T) {
	originalConfig := configSnapshot()
	t.Cleanup(func() { Set(originalConfig) })
	testConfig := &Config{}
	testConfig.NetdevStats.DeviceIncluded = "eth0"
	Set(testConfig)

	root := withFixtureProcfs(t)

	for pid, content := range map[int]string{
		1111: "Inter-|   Receive                            |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n  eth0: 1000    1    0    0    0     0          0         0     2000       2    0    0    0     0       0          0\n",
		2222: "garbage\n",
	} {
		dir := filepath.Join(root, "proc", itoa(pid), "net")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll() error=%v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dev"), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile() error=%v", err)
		}
	}

	withFakeContainers(t, map[string]*pod.Container{
		"a": {Name: "healthy", InitPid: 1111, Labels: map[string]any{"HostNamespace": "default"}},
		"b": {Name: "broken", InitPid: 2222, Labels: map[string]any{"HostNamespace": "default"}},
	})

	collector := &netdevCollector{}
	data, err := collector.Update()
	if err != nil {
		t.Fatalf("Update() error=%v, want broken container skipped", err)
	}
	if !findSeries(data, "container_receive_bytes_total", 1000) {
		t.Errorf("healthy container's receive_bytes_total=1000 missing from %d series", len(data))
	}
}

type fakeMemEventsCgroup struct {
	cgroups.Cgroup
	byPath map[string]map[string]uint64
}

func (c *fakeMemEventsCgroup) MemoryEventRaw(path string) (map[string]uint64, error) {
	raw, ok := c.byPath[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return raw, nil
}

// memory_events must keep the healthy container's events when another
// container's cgroup has disappeared.
func TestMemoryEventsSkipsBrokenContainer(t *testing.T) {
	originalConfig := configSnapshot()
	t.Cleanup(func() { Set(originalConfig) })
	Set(&Config{})

	withFakeContainers(t, map[string]*pod.Container{
		"a": {Name: "healthy", CgroupPath: "cgrp-a", Labels: map[string]any{"HostNamespace": "default"}},
		"b": {Name: "broken", CgroupPath: "cgrp-b", Labels: map[string]any{"HostNamespace": "default"}},
	})

	fake := &fakeMemEventsCgroup{byPath: map[string]map[string]uint64{
		"cgrp-a": {"oom_kill": 2},
	}}
	collector := &memEventsCollector{cgroup: fake}

	data, err := collector.Update()
	if err != nil {
		t.Fatalf("Update() error=%v, want broken container skipped", err)
	}
	if !findSeries(data, "container_oom_kill", 2) {
		t.Errorf("healthy container's oom_kill=2 missing from %d series", len(data))
	}
}
