// Package podid provides utilities to extract Kubernetes Pod ID (UUID)
// from the current pod environment by parsing /proc/self/mountinfo.
//
// This package works with all standard Kubernetes distributions including
// kubeadm, MicroK8s, k3s, minikube, and others.
package podid

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"
)

const (
	// MountInfoPath is the default path to the Linux mountinfo file.
	MountInfoPath = "/proc/self/mountinfo"
)

var (
	// ErrPodIDNotFound is returned when a Pod ID (UUID)
	// could not be found in /proc/self/mountinfo.
	ErrPodIDNotFound = errors.New("pod ID (UUID) not found in /proc/self/mountinfo")

	// This regex matches a standard UUID in kubelet pod paths.
	// Format: 8-4-4-4-12 hexadecimal characters (lowercase).
	// Example: /pods/036da4f7-d553-4eb6-9802-90f81041a412/
	podIDRegex = regexp.MustCompile(`/pods/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/`)

	// Cache to store the Pod ID after first successful retrieval
	cachedID string
	hasID    bool
	mu       sync.RWMutex

	getPodIDFunc = getPodIDFromMountInfo
)

// Get retrieves the Kubernetes Pod ID (UUID) from /proc/self/mountinfo.
// The result is cached after the first successful call for performance.
//
// Returns ErrPodIDNotFound if not running in a Kubernetes pod.
func Get() (string, error) {
	mu.RLock()
	if hasID {
		id := cachedID
		mu.RUnlock()
		return id, nil
	}
	mu.RUnlock()

	id, err := getPodIDFunc()
	if err != nil {
		return "", err
	}

	mu.Lock()
	cachedID = id
	hasID = true
	mu.Unlock()

	return id, nil
}

// GetFromFile retrieves the Pod ID from a specific mountinfo file path.
// This is useful for testing or reading from non-standard locations.
func GetFromFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		// The regex will find a path like:
		// ".../kubelet/pods/036da4f7-d553-4eb6-9802-90f81041a412/etc-hosts..."
		//
		// match[0] will be the full string: "/pods/036da4f7.../"
		// match[1] will be just the UUID: "036da4f7..."
		match := podIDRegex.FindStringSubmatch(line)

		if len(match) == 2 {
			// We found it. match[1] is the sub-match
			// (the part in the parentheses).
			return match[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading %s: %w", path, err)
	}

	// We scanned the whole file and found nothing.
	return "", ErrPodIDNotFound
}

// getPodIDFromMountInfo retrieves the Pod ID by parsing
// /proc/self/mountinfo and looking for a kubelet mount
// path containing a UUID.
func getPodIDFromMountInfo() (string, error) {
	return GetFromFile(MountInfoPath)
}

// IsInPod checks if the current process is running inside a Kubernetes pod.
// It returns true if a pod ID can be detected.
func IsInPod() bool {
	id, err := Get()
	return err == nil && id != ""
}

// MustGet retrieves the Pod ID and panics if an error occurs.
// This is useful for initialization where the pod ID must be available.
//
// Example:
//
//	var podID = podid.MustGet()
func MustGet() string {
	id, err := Get()
	if err != nil {
		panic(fmt.Sprintf("podid: failed to get pod ID: %v", err))
	}
	return id
}
