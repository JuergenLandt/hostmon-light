package main

import (
	"encoding/json"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"strings"
	"testing"
)

func TestMemoryDefinition(t *testing.T) {
	v := memoryValue(&mem.VirtualMemoryStat{Total: 1000, Available: 400, UsedPercent: 99})
	if v.UsedPercent != 60 {
		t.Fatal(v)
	}
}
func TestMemoryClamp(t *testing.T) {
	v := memoryValue(&mem.VirtualMemoryStat{Total: 10, Available: 20})
	if v.Available != 10 || v.UsedPercent != 0 {
		t.Fatal(v)
	}
}
func TestNoSwap(t *testing.T) {
	v := swapValue(&mem.SwapMemoryStat{})
	if v.Total != 0 || v.Used != 0 || v.UsedPercent != 0 {
		t.Fatal(v)
	}
}
func TestFilesystemReserved(t *testing.T) {
	v := filesystemValue(&disk.UsageStat{Total: 100, Used: 80, Free: 10})
	if v.UsedPercent < 88.88 || v.UsedPercent > 88.89 {
		t.Fatal(v)
	}
}
func TestNoInodes(t *testing.T) {
	if filesystemValue(&disk.UsageStat{}).InodesUsedPercent != nil {
		t.Fatal("expected null")
	}
}
func TestMemoryJSON(t *testing.T) {
	b, e := json.Marshal(memoryValue(&mem.VirtualMemoryStat{Total: 100, Available: 20}))
	if e != nil || !strings.Contains(string(b), `"used_percent":80`) || strings.Contains(string(b), "usedPercent") {
		t.Fatal(string(b), e)
	}
}
