//go:build !linux

package indexing

import "errors"

// fork: disk power state is only implemented on Linux; elsewhere disks count as awake.

var errDiskStateUnsupported = errors.New("disk power state checks are only supported on Linux")

func resolveSourceDisks(string) ([]string, error) { return nil, errDiskStateUnsupported }
func disksFromConfig([]string) ([]string, error)  { return nil, errDiskStateUnsupported }
func diskIsRotational(string) bool                { return false }
func diskIOCounters(string) (string, error)       { return "", errDiskStateUnsupported }
func ataPowerMode(string) (bool, error)           { return false, errDiskStateUnsupported }
