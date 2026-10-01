//go:build !windows && !linux && !darwin

package main

func readVolumeInfo(path string) volumeInfo { return volumeInfo{} }
