package containerid

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sync"
)

var (
	// reGeneric matches container IDs in common mount paths
	// Matches: /containers/<containerID>/hostname, /sandboxes/<containerID>/resolv.conf, etc.
	reGeneric = regexp.MustCompile(`/([0-9a-f]{64})/(?:hostname|hosts|resolv\.conf)`)

	// Cache the container ID after first retrieval
	cachedID   string
	cacheOnce  sync.Once
	cacheError error
)

const (
	// MountInfoPath is the default path to the mountinfo file
	MountInfoPath = "/proc/self/mountinfo"

	// ShortIDLength is the standard length for short container IDs
	ShortIDLength = 12
)

// Get retrieves the full container ID from /proc/self/mountinfo.
// The result is cached after the first successful call.
func Get() (string, error) {
	cacheOnce.Do(func() {
		cachedID, cacheError = get()
	})
	return cachedID, cacheError
}

// GetShort returns the short version (12 characters) of the container ID.
// The result is cached after the first successful call.
func GetShort() (string, error) {
	fullID, err := Get()
	if err != nil {
		return "", err
	}
	if len(fullID) >= ShortIDLength {
		return fullID[:ShortIDLength], nil
	}
	return fullID, nil
}

// GetFromFile retrieves the container ID from a specific mountinfo file path.
// This is useful for testing or reading from non-standard locations.
func GetFromFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open mountinfo: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if matches := reGeneric.FindStringSubmatch(line); len(matches) > 1 {
			return matches[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading mountinfo: %w", err)
	}

	return "", fmt.Errorf("container ID not found in mountinfo")
}

// get is the internal implementation that reads from the default path
func get() (string, error) {
	return GetFromFile(MountInfoPath)
}

// IsInContainer checks if the current process is running inside a container.
// It returns true if a container ID can be detected.
func IsInContainer() bool {
	id, err := Get()
	return err == nil && id != ""
}
