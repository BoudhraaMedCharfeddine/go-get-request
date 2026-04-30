# go-get-request

A lightweight mock HTTP server with a built-in web GUI — designed to help frontend developers test their apps without a real backend.

Define routes, configure responses, start the server, and watch every incoming request in real time.

---

## Features

- **Web GUI** — create and manage mock routes from a browser interface
- **Dynamic mock server** — start and stop on any port without restarting the app
- **Route matching** — exact paths and wildcard (`/api/users/*`)
- **Any method** — GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, or `*` for all
- **Custom responses** — status code, headers, body, and optional delay (ms)
- **Real-time logs** — every request appears instantly via SSE, with matched/unmatched status
- **Request detail** — click any log entry to inspect headers and body
- **CORS enabled** — mock server adds CORS headers automatically, frontends can call it directly
- **Zero dependencies** — Go standard library only

---

## Getting started

**Prerequisites:** Go 1.22+

```bash
git clone https://github.com/your-username/go-get-request.git
cd go-get-request
go run .
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

To use a different GUI port:

```bash
go run . --port 9000
```

Build a standalone binary:

```bash
go build -o go-get-request .
./go-get-request
```

---

## Usage

### 1. Create a route

Click **+ New** in the sidebar, fill in the form, and click **Save Route**.

| Field | Description |
|---|---|
| Method | HTTP verb — or `*` to match any method |
| Path | Exact path or wildcard, e.g. `/api/users/*` |
| Status Code | HTTP response status (default `200`) |
| Delay | Simulated latency in milliseconds |
| Response Headers | Key/value pairs, e.g. `Content-Type: application/json` |
| Response Body | Raw response body (plain text, JSON, HTML…) |

### 2. Start the mock server

Enter a port (default `3000`) in the header and click **▶ Start**.  
Your frontend can now call `http://localhost:3000` and receive the configured responses.

### 3. Watch requests

Every incoming request appears in the **Request Logs** panel at the bottom — color-coded by status and flagged as `matched` or `no match`. Click a row to inspect the full request headers and body.

---

## Path matching

| Route path | Matches |
|---|---|
| `/api/users` | only `GET /api/users` |
| `/api/users/*` | `/api/users/1`, `/api/users/profile`, etc. |
| `*` (method) | any HTTP method |

---

## REST API

The GUI itself is backed by a REST API you can use directly.

### Routes

```
GET    /api/routes          list all routes
POST   /api/routes          create a route
PUT    /api/routes/:id      update a route
DELETE /api/routes/:id      delete a route
```

**Route payload:**
```json
{
  "method": "GET",
  "path": "/api/users",
  "statusCode": 200,
  "delayMs": 0,
  "responseHeaders": {
    "Content-Type": "application/json"
  },
  "responseBody": "{\"users\": []}"
}
```

### Mock server

```
GET  /api/mock/status       current status { running, port }
POST /api/mock/start        start  — body: { "port": 3000 }
POST /api/mock/stop         stop
```

### Logs

```
GET  /api/logs              list captured requests
POST /api/logs/clear        clear all logs
GET  /api/events            SSE stream (real-time log + status updates)
```

---

## Project structure

```
go-get-request/
├── main.go            entry point, CLI flag parsing
├── store/store.go     in-memory store for routes and logs
├── events/bus.go      pub/sub bus for SSE broadcasting
├── mock/server.go     dynamic mock HTTP server
└── gui/
    ├── server.go      GUI HTTP server, REST API, SSE handler
    └── web/
        └── index.html single-file web app (embedded in the binary)
```

---

## License

MIT
