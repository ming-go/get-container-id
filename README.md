# get-container-id

HTTP server that provides container and Kubernetes pod identification information along with utility endpoints.

## Features

- Extract container ID from cgroup v1/v2
- Extract Kubernetes pod ID (UUID) from mountinfo
- UUIDv7 instance identifier generation
- Health check endpoints
- Request echo and debugging utilities

## Quick Start

```bash
# Build
go build -v .

# Run with default port (8080)
./get-container-id

# Run with custom port
./get-container-id -httpPort 9000

# Run with environment variables
PORT=9000 INSTANCE_ID=custom-id ./get-container-id
```

### Docker

```bash
# Build image
./build.sh v1.0.0

# Run container
docker run -p 8080:8080 get-container-id:v1.0.0
```

## Configuration

### Command-line Flags

- `-httpPort` - HTTP server port (default: 8080)

### Environment Variables

- `PORT` - HTTP server port (overridden by `-httpPort` flag)
- `INSTANCE_ID` - Custom instance identifier (auto-generates UUIDv7 if not set)

## API Endpoints

### GET /

Root endpoint.

```bash
curl http://localhost:8080/
```

Response:
```json
{"data":"Hello, ming-go!"}
```

### GET /hello

Hello world endpoint.

```bash
curl http://localhost:8080/hello
```

Response:
```json
{"data":"Hello, world!"}
```

### GET /id

Returns the instance identifier (UUIDv7 format).

```bash
curl http://localhost:8080/id
```

Response:
```json
{"data":"019aa0d4-50c0-71d5-8318-c5400284ce60"}
```

### GET /hostname

Returns the container hostname.

```bash
curl http://localhost:8080/hostname
```

Response:
```json
{"data":"my-hostname"}
```

### GET /container_id

Returns the container ID (64-character hex string).

```bash
curl http://localhost:8080/container_id
```

Response (success):
```json
{"data":"a1b2c3d4e5f6..."}
```

Response (not in container):
```json
{"errors":{"message":"container ID not found"}}
```

### GET /pod_id

Returns the Kubernetes pod ID (UUID).

```bash
curl http://localhost:8080/pod_id
```

Response (success):
```json
{"data":"036da4f7-d553-4eb6-9802-90f81041a412"}
```

Response (not in pod):
```json
{"errors":{"message":"pod ID (UUID) not found in /proc/self/mountinfo"}}
```

### GET /time

Returns current time in RFC3339 format.

```bash
curl http://localhost:8080/time
```

Response:
```json
{"data":"2025-01-15T10:30:45Z"}
```

### GET /timestamp

Returns current Unix timestamp (seconds).

```bash
curl http://localhost:8080/timestamp
```

Response:
```json
{"data":1737025845}
```

### GET /timestamp_nano

Returns current Unix timestamp (nanoseconds).

```bash
curl http://localhost:8080/timestamp_nano
```

Response:
```json
{"data":1737025845123456789}
```

### GET /counter

Request counter that increments on each call.

```bash
curl http://localhost:8080/counter
curl http://localhost:8080/counter
curl http://localhost:8080/counter
```

Response:
```json
{"data":"1"}
{"data":"2"}
{"data":"3"}
```

### POST /echo

Echoes back request details including method, headers, query, and body.

```bash
curl -X POST http://localhost:8080/echo?key=value \
  -H "Content-Type: application/json" \
  -H "X-Custom-Header: test" \
  -d '{"test":"data"}'
```

Response:
```json
{
  "data": {
    "method": "POST",
    "path": "/echo",
    "query": "key=value",
    "header": {
      "Content-Type": ["application/json"],
      "X-Custom-Header": ["test"]
    },
    "host": "localhost:8080",
    "remote": "127.0.0.1:12345",
    "body": "{\"test\":\"data\"}"
  }
}
```

### GET /livez

Liveness probe for health checks.

```bash
curl http://localhost:8080/livez
```

Response:
```
ok
```

### GET /readyz

Readiness probe for health checks.

```bash
curl http://localhost:8080/readyz
```

Response:
```
ok
```

## Development

### Run Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Verbose output
go test -v ./...
```

### Build

```bash
# Local build
go build -v .

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -v .

# Using build script
./build.sh v1.0.0
```

## Project Structure

```
.
├── main.go              # HTTP server and handlers
├── main_test.go         # Unit and integration tests
├── containerid/         # Container ID extraction
│   ├── containerid.go
│   └── containerid_test.go
├── podid/               # Kubernetes pod ID extraction
│   ├── podid.go
│   └── podid_test.go
├── Dockerfile           # Container image definition
├── build.sh             # Build script
└── README.md
```

## Requirements

- Go 1.22 or later
- Linux kernel with cgroup support (for container/pod ID detection)

## License

See LICENSE file for details.
