//go:build !windows

package main

import "os"

func atomicReplaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func syncStoreDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
