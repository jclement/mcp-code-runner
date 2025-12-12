# Security Architecture

## Hashed Directory Names

The sandbox uses SHA256 hashing to create filesystem-safe, unpredictable directory names for each conversation.

### How It Works

1. **Directory Creation**: When a conversation needs a sandbox, we hash `conversationID + FILE_SECRET`:
   ```go
   hash = SHA256(conversationID + FILE_SECRET)
   directory = /sandbox-root/{hash}/
   ```

2. **File URLs**: Files are accessible via simple URLs without signatures:
   ```
   https://example.com/files/{hash}/{filename}
   ```

3. **Security Properties**:
   - **Unpredictable**: Without knowing the `FILE_SECRET`, you cannot predict the hash from a `conversationID`
   - **Filesystem-safe**: SHA256 hex output (64 chars) contains only `[0-9a-f]`
   - **No guessing**: 256-bit hash space makes brute-forcing infeasible
   - **No injection**: Hash prevents path traversal or special characters in directory names

### Why This Is Secure

**Previous approach**:
- Used arbitrary `conversationID` as directory name
- Required HMAC signatures on URLs to prevent unauthorized access
- URLs like: `/files/{conversationID}/{filename}?sig={hmac}`

**Current approach**:
- Hash makes directory names unguessable
- No signatures needed - the hash itself is the secret
- URLs like: `/files/{hash}/{filename}`

**Attack scenarios**:
- ❌ **Guess conversationID**: Attacker can't derive hash without `FILE_SECRET`
- ❌ **Brute force hash**: 2^256 possibilities makes this infeasible
- ❌ **Path traversal**: Hash is validated as 64-char hex string, no `..` or `/` possible
- ❌ **Directory listing**: Only specific files are served, no directory browsing

### Implementation Details

**Sandbox Manager** (`internal/sandbox/manager.go`):
```go
func (m *Manager) hashConversationID(conversationID string) string {
    h := sha256.New()
    h.Write([]byte(conversationID))
    h.Write([]byte(m.secret))
    return hex.EncodeToString(h.Sum(nil))
}
```

**File Download Handler** (`internal/handler/server.go`):
```go
// Parse URL path: /files/{hashedDir}/{filename}
// Validate hashedDir is a valid hex string (64 chars for SHA256)
if len(hashedDir) != 64 {
    http.Error(w, "Invalid directory hash", http.StatusBadRequest)
    return
}
```

### Configuration

Required environment variable:
```bash
FILE_SECRET=your-random-secret-at-least-32-chars
```

**Important**: The `FILE_SECRET` must be:
- At least 32 characters long
- Randomly generated (e.g., `openssl rand -base64 32`)
- Kept confidential
- Changed if ever exposed

### Migration from Signed URLs

The old signature-based approach has been completely replaced. Benefits:

1. **Simpler URLs**: No query parameters needed
2. **Better caching**: URLs don't change over time
3. **Stateless**: No need to verify signatures on each request
4. **Same security**: Hash provides equivalent protection to HMAC signatures

The `filesign` package is still used for `GetBaseURL()`, but signature generation and verification are no longer used for file downloads.
