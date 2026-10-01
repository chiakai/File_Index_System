package main

import "golang.org/x/sys/unix"

func readVolumeInfo(path string) volumeInfo {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return volumeInfo{}
	}
	info := volumeInfo{Filesystem: unix.ByteSliceToString(st.Fstypename[:])}
	if st.Bsize > 0 && st.Blocks <= uint64(1<<63-1)/uint64(st.Bsize) {
		capacity := int64(st.Blocks) * int64(st.Bsize)
		info.Capacity = &capacity
	}
	return info
}
