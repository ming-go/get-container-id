package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ming-go/lab/get-container-id/containerid"
	"github.com/ming-go/lab/get-container-id/podid"
)

var replacer = strings.NewReplacer("\n", "")

var ErrContainerIDNotFound = errors.New("container ID not found")
var containerIDRegex = regexp.MustCompile(`[0-9a-f]{64}`)

// instanceID holds the unique identifier for this instance.
// It can be set via INSTANCE_ID environment variable or is auto-generated.
var instanceID string

// generateRandomID generates a UUIDv7 identifier with timestamp and random components.
// UUIDv7 format: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx
// - First 48 bits: Unix timestamp in milliseconds
// - Next 12 bits: sub-millisecond precision (random)
// - Version bits: 0111 (7)
// - Variant bits: 10
// - Remaining 62 bits: random
func generateRandomID() (string, error) {
	b := make([]byte, 16)

	// Get current Unix timestamp in milliseconds (48 bits)
	timestamp := time.Now().UnixMilli()

	// Place timestamp in first 6 bytes (48 bits)
	b[0] = byte(timestamp >> 40)
	b[1] = byte(timestamp >> 32)
	b[2] = byte(timestamp >> 24)
	b[3] = byte(timestamp >> 16)
	b[4] = byte(timestamp >> 8)
	b[5] = byte(timestamp)

	// Fill remaining bytes with random data
	if _, err := rand.Read(b[6:]); err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}

	// Set version bits to 7 (0111) in byte 6, high nibble
	b[6] = (b[6] & 0x0f) | 0x70

	// Set variant bits to 10 in byte 8, high 2 bits
	b[8] = (b[8] & 0x3f) | 0x80

	// Format as UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}

// initInstanceID initializes the instance ID from environment variable or generates a random one.
func initInstanceID() error {
	// Try to get from environment variable first
	if id := os.Getenv("INSTANCE_ID"); id != "" {
		instanceID = id
		return nil
	}

	// Generate random ID if not set
	id, err := generateRandomID()
	if err != nil {
		return err
	}
	instanceID = id
	return nil
}

func getContainerID() (string, error) {
	b, err := os.ReadFile("/proc/self/cpuset")
	if err != nil {
		return "", err
	}

	cpuset := string(b)

	// cgroup v1
	if strings.TrimSpace(cpuset) != "/" {
		cpusetSplit := strings.Split(cpuset, "/")
		return replacer.Replace(cpusetSplit[len(cpusetSplit)-1]), nil
	}

	// cgroup v2
	id, err := containerid.Get()

	if id == "" {
		return "", ErrContainerIDNotFound
	}

	return id, nil
}

type responseSuccess struct {
	Data interface{} `json:"data"`
}

type errs struct {
	Message string `json:"message"`
}

type responseError struct {
	Errors errs `json:"errors"`
}

var httpPort string

const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
	maxBodySize       = 1 << 20 // 1MB
)

func getRequestURL(r *http.Request) string {
	scheme := "http://"
	if r.TLS != nil {
		scheme = "https://"
	}

	return scheme + r.Host + r.RequestURI
}

// writeJSONResponse marshals data to JSON and writes it to the response with the given status code.
// If marshaling fails, it writes an HTTP 500 error instead.
func writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	b, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(statusCode)
	w.Write(b)
}

// writeJSONSuccess is a convenience wrapper for writeJSONResponse that wraps data in responseSuccess
// and uses HTTP 200 status code.
func writeJSONSuccess(w http.ResponseWriter, data interface{}) {
	writeJSONResponse(w, responseSuccess{Data: data}, http.StatusOK)
}

// writeJSONError is a convenience wrapper for writeJSONResponse that wraps an error message
// in responseError and uses the given status code.
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	writeJSONResponse(w, responseError{Errors: errs{Message: message}}, statusCode)
}

func main() {
	// Get default port from PORT env variable, or use "8080"
	defaultPort := "8080"
	if port := os.Getenv("PORT"); port != "" {
		defaultPort = port
	}

	flag.StringVar(&httpPort, "httpPort", defaultPort, "HTTP server port (also configurable via PORT env variable)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Initialize instance ID
	if err := initInstanceID(); err != nil {
		logger.Error("failed to initialize instance ID", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("instance ID initialized", slog.String("instance_id", instanceID))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reqBody := []byte{}
		if r.Body != nil { // Read
			var err error
			reqBody, err = io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // Reset

		logger.Info(
			"IncomeLog",
			slog.String("request_method", r.Method),
			slog.String("request_url", getRequestURL(r)),
			slog.String("request_url_path", r.URL.Path),
			slog.String("request_protocol", r.Proto),
			slog.Any("request_header", r.Header),
			slog.String("remote_address", r.RemoteAddr),
			slog.Any("request_body", reqBody),
		)

		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		writeJSONSuccess(w, "Hello, ming-go!")
	})

	var counter uint64

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
		_ = r.Body.Close()

		resp := map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"query":  r.URL.RawQuery,
			"header": r.Header,
			"host":   r.Host,
			"remote": r.RemoteAddr,
			"body":   string(body),
		}

		writeJSONSuccess(w, resp)
	})

	mux.HandleFunc("/hostname", func(w http.ResponseWriter, r *http.Request) {
		name, err := os.Hostname()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONSuccess(w, name)
	})

	mux.HandleFunc("/time", func(w http.ResponseWriter, r *http.Request) {
		writeJSONSuccess(w, time.Now().Format(time.RFC3339))
	})

	mux.HandleFunc("/timestamp", func(w http.ResponseWriter, r *http.Request) {
		writeJSONSuccess(w, time.Now().Unix())
	})

	mux.HandleFunc("/timestamp_nano", func(w http.ResponseWriter, r *http.Request) {
		writeJSONSuccess(w, time.Now().UnixNano())
	})

	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/counter", func(w http.ResponseWriter, r *http.Request) {
		currCount := atomic.AddUint64(&counter, 1)
		writeJSONSuccess(w, strconv.FormatUint(currCount, 10))
	})

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSONSuccess(w, "Hello, world!")
	})

	mux.HandleFunc("/id", func(w http.ResponseWriter, r *http.Request) {
		writeJSONSuccess(w, instanceID)
	})

	mux.HandleFunc("/pod_id", func(w http.ResponseWriter, r *http.Request) {
		pid, err := podid.Get()
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, podid.ErrPodIDNotFound) {
				status = http.StatusNotFound
			}
			writeJSONError(w, err.Error(), status)
			return
		}

		writeJSONSuccess(w, pid)
	})

	mux.HandleFunc("/container_id", func(w http.ResponseWriter, r *http.Request) {
		containerID, err := getContainerID()
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrContainerIDNotFound) {
				status = http.StatusNotFound
			}
			writeJSONError(w, err.Error(), status)
			return
		}

		writeJSONSuccess(w, containerID)
	})

	go func() {
		for {
			currCount := atomic.LoadUint64(&counter)
			if currCount != 0 {
				logger.Info(
					"CounterLogger",
					slog.Uint64("counter", currCount),
				)
			}

			atomic.CompareAndSwapUint64(&counter, currCount, 0)
			<-time.After(1 * time.Second)
		}
	}()

	listener, err := net.Listen("tcp", ":"+httpPort)
	if err != nil {
		logger.Error("failed to create listener", slog.String("port", httpPort), slog.Any("error", err))
		os.Exit(1)
	}

	httpServer := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("http server started", slog.String("port", httpPort))

	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server stopped with error", slog.Any("error", err))
	}
}
