package main

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Test generateRandomID generates valid UUIDv7 format
func TestGenerateRandomID(t *testing.T) {
	// UUIDv7 regex: xxxxxxxx-xxxx-7xxx-[89ab]xxx-xxxxxxxxxxxx
	uuidv7Regex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	for i := 0; i < 10; i++ {
		id, err := generateRandomID()
		if err != nil {
			t.Fatalf("generateRandomID() returned error: %v", err)
		}

		if !uuidv7Regex.MatchString(id) {
			t.Errorf("generateRandomID() = %q, does not match UUIDv7 format", id)
		}

		// Verify version 7
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("generateRandomID() = %q, expected 5 parts", id)
		}
		if !strings.HasPrefix(parts[2], "7") {
			t.Errorf("generateRandomID() = %q, third part should start with '7' (version)", id)
		}

		// Verify variant bits (10)
		firstChar := parts[3][0]
		if firstChar != '8' && firstChar != '9' && firstChar != 'a' && firstChar != 'b' {
			t.Errorf("generateRandomID() = %q, fourth part should start with 8/9/a/b (variant)", id)
		}
	}
}

// Test that UUIDv7s are sortable by time
func TestGenerateRandomID_Sortable(t *testing.T) {
	id1, err := generateRandomID()
	if err != nil {
		t.Fatalf("generateRandomID() error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	id2, err := generateRandomID()
	if err != nil {
		t.Fatalf("generateRandomID() error: %v", err)
	}

	// UUIDs generated later should sort after earlier ones
	if id1 >= id2 {
		t.Errorf("generateRandomID() not sortable: %q should be less than %q", id1, id2)
	}
}

// Test that generateRandomID generates unique IDs
func TestGenerateRandomID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		id, err := generateRandomID()
		if err != nil {
			t.Fatalf("generateRandomID() error: %v", err)
		}

		if ids[id] {
			t.Errorf("generateRandomID() generated duplicate ID: %q", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("generateRandomID() expected %d unique IDs, got %d", count, len(ids))
	}
}

// Test initInstanceID with environment variable
func TestInitInstanceID_WithEnvVar(t *testing.T) {
	// Save original value and restore after test
	originalID := instanceID
	originalEnv := os.Getenv("INSTANCE_ID")
	defer func() {
		instanceID = originalID
		if originalEnv != "" {
			os.Setenv("INSTANCE_ID", originalEnv)
		} else {
			os.Unsetenv("INSTANCE_ID")
		}
	}()

	// Set environment variable
	testID := "test-instance-12345"
	os.Setenv("INSTANCE_ID", testID)
	instanceID = "" // Reset

	err := initInstanceID()
	if err != nil {
		t.Fatalf("initInstanceID() returned error: %v", err)
	}

	if instanceID != testID {
		t.Errorf("initInstanceID() with env var: got %q, want %q", instanceID, testID)
	}
}

// Test initInstanceID without environment variable (auto-generate)
func TestInitInstanceID_WithoutEnvVar(t *testing.T) {
	// Save original value and restore after test
	originalID := instanceID
	originalEnv := os.Getenv("INSTANCE_ID")
	defer func() {
		instanceID = originalID
		if originalEnv != "" {
			os.Setenv("INSTANCE_ID", originalEnv)
		} else {
			os.Unsetenv("INSTANCE_ID")
		}
	}()

	// Unset environment variable
	os.Unsetenv("INSTANCE_ID")
	instanceID = "" // Reset

	err := initInstanceID()
	if err != nil {
		t.Fatalf("initInstanceID() returned error: %v", err)
	}

	if instanceID == "" {
		t.Error("initInstanceID() without env var: instanceID should not be empty")
	}

	// Should be a valid UUIDv7
	uuidv7Regex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidv7Regex.MatchString(instanceID) {
		t.Errorf("initInstanceID() without env var: got %q, not a valid UUIDv7", instanceID)
	}
}

// Test writeJSONSuccess
func TestWriteJSONSuccess(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
		want string
	}{
		{
			name: "string data",
			data: "hello",
			want: `{"data":"hello"}`,
		},
		{
			name: "number data",
			data: 42,
			want: `{"data":42}`,
		},
		{
			name: "map data",
			data: map[string]string{"key": "value"},
			want: `{"data":{"key":"value"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSONSuccess(w, tt.data)

			if w.Code != http.StatusOK {
				t.Errorf("writeJSONSuccess() status = %d, want %d", w.Code, http.StatusOK)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("writeJSONSuccess() Content-Type = %q, want %q", contentType, "application/json")
			}

			got := strings.TrimSpace(w.Body.String())
			if got != tt.want {
				t.Errorf("writeJSONSuccess() body = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test writeJSONError
func TestWriteJSONError(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		statusCode int
	}{
		{
			name:       "not found error",
			message:    "resource not found",
			statusCode: http.StatusNotFound,
		},
		{
			name:       "internal error",
			message:    "something went wrong",
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "bad request",
			message:    "invalid input",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSONError(w, tt.message, tt.statusCode)

			if w.Code != tt.statusCode {
				t.Errorf("writeJSONError() status = %d, want %d", w.Code, tt.statusCode)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("writeJSONError() Content-Type = %q, want %q", contentType, "application/json")
			}

			var resp responseError
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("writeJSONError() body unmarshal error: %v", err)
			}

			if resp.Errors.Message != tt.message {
				t.Errorf("writeJSONError() message = %q, want %q", resp.Errors.Message, tt.message)
			}
		})
	}
}

// Test writeJSONResponse with invalid data (should return 500)
func TestWriteJSONResponse_MarshalError(t *testing.T) {
	w := httptest.NewRecorder()

	// channels cannot be marshaled to JSON
	invalidData := make(chan int)
	writeJSONResponse(w, invalidData, http.StatusOK)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("writeJSONResponse() with invalid data: status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	body := w.Body.String()
	if !strings.Contains(body, "json") {
		t.Errorf("writeJSONResponse() with invalid data: expected JSON error in body, got %q", body)
	}
}

// Test getRequestURL
func TestGetRequestURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		uri      string
		hasTLS   bool
		expected string
	}{
		{
			name:     "http request",
			host:     "localhost:8080",
			uri:      "/test",
			hasTLS:   false,
			expected: "http://localhost:8080/test",
		},
		{
			name:     "https request",
			host:     "example.com",
			uri:      "/api/v1",
			hasTLS:   true,
			expected: "https://example.com/api/v1",
		},
		{
			name:     "with query string",
			host:     "api.example.com",
			uri:      "/search?q=test",
			hasTLS:   true,
			expected: "https://api.example.com/search?q=test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.uri, nil)
			req.Host = tt.host
			if tt.hasTLS {
				req.TLS = &tls.ConnectionState{}
			}

			got := getRequestURL(req)
			if got != tt.expected {
				t.Errorf("getRequestURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
