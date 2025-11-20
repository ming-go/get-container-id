package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
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

var httpPort = "8080"

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

func main() {
	flag.StringVar(&httpPort, "httpPort", "8080", "-httpPort 8080")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

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

		b, err := json.Marshal(responseSuccess{Data: "Hello, ming-go!"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
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

		b, err := json.Marshal(responseSuccess{Data: resp})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(b)
	})

	mux.HandleFunc("/hostname", func(w http.ResponseWriter, r *http.Request) {
		name, err := os.Hostname()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		b, err := json.Marshal(responseSuccess{Data: name})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	mux.HandleFunc("/time", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(responseSuccess{Data: time.Now().Format(time.RFC3339)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	mux.HandleFunc("/timestamp", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(responseSuccess{Data: time.Now().Unix()})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	mux.HandleFunc("/timestamp_nano", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(responseSuccess{Data: time.Now().UnixNano()})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	http.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/counter", func(w http.ResponseWriter, r *http.Request) {
		currCount := atomic.AddUint64(&counter, 1)
		b, err := json.Marshal(responseSuccess{Data: strconv.FormatUint(currCount, 10)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(responseSuccess{Data: "Hello, world!"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	mux.HandleFunc("/pod_id", func(w http.ResponseWriter, r *http.Request) {
		pid, err := podid.Get()
		if err != nil {
			respErr := responseError{Errors: errs{Message: err.Error()}}
			b, marshalErr := json.Marshal(respErr)
			if marshalErr != nil {
				http.Error(w, marshalErr.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set(headerContentType, contentTypeJSON)
			status := http.StatusInternalServerError
			if errors.Is(err, podid.ErrPodIDNotFound) {
				status = http.StatusNotFound
			}
			w.WriteHeader(status)
			w.Write(b)
			return
		}

		b, err := json.Marshal(responseSuccess{Data: pid})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
	})

	mux.HandleFunc("/container_id", func(w http.ResponseWriter, r *http.Request) {
		containerID, err := getContainerID()
		if err != nil {
			respErr := responseError{Errors: errs{Message: err.Error()}}
			b, marshalErr := json.Marshal(respErr)
			if marshalErr != nil {
				http.Error(w, marshalErr.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set(headerContentType, contentTypeJSON)
			status := http.StatusInternalServerError
			if errors.Is(err, ErrContainerIDNotFound) {
				status = http.StatusNotFound
			}
			w.WriteHeader(status)
			w.Write(b)
			return
		}

		b, err := json.Marshal(responseSuccess{Data: containerID})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.Write(b)
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
