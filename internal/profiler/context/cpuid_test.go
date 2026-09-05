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

package context

import (
	"runtime"
	"strconv"
	"testing"
)

func TestParseCPUIDListHonorsOnlineCPUList(t *testing.T) {
	orig := onlineMaxCPUID
	defer func() { onlineMaxCPUID = orig }()
	maxID := runtime.NumCPU()*2 + 13
	onlineMaxCPUID = func() int { return maxID }

	got, err := parseCPUIDList(strconv.Itoa(maxID))
	if err != nil {
		t.Fatalf("parseCPUIDList(%q) error = %v", strconv.Itoa(maxID), err)
	}
	if len(got) != 1 || got[0] != maxID {
		t.Fatalf("parseCPUIDList(%q) = %v, want [%d]", strconv.Itoa(maxID), got, maxID)
	}
}

func TestParseCPUIDListRejectsBeyondOnlineBound(t *testing.T) {
	orig := onlineMaxCPUID
	defer func() { onlineMaxCPUID = orig }()
	onlineMaxCPUID = func() int { return 9 }

	if _, err := parseCPUIDList("10"); err == nil {
		t.Fatal("parseCPUIDList(10) error = nil, want out-of-range error")
	}
}

func TestParseCPUIDListFallbackToNumCPUWhenSysfsUnavailable(t *testing.T) {
	orig := onlineMaxCPUID
	defer func() { onlineMaxCPUID = orig }()
	onlineMaxCPUID = func() int { return -1 }

	highest := strconv.Itoa(runtime.NumCPU() - 1)
	got, err := parseCPUIDList(highest)
	if err != nil {
		t.Fatalf("parseCPUIDList(%q) error = %v", highest, err)
	}
	if len(got) != 1 || got[0] != runtime.NumCPU()-1 {
		t.Fatalf("parseCPUIDList(%q) = %v, want [%d]", highest, got, runtime.NumCPU()-1)
	}

	if _, err := parseCPUIDList(strconv.Itoa(runtime.NumCPU())); err == nil {
		t.Fatal("parseCPUIDList(NumCPU) error = nil, want out-of-range error")
	}
}
