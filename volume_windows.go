package main

import (
	"fmt"
	"golang.org/x/sys/windows"
)

func readVolumeInfo(path string) volumeInfo {
	var info volumeInfo
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return info
	}
	root := make([]uint16, 32768)
	if windows.GetVolumePathName(p, &root[0], uint32(len(root))) != nil {
		return info
	}
	var label, filesystem [256]uint16
	var serial uint32
	if windows.GetVolumeInformation(&root[0], &label[0], uint32(len(label)), &serial, nil, nil, &filesystem[0], uint32(len(filesystem))) == nil {
		info.Filesystem = windows.UTF16ToString(filesystem[:])
		info.Label = windows.UTF16ToString(label[:])
		info.ID = fmt.Sprintf("%08X", serial)
	}
	var total uint64
	if windows.GetDiskFreeSpaceEx(&root[0], nil, &total, nil) == nil && total <= 1<<63-1 {
		capacity := int64(total)
		info.Capacity = &capacity
	}
	return info
}
