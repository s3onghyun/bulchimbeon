//go:build linux

// Package linux is the Linux backend: /proc for processes, systemd-inhibit and logind for
// the sleep lock, /sys for battery and temperature.
//
// The parsers are plain functions over bytes or a directory root, so they are tested with
// recorded fixtures (testdata/) on any OS. Only the inhibitor and doctor run commands.
package linux

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// ProcScanner lists the processes of one user from /proc.
//
// Security and privacy (same rules as the macOS libproc scanner):
//   - only processes whose /proc/<pid> belongs to UID are read
//   - argv[0] only, the first 8 arguments for node/bun/deno, the first 64 for processes that
//     already look like an agent (to spot helper sessions); never the environment
//   - the cmdline buffer is zeroed after parsing; nothing is stored or printed
type ProcScanner struct {
	Root string // "/proc"
	UID  int
	// ClockTick is USER_HZ (`getconf CLK_TCK`): the unit of utime and stime.
	ClockTick int64
}

// NewProcScanner scans the real /proc for the current user.
func NewProcScanner() *ProcScanner {
	return &ProcScanner{Root: "/proc", UID: os.Getuid(), ClockTick: clockTick("/proc/self/auxv")}
}

// maxCmdline caps how much of /proc/<pid>/cmdline is read.
const maxCmdline = 64 << 10

// Scan implements platform.ProcessSource.
func (s *ProcScanner) Scan(kinds []engine.AgentKind) (platform.ProcessScan, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return platform.ProcessScan{}, err
	}
	scan := platform.ProcessScan{CPUTimes: map[int32]uint64{}}
	boot, bootOK := time.Time{}, false
	if data, err := os.ReadFile(filepath.Join(s.Root, "stat")); err == nil {
		boot, bootOK = ParseBootTime(data)
	}
	for _, e := range entries {
		pid, err := strconv.ParseInt(e.Name(), 10, 32)
		if err != nil || pid <= 0 {
			continue
		}
		snap, ticks, start, ok := s.snapshot(int32(pid), kinds)
		if !ok {
			continue
		}
		if bootOK && start > 0 {
			snap.StartedAt = boot.Add(time.Duration(start) * (time.Second / time.Duration(s.ClockTick)))
		}
		scan.Snapshots = append(scan.Snapshots, snap)
		scan.CPUTimes[snap.PID] = ticks * uint64(1_000_000_000/s.ClockTick)
	}
	sort.Slice(scan.Snapshots, func(i, j int) bool { return scan.Snapshots[i].PID < scan.Snapshots[j].PID })
	return scan, nil
}

// snapshot reads one process. Processes that vanish, belong to someone else or refuse
// readlink (non-dumpable ones such as gpg-agent or sshd) are skipped.
// start is the process start time in clock ticks after boot (0 when unreadable).
func (s *ProcScanner) snapshot(pid int32, kinds []engine.AgentKind) (snap engine.ProcessSnapshot, ticks, start uint64, ok bool) {
	dir := filepath.Join(s.Root, strconv.Itoa(int(pid)))
	info, err := os.Stat(dir)
	if err != nil {
		return engine.ProcessSnapshot{}, 0, 0, false
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != s.UID {
		return engine.ProcessSnapshot{}, 0, 0, false
	}
	exe, err := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil {
		return engine.ProcessSnapshot{}, 0, 0, false
	}
	exe = StripDeleted(exe)
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return engine.ProcessSnapshot{}, 0, 0, false
	}
	ppid, ticks, err := ParseStat(stat)
	start, _ = ParseStartTicks(stat)
	if err != nil {
		return engine.ProcessSnapshot{}, 0, 0, false
	}

	raw := readCmdline(filepath.Join(dir, "cmdline"))
	defer clear(raw)
	limit := engine.ArgsDefault
	if engine.NeedsFullArguments(exe) {
		limit = engine.ArgsRunner
	}
	args := ParseCmdline(raw, limit)
	var argv0 *string
	if len(args) > 0 {
		argv0 = &args[0]
	}
	snap = engine.ProcessSnapshot{PID: pid, PPID: ppid, ExecutablePath: exe}
	if engine.IsCandidate(exe, argv0, kinds) {
		args = ParseCmdline(raw, engine.ArgsCandidate)
	}
	snap.Arguments = args
	return snap, ticks, start, true
}

func readCmdline(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, maxCmdline)
	n, _ := io.ReadFull(f, buf)
	return buf[:n]
}

// StripDeleted removes the " (deleted)" suffix the kernel adds when the executable was
// replaced after start (apt upgrades, Claude Code's auto-updater).
func StripDeleted(path string) string { return strings.TrimSuffix(path, " (deleted)") }

// ParseStat reads the parent PID and utime+stime (clock ticks) from /proc/<pid>/stat.
// Fields are counted after the last ')', because the command name may contain spaces
// and parentheses.
func ParseStat(data []byte) (ppid int32, ticks uint64, err error) {
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return 0, 0, errors.New("stat: no command name")
	}
	f := strings.Fields(string(data[end+1:]))
	// f[0] state, f[1] ppid, ..., f[11] utime, f[12] stime (fields 3, 4, 14, 15 of proc(5))
	if len(f) < 13 {
		return 0, 0, fmt.Errorf("stat: %d fields", len(f))
	}
	p, err := strconv.ParseInt(f[1], 10, 32)
	if err != nil {
		return 0, 0, err
	}
	utime, err := strconv.ParseUint(f[11], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	stime, err := strconv.ParseUint(f[12], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return int32(p), utime + stime, nil
}

// ParseStartTicks reads the start time (clock ticks after boot, field 22 of proc(5)) from
// /proc/<pid>/stat.
func ParseStartTicks(data []byte) (uint64, bool) {
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return 0, false
	}
	f := strings.Fields(string(data[end+1:]))
	if len(f) < 20 {
		return 0, false
	}
	v, err := strconv.ParseUint(f[19], 10, 64)
	return v, err == nil
}

// ParseBootTime reads the boot time (`btime`, Unix seconds) from /proc/stat.
func ParseBootTime(data []byte) (time.Time, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "btime" {
			if v, err := strconv.ParseInt(f[1], 10, 64); err == nil && v > 0 {
				return time.Unix(v, 0), true
			}
		}
	}
	return time.Time{}, false
}

// ParseCmdline splits /proc/<pid>/cmdline (NUL-separated, NUL-terminated) and keeps at
// most limit arguments.
func ParseCmdline(data []byte, limit int) []string {
	var args []string
	for len(data) > 0 && len(args) < limit {
		i := bytes.IndexByte(data, 0)
		if i < 0 {
			args = append(args, string(data))
			break
		}
		args = append(args, string(data[:i]))
		data = data[i+1:]
	}
	return args
}

// clockTick reads AT_CLKTCK from the auxiliary vector, falling back to 100 (USER_HZ on
// every mainstream architecture).
func clockTick(auxv string) int64 {
	data, err := os.ReadFile(auxv)
	if err != nil {
		return 100
	}
	return ParseClockTick(data)
}

// ParseClockTick finds AT_CLKTCK (17) in a 64-bit little-endian auxiliary vector.
func ParseClockTick(data []byte) int64 {
	const atClkTck = 17
	for i := 0; i+16 <= len(data); i += 16 {
		key, val := binary.LittleEndian.Uint64(data[i:]), binary.LittleEndian.Uint64(data[i+8:])
		if key == 0 {
			break
		}
		if key == atClkTck && val > 0 && val <= 1_000_000 {
			return int64(val)
		}
	}
	return 100
}
