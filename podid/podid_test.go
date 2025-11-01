package podid

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func resetTestState() func() {
	origFunc := getPodIDFunc
	origCachedID := cachedID
	origHasID := hasID

	cachedID = ""
	hasID = false
	mu = sync.RWMutex{}
	getPodIDFunc = getPodIDFromMountInfo

	return func() {
		cachedID = origCachedID
		hasID = origHasID
		mu = sync.RWMutex{}
		getPodIDFunc = origFunc
	}
}

func writeTempMountInfo(t *testing.T, content string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "mountinfo-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("WriteString: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	return tmpFile.Name()
}

func TestGetFromFileSuccess(t *testing.T) {
	want := "036da4f7-d553-4eb6-9802-90f81041a412"
	line := fmt.Sprintf("29 37 0:25 / /var/lib/kubelet/pods/%s/etc-hosts rw,relatime - tmpfs tmpfs rw\n", want)
	path := writeTempMountInfo(t, line)

	got, err := GetFromFile(path)
	if err != nil {
		t.Fatalf("GetFromFile returned error: %v", err)
	}
	if got != want {
		t.Fatalf("GetFromFile = %q, want %q", got, want)
	}
}

func TestGetFromFileNoMatch(t *testing.T) {
	path := writeTempMountInfo(t, "15 29 0:40 / /var/lib/containers/abc rw - tmpfs tmpfs rw\n")

	_, err := GetFromFile(path)
	if !errors.Is(err, ErrPodIDNotFound) {
		t.Fatalf("GetFromFile error = %v, want ErrPodIDNotFound", err)
	}
}

func TestGetCachesSuccessfulResult(t *testing.T) {
	restore := resetTestState()
	defer restore()

	want := "12345678-90ab-cdef-1234-567890abcdef"
	calls := 0
	getPodIDFunc = func() (string, error) {
		calls++
		return want, nil
	}

	got, err := Get()
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Get = %q, want %q", got, want)
	}
	if calls != 1 {
		t.Fatalf("Get called provider %d times, want 1", calls)
	}

	got, err = Get()
	if err != nil {
		t.Fatalf("Get second call returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Get second call = %q, want %q", got, want)
	}
	if calls != 1 {
		t.Fatalf("Get should not call provider again, calls = %d", calls)
	}
}

func TestGetDoesNotCacheFailure(t *testing.T) {
	restore := resetTestState()
	defer restore()

	want := "fedcba98-7654-3210-fedc-ba9876543210"
	calls := 0
	getPodIDFunc = func() (string, error) {
		calls++
		if calls == 1 {
			return "", ErrPodIDNotFound
		}
		return want, nil
	}

	if _, err := Get(); !errors.Is(err, ErrPodIDNotFound) {
		t.Fatalf("Get first call error = %v, want ErrPodIDNotFound", err)
	}
	if calls != 1 {
		t.Fatalf("Get first call should invoke provider once, calls = %d", calls)
	}

	got, err := Get()
	if err != nil {
		t.Fatalf("Get second call error: %v", err)
	}
	if got != want {
		t.Fatalf("Get second call = %q, want %q", got, want)
	}
	if calls != 2 {
		t.Fatalf("Get should invoke provider again after failure, calls = %d", calls)
	}
}

func TestGetFromFileHandlesLongLines(t *testing.T) {
	want := "0f8fad5b-d9cb-469f-a165-70867728950e"
	padding := strings.Repeat("a", 70*1024)
	line := fmt.Sprintf("29 37 0:25 / /var/lib/kubelet/pods/%s/etc-hosts%s rw,relatime - tmpfs tmpfs rw\n", want, padding)
	path := writeTempMountInfo(t, line)

	got, err := GetFromFile(path)
	if err != nil {
		t.Fatalf("GetFromFile returned error for long line: %v", err)
	}
	if got != want {
		t.Fatalf("GetFromFile for long line = %q, want %q", got, want)
	}
}
