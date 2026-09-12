//go:build darwin

package sysinfo

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Start no macOS: memória e horário de boot vêm de sysctl (sem CGO); a
// porcentagem de CPU não tem um sysctl direto, então usa a saída do "top"
// (ferramenta padrão do macOS) — é o mesmo truque usado por scripts de shell
// de monitoramento, só que sem precisar de CGO/IOKit.
func (s *Sampler) Start(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			st := Stats{CPU: -1, GPU: -1, CPUTemp: -1, GPUTemp: -1, NetDown: -1, NetUp: -1}
			st.CPU = cpuPercentDarwin()
			if mem, err := unix.SysctlUint64("hw.memsize"); err == nil {
				st.RAMTotal = mem
				st.RAMUsed = mem - freeMemDarwin()
			}
			var fs syscall.Statfs_t
			if syscall.Statfs("/", &fs) == nil {
				st.DiskTotal = fs.Blocks * uint64(fs.Bsize)
				st.DiskUsed = st.DiskTotal - fs.Bfree*uint64(fs.Bsize)
			}
			if bt, err := unix.SysctlTimeval("kern.boottime"); err == nil {
				boot := time.Unix(bt.Sec, int64(bt.Usec)*1000)
				st.Uptime = time.Since(boot)
			}
			s.store(st)
		}
	}()
}

var topCPURe = regexp.MustCompile(`CPU usage:\s*[\d.]+%\s*user,\s*[\d.]+%\s*sys,\s*([\d.]+)%\s*idle`)

// cpuPercentDarwin lê "CPU usage: X% user, Y% sys, Z% idle" do top.
func cpuPercentDarwin() float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "top", "-l", "1", "-n", "0").Output()
	if err != nil {
		return -1
	}
	m := topCPURe.FindStringSubmatch(string(out))
	if m == nil {
		return -1
	}
	idle, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return -1
	}
	return clamp100(100 - idle)
}

var (
	vmPageSizeRe = regexp.MustCompile(`page size of (\d+) bytes`)
	vmFreeRe     = regexp.MustCompile(`Pages free:\s*(\d+)`)
	vmInactiveRe = regexp.MustCompile(`Pages inactive:\s*(\d+)`)
)

// freeMemDarwin soma páginas livres + infativas (recuperáveis sem swap), via vm_stat.
func freeMemDarwin() uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vm_stat").Output()
	if err != nil {
		return 0
	}
	text := string(out)
	pageSize := uint64(4096)
	if m := vmPageSizeRe.FindStringSubmatch(text); m != nil {
		if n, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			pageSize = n
		}
	}
	pages := func(re *regexp.Regexp) uint64 {
		m := re.FindStringSubmatch(text)
		if m == nil {
			return 0
		}
		n, _ := strconv.ParseUint(strings.TrimSuffix(m[1], "."), 10, 64)
		return n
	}
	return (pages(vmFreeRe) + pages(vmInactiveRe)) * pageSize
}

func clamp100(v float64) float64 {
	if v < 0 {
		return v
	}
	if v > 100 {
		return 100
	}
	return v
}
