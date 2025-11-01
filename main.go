package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ming-go/lab/get-container-id/containerid"
	"go.uber.org/zap"
)

var replacer = strings.NewReplacer("\n", "")

var ErrContainerIDNotFound = errors.New("container ID not found")
var containerIDRegex = regexp.MustCompile(`[0-9a-f]{64}`)

func getConatinerID() (string, error) {
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

type stringCache struct {
	s    string
	flag bool
	sync.RWMutex
}

func (sc *stringCache) Get() (string, bool) {
	sc.RLock()
	defer sc.RUnlock()

	return sc.s, sc.flag
}

func (sc *stringCache) Set(s string) {
	sc.Lock()
	sc.s = s
	sc.flag = true
	sc.Unlock()
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

var httpPort = "8787"

func getRequestURL(r *http.Request) string {
	scheme := "http://"
	if r.TLS != nil {
		scheme = "https://"
	}

	return scheme + r.Host + r.RequestURI
}

func main() {
	flag.StringVar(&httpPort, "httpPort", "8787", "-httpPort 8787")
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}

	zap.ReplaceGlobals(logger)

	sc := stringCache{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reqBody := []byte{}
		if r.Body != nil { // Read
			reqBody, _ = io.ReadAll(r.Body)
		}
		r.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // Reset

		zapFields := []zap.Field{}
		zapFields = append(zapFields, zap.String("Request Method", r.Method))
		zapFields = append(zapFields, zap.String("Request URL", getRequestURL(r)))
		zapFields = append(zapFields, zap.String("Request URL Path", r.URL.Path))
		zapFields = append(zapFields, zap.String("Request Protocol", r.Proto))
		zapFields = append(zapFields, zap.Any("Request Header", r.Header))
		zapFields = append(zapFields, zap.Any("Remote Address", r.RemoteAddr))

		zapFields = append(zapFields, zap.ByteString("Request Body", reqBody))

		zap.L().Info("IncomeLog", zapFields...)

		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		b, err := json.Marshal(responseSuccess{Data: "Hello, ming-go!"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	var counter uint64

	mux.HandleFunc("/timeNowUnixNano", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(responseSuccess{Data: time.Now().UnixNano()})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	mux.HandleFunc("/counter", func(w http.ResponseWriter, r *http.Request) {
		currCount := atomic.AddUint64(&counter, 1)
		b, err := json.Marshal(responseSuccess{Data: strconv.FormatUint(currCount, 10)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(responseSuccess{Data: "Hello, world!"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	mux.HandleFunc("/containerID", func(w http.ResponseWriter, r *http.Request) {
		containerID, exists := sc.Get()
		if !exists {
			var err error
			containerID, err = getConatinerID()
			if err != nil {
				b, err := json.Marshal(responseError{Errors: errs{Message: err.Error()}})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write(b)
				return
			}

			sc.Set(containerID)
		}

		b, err := json.Marshal(responseSuccess{Data: containerID})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	go func() {
		for {
			currCount := atomic.LoadUint64(&counter)
			if currCount != 0 {
				zap.L().Info(
					"CounterLogger",
					zap.Uint64("Counter", atomic.LoadUint64(&counter)),
				)
			}

			atomic.CompareAndSwapUint64(&counter, counter, 0)
			<-time.After(1 * time.Second)
		}
	}()

	listener, err := net.Listen("tcp", ":"+httpPort)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{
		Handler: mux,
	}

	log.Println("http server started on :" + httpPort)
	httpServer.Serve(listener)
}
