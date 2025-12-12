Here’s a spec you can hand straight to Claude Code as “here’s what to build.”

I’ll write it as if it’s the product/tech spec for the project.

---

# Project: MCP Code Sandbox Server

## 1. Purpose

Implement an **MCP-compatible HTTP + SSE server** in Go that lets a client (e.g. ChatGPT / Claude MCP) execute short snippets of **Python** or **TypeScript** code inside **sandboxed containers**, with per-conversation scratch storage mounted at `/data`.

The service must:

* Accept a `conversationId`, `language`, and `code`.
* Spin up the appropriate sandbox container (`runner-python`, `runner-typescript`, etc.).
* Bind a per-conversation directory on the host to `/data` inside the container.
* Pipe the code to the container’s STDIN and capture STDOUT/STDERR.
* Return success/failure, combined output, and a list of files in the sandbox, each with a **signed download URL**.
* Expose MCP-compatible HTTP+SSE endpoints with header-based authentication.

---

## 2. Tech Stack

* **Language:** Go (latest stable)
* **Process model:** Single binary
* **HTTP server:** `net/http` (optionally with a router like `chi` if helpful)
* **Docker integration:** Docker SDK for Go (`github.com/docker/docker/client`)
* **Signing:** HMAC-SHA256 (`crypto/hmac`, `crypto/sha256`)
* **Wire protocol for MCP:** JSON-RPC 2.0 over HTTP and SSE

The server is expected to run in a Docker / container environment itself, with access to the Docker daemon (via socket).

---

## 3. Environment Configuration

Use environment variables:

* `MCP_HTTP_ADDR`

  * Address to bind HTTP server, e.g. `":8080"`.
* `MCP_API_TOKEN`

  * Bearer token for protected endpoints (MCP JSON-RPC).
* `SANDBOX_ROOT`

  * Root folder on host for all conversation sandboxes, e.g. `/var/sandboxes`.
* `FILE_SECRET`

  * Secret key used to sign file URLs.
* `PUBLIC_BASE_URL`

  * Base URL used to construct file download URLs, e.g. `https://sandbox.example.com`.
* `DOCKER_HOST` (if necessary)

  * For Docker SDK configuration (or use default Unix socket).

---

## 4. High-Level Components

1. **HTTP Server**

   * Routes:

     * `POST /mcp` – JSON-RPC 2.0 endpoint (MCP methods).
     * `GET /mcp/events` – SSE endpoint for JSON-RPC notifications.
     * `GET /files/{conversationId}/{filename}` – Raw file download with signed URLs.
2. **Auth Middleware**

   * Validates `Authorization: Bearer <MCP_API_TOKEN>` on `/mcp` and `/mcp/events`.
   * `/files/...` does **not** require header auth (URLs are signed).
3. **Runner Registry**

   * Discovers available runner images at startup and maps `language` → `image`.
4. **Sandbox Manager**

   * Manages filesystem layout under `SANDBOX_ROOT`.
5. **Container Runner**

   * Creates and manages Docker containers to execute code.
6. **File Signer & Validator**

   * Generates and verifies signed download URLs.

---

## 5. Filesystem Layout

Under `SANDBOX_ROOT`:

* `<SANDBOX_ROOT>/<conversationId>/`

  * `files/` → bound to `/data` inside containers.
  * (Optional extras like `logs/`, `meta.json` if desired.)

### Behavior

* On each `run_code` call:

  * Ensure `<SANDBOX_ROOT>/<conversationId>/files` exists (mkdir -p).
* The system does **not** auto-delete sandboxes; retention policy can be added later.

---

## 6. Docker Runner Images

### 6.1 Naming & Discovery

Runner images are built externally (via a separate build script) from:

* `Dockerfile-python` → image `runner-python`
* `Dockerfile-typescript` → image `runner-typescript`

Each image must be labeled:

* `sandbox.runner=true`
* `sandbox.language=<language>` (e.g. `python`, `typescript`)

At server startup:

* Use Docker client to list images with label `sandbox.runner=true`.
* For each image, read `sandbox.language` label and populate:

  ```go
  type RunnerInfo struct {
      Image    string
      Language string
  }

  var runnersByLanguage map[string]RunnerInfo
  ```
* If a request references a `language` not present in `runnersByLanguage`, return a controlled error.

### 6.2 Runtime Contract

Each runner image must satisfy:

* Working directory: `/data`
* Entry point: a script/binary that:

  * Reads source code from STDIN.
  * Writes it to a temp file (`/tmp`).
  * Executes it with the relevant interpreter.
  * Uses `/data` as the working directory so user code can read/write files there.
  * Writes program output to STDOUT and STDERR.

#### Runner Example (Python) – behavior

* Image: `runner-python`
* On container start:

  1. Read all of STDIN into `/tmp/main-XXXX.py`.
  2. `cd /data`
  3. Run `python /tmp/main-XXXX.py`.
  4. Exit with the same exit code as Python process.

#### Runner Example (TypeScript) – behavior

* Image: `runner-typescript`
* On container start:

  1. Read all of STDIN into `/tmp/main-XXXX.ts`.
  2. `cd /data`
  3. Run `ts-node /tmp/main-XXXX.ts` (or similar configured interpreter).
  4. Exit with the same exit code as the interpreter.

---

## 7. JSON-RPC / MCP API

We’ll implement a minimal subset:

### 7.1 JSON-RPC Transport

* Endpoint: `POST /mcp`
* Content-Type: `application/json`
* Body: JSON-RPC 2.0 request.

#### Request structure (generic JSON-RPC)

```json
{
  "jsonrpc": "2.0",
  "id": "some-id",
  "method": "tools/call",
  "params": {
    "name": "sandbox.run_code",
    "arguments": {
      "conversationId": "abc123",
      "language": "python",
      "code": "print('hello')"
    }
  }
}
```

For now, implement:

* `tools/call` with tool names:

  * `sandbox.run_code`
  * `sandbox.list_runners` (optional but useful)

### 7.2 `sandbox.run_code` Tool

**Input (arguments object):**

```json
{
  "conversationId": "string (non-empty)",
  "language": "python | typescript | future",
  "code": "string (source code)"
}
```

**Output (wrapped in MCP tool result):**

```json
{
  "jsonrpc": "2.0",
  "id": "<same as request>",
  "result": {
    "content": [
      {
        "type": "output",
        "data": {
          "success": true,
          "output": "combined stdout and stderr as a single string",
          "files": [
            {
              "name": "result.csv",
              "url": "https://.../files/abc123/result.csv?sig=..."
            }
          ]
        }
      }
    ]
  }
}
```

Exact shapes of `content` can be adapted to MCP’s expectations, but the nested `data` object must have at least:

* `success: bool`
* `output: string`
* `files: Array<{name: string, url: string}>`

#### Error case behavior

On failures (e.g., missing runner, container error, timeout):

* Return `success: false`
* Include diagnostic info in `output` (e.g., error messages, stack trace).
* `files` may be empty or partial if some files got created before failure.

#### Go data structures

```go
type RunCodeArguments struct {
    ConversationID string `json:"conversationId"`
    Language       string `json:"language"`
    Code           string `json:"code"`
}

type FileDescriptor struct {
    Name string `json:"name"`
    URL  string `json:"url"`
}

type RunCodeResult struct {
    Success bool             `json:"success"`
    Output  string           `json:"output"`
    Files   []FileDescriptor `json:"files"`
}
```

### 7.3 `sandbox.list_runners` Tool (optional but recommended)

Tool for debugging / introspection.

**Input:** no arguments.

**Output:**

```json
{
  "languages": [
    {
      "language": "python",
      "image": "runner-python"
    },
    {
      "language": "typescript",
      "image": "runner-typescript"
    }
  ]
}
```

---

## 8. Execution Flow for `sandbox.run_code`

Pseudocode for main logic in Go.

### 8.1 Validation & Sandbox Setup

1. **Auth check** – verify `Authorization` header.
2. **Parse JSON-RPC** request.
3. Validate `conversationId`, `language`, `code`:

   * If `conversationId == ""` → error.
   * If `language` not found in `runnersByLanguage` → error.
4. Compute sandbox path:

   ```go
   sandboxDir := filepath.Join(sandboxRoot, args.ConversationID, "files")
   if err := os.MkdirAll(sandboxDir, 0o700); err != nil { ... }
   ```

### 8.2 Launch Container

Use Docker SDK:

* Image: `runnersByLanguage[args.Language].Image`
* Mounts:

  * Host: `sandboxDir`
  * Container: `/data`

Container creation (conceptual):

* **Config:**

  * `Image: ...`
  * `WorkingDir: "/data"`
  * `Cmd: []string{}` (use default entrypoint).
  * Disable network if possible (security).
  * Set resource limits if desired (CPU, memory).

* **HostConfig:**

  * `Binds: []string{sandboxDir + ":/data"}`

Steps:

1. `cli.ContainerCreate(...)`

2. Attach streams:

   * `ContainerAttach` for STDIN, STDOUT, STDERR.

3. Start container: `ContainerStart`.

4. Write code to container’s STDIN in a goroutine:

   ```go
   go func() {
       io.WriteString(stdin, args.Code)
       stdin.Close()
   }()
   ```

5. Concurrently read STDOUT and STDERR into buffers:

   ```go
   var bufOut, bufErr bytes.Buffer
   go io.Copy(&bufOut, stdout)
   go io.Copy(&bufErr, stderr)
   ```

6. Wait for container exit with timeout (`context.WithTimeout` for 30 seconds or configurable):

   ```go
   statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
   // handle with select on statusCh / errCh / timeout
   ```

7. After exit or timeout, call `ContainerRemove` to clean up.

### 8.3 Construct Result

1. Determine success:

   ```go
   success := (exitCode == 0 && !timedOut && !internalError)
   ```

2. Combine output:

   ```go
   combined := bufOut.String()
   if stderrStr := bufErr.String(); stderrStr != "" {
       if combined != "" {
           combined += "\n"
       }
       combined += stderrStr
   }
   ```

3. List files in sandbox directory:

   ```go
   entries, err := os.ReadDir(sandboxDir)
   var files []FileDescriptor
   for _, e := range entries {
       if e.IsDir() {
           continue
       }
       name := e.Name()
       url := makeFileURL(publicBaseURL, args.ConversationID, name, fileSecret)
       files = append(files, FileDescriptor{Name: name, URL: url})
   }
   ```

4. Build `RunCodeResult` and wrap in JSON-RPC `result`.

---

## 9. File Download API

### 9.1 URL Format

* Path: `/files/{conversationId}/{filename}`
* Query: `?sig=<signature>`

**Example:**

`GET /files/abc123/result.csv?sig=deadbeef...`

### 9.2 Signature Scheme

* Env: `FILE_SECRET`
* Signature formula:

  * Input: `conversationId + "/" + filename`
  * `sig = HMAC_SHA256(FILE_SECRET, input)`
  * Represented as hex.

#### Helper

```go
func signPath(secret, conversationID, filename string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(conversationID + "/" + filename))
    return hex.EncodeToString(mac.Sum(nil))
}
```

`makeFileURL`:

```go
func makeFileURL(baseURL, conversationID, filename, secret string) string {
    sig := signPath(secret, conversationID, filename)
    return fmt.Sprintf("%s/files/%s/%s?sig=%s",
        strings.TrimRight(baseURL, "/"),
        url.PathEscape(conversationID),
        url.PathEscape(filename),
        sig,
    )
}
```

### 9.3 Handler Behavior

`GET /files/{conversationId}/{filename}`:

1. Extract `conversationId`, `filename`, and `sig`.

2. Compute expected signature with `signPath`.

3. If not equal to provided `sig` (constant-time comparison), respond `403 Forbidden`.

4. Build file path:

   ```go
   path := filepath.Join(sandboxRoot, conversationID, "files", filename)
   ```

5. If file doesn’t exist or not regular, respond `404 Not Found`.

6. Otherwise, serve file contents (`http.ServeFile`).

No header auth required; security is via unguessable signed URL.

---

## 10. SSE Endpoint

`GET /mcp/events`:

* Auth: same Bearer token as `/mcp`.
* For v1, you can keep it minimal:

  * Accept connection, write an initial SSE comment or heartbeat.
  * When implementing streaming / notifications later, send JSON-RPC notifications as SSE `data:` lines.
* For now, a no-op or stubbed SSE endpoint is acceptable if MCP client doesn’t strictly require streaming.

---

## 11. Error Handling & Logging

* All JSON-RPC errors should set `error` field per JSON-RPC 2.0 when something goes wrong at the protocol level.
* For tool-level errors (e.g. code fails), prefer returning a successful JSON-RPC response with `success: false` in the tool result.
* Log:

  * Incoming requests (sanitized; don’t log entire code by default unless debug).
  * Container creation/exit/cleanup events.
  * File serving errors (without leaking secrets).

---

## 12. Security Considerations

* Containers should:

  * Have no network access (if possible via Docker’s config).
  * Run as non-root if possible.
  * Use minimal base images (alpine, etc.).
* Limit CPU/memory:

  * Use Docker resource constraints per container (e.g., 0.5 CPU, 256MB RAM – configurable).
* Add a reasonable **timeout** on code execution (e.g., 30 seconds).

---

## 13. Possible Extensions (Not Required for v1)

* Time-limited signed URLs (e.g. `expires` query param included in HMAC).
* Per-conversation resource quotas (max files, max total size).
* Streaming stdout/stderr via SSE while run is in progress.
* Reusable container pools per runner to reduce startup latency.


BONUS:

Root of server should host basic page that prompts for API Key, language, code, conversation ID and gives a graphical runner for testing this sucker out. 

---



If you paste this into Claude Code and say “generate the Go implementation and Dockerfiles for this spec,” it should have a very clear blueprint to follow.
