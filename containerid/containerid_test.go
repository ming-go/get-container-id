package containerid

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func resetTestState() func() {
	origFunc := getFunc
	origCachedID := cachedID
	origHasID := hasID

	cachedID = ""
	hasID = false
	mu = sync.RWMutex{}
	getFunc = get

	return func() {
		cachedID = origCachedID
		hasID = origHasID
		mu = sync.RWMutex{}
		getFunc = origFunc
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
	want := strings.Repeat("a", 64)
	content := strings.Join([]string{
		"12582 11777 0:786 / / rw,relatime - overlay overlay rw",
		fmt.Sprintf("12590 12584 259:2 /var/lib/kubelet/pods/036da4f7/containers/debug/%s/hostname /etc/hostname rw,relatime - ext4 /dev/nvme0n1p2 rw", want),
	}, "\n") + "\n"
	path := writeTempMountInfo(t, content)

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
	if err == nil {
		t.Fatal("GetFromFile expected error, got nil")
	}
	if !strings.Contains(err.Error(), "container ID not found") {
		t.Fatalf("GetFromFile error = %v, want 'container ID not found'", err)
	}
}

func TestGetCachesSuccessfulResult(t *testing.T) {
	restore := resetTestState()
	defer restore()

	want := strings.Repeat("b", 64)
	calls := 0
	getFunc = func() (string, error) {
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

	want := strings.Repeat("c", 64)
	calls := 0
	testErr := errors.New("temporary failure")
	getFunc = func() (string, error) {
		calls++
		if calls == 1 {
			return "", testErr
		}
		return want, nil
	}

	if _, err := Get(); !errors.Is(err, testErr) {
		t.Fatalf("Get first call error = %v, want %v", err, testErr)
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
	id := strings.Repeat("d", 64)
	padding := strings.Repeat("x", 70*1024)
	line := fmt.Sprintf("29 37 0:25 / /var/lib/kubelet/pods/123/containers/app/%s/hosts%s rw,relatime - tmpfs tmpfs rw\n", id, padding)
	path := writeTempMountInfo(t, line)

	got, err := GetFromFile(path)
	if err != nil {
		t.Fatalf("GetFromFile returned error for long line: %v", err)
	}
	if got != id {
		t.Fatalf("GetFromFile for long line = %q, want %q", got, id)
	}
}
