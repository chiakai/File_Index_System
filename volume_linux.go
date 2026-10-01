package main

import (
	"bufio"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
)

func readVolumeInfo(path string) volumeInfo {
	var info volumeInfo
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return info
	}
	if st.Bsize > 0 && st.Blocks <= uint64((1<<63-1)/st.Bsize) {
		capacity := int64(st.Blocks) * st.Bsize
		info.Capacity = &capacity
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return info
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return info
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return info
	}
	defer f.Close()
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	longest := -1
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 5 || len(right) < 1 {
			continue
		}
		mount := unescape.Replace(left[4])
		if (resolved == mount || strings.HasPrefix(resolved, strings.TrimRight(mount, "/")+"/")) && len(mount) > longest {
			info.Filesystem, longest = right[0], len(mount)
		}
	}
	return info
}
