//go:build linux

package sysinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Start usa /proc no Linux.
func (s *Sampler) Start(interval time.Duration) {
	go func() {
		prevIdle, prevTotal := cpuTimes()
		for {
			time.Sleep(interval)
			st := Stats{CPU: -1, GPU: -1, CPUTemp: -1, GPUTemp: -1, NetDown: -1, NetUp: -1}
			idle, total := cpuTimes()
			if dt := total - prevTotal; dt > 0 {
				st.CPU = 100 * (1 - float64(idle-prevIdle)/float64(dt))
			}
			prevIdle, prevTotal = idle, total
			st.RAMTotal, st.RAMUsed = memInfo()
			var fs syscall.Statfs_t
			if syscall.Statfs("/", &fs) == nil {
				st.DiskTotal = fs.Blocks * uint64(fs.Bsize)
				st.DiskUsed = st.DiskTotal - fs.Bfree*uint64(fs.Bsize)
			}
			if b, err := os.ReadFile("/proc/uptime"); err == nil {
				if f, err := strconv.ParseFloat(strings.Fields(string(b))[0], 64); err == nil {
					st.Uptime = time.Duration(f * float64(time.Second))
				}
			}
			s.store(st)
		}
	}()
}

func cpuTimes() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		fields := strings.Fields(sc.Text())
		for i, v := range fields[1:] {
			n, _ := strconv.ParseUint(v, 10, 64)
			total += n
			if i == 3 || i == 4 {
				idle += n
			}
		}
	}
	return
}

func memInfo() (total, used uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var avail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = n * 1024
		case "MemAvailable:":
			avail = n * 1024
		}
	}
	return total, total - avail
}
