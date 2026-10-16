# Proxylib Deduplication & Shared Architecture Roadmap

Following the removal of historical quota metrics storage from both `codex-proxy` and `antigravity-proxy`, this document analyzes remaining code overlap between the two proxies and outlines a phased plan to extract reusable, provider-neutral components into `github.com/terastal-hq/proxylib`.

---

## 1. Code Overlap Overview

While the core proxy engines (upstream protocol handling, provider-specific inference APIs, quota wire parsing, OAuth exchange mechanics) are genuinely distinct, the management and infrastructure layers surrounding them are largely identical.

| Area / File | Size | Overlap & Similarity |
|---|---|---|
| `cmd/*/commands.go` (CLI) | ~289 LOC | **~90% identical**: Differences are strictly app name strings, CLI prompt labels, and local package imports. |
| `internal/config/accounts.go` | ~238 LOC | **~60% identical**: Identical `AccountKey` model, identical `ValidateAccounts` logic (differing only in error message provider label), identical raw JSON patch and legacy field stripping patterns. |
| `internal/proxy/admin.go` | ~700 LOC | **~65% identical**: Over 100 lines differ only in JSON writer helpers (`writeAdminJSON` vs `httpjson.Write`). Account CRUD, API key lifecycle, and quota inspection handlers share structure and validation flow. |
| `internal/oauth/session.go` | ~700 LOC | **~30% structural overlap**: Identical public DTO types (`SessionOptions`, `SessionInfo`, `SessionStatus`, `IssuedKey`, `AccountConfigResult`), but different exchange internals (PKCE callback server vs Google OAuth token exchange). |
| `internal/status/` (Quota polling) | ~200 LOC | **~85% algorithmic overlap**: Both poll on an interval or immediate trigger, store latest `quota.Snapshot` in memory, compute staleness over 2x polling interval or last error, and expose status without disk I/O. |
| File I/O & Durability | ~150 LOC | **~85% identical**: Both enforce owner-only permissions (0600), reject symlinks, validate regular files, require top-level JSON objects, reject duplicate case-insensitive keys, write via temp file + fsync + atomic rename + directory sync, and define `CommitError`. |
| HTTP & Lifecycle Utilities | ~100 LOC | **~90% identical**: Unescaped JSON response writing, bounded body reading, bearer token extraction, unread body rejection, and idempotent lifecycle `Closer`. |

---

## 2. Extraction Roadmap by Priority & Risk

### Tier 1: Low Risk, High Payoff (Recommended Next Phase)

These components are clean, provider-agnostic, and have existing 100% test coverage.

1. **Quota Poller (`proxylib/poller`)**
   * **Problem**: `AccountService` (Codex) and `Collector` (Antigravity) implement identical background loop logic: ticker + manual trigger channel, mutex-guarded `Latest()` snapshot, `PollInfo` tracking (exact same fields: `IntervalSeconds`, `LastAttemptAt`, `LastSuccessAt`, `LastError`), and staleness detection (`now > observedAt + 2*interval || lastError != ""`).
   * **Proposed Design**: A generic `Poller[T any]` interface:
     ```go
     type Poller[T any] struct { ... }

     func New[T any](interval time.Duration, fetch func(context.Context) (T, error)) *Poller[T]
     func (p *Poller[T]) Start(ctx context.Context) error
     func (p *Poller[T]) Latest() (T, bool, PollInfo)
     func (p *Poller[T]) Stale(observedAt time.Time, info PollInfo) bool
     func (p *Poller[T]) TriggerPoll()
     func (p *Poller[T]) Close() error
     ```
   * **Benefit**: Replaces ~200 LOC across both proxies with a single well-tested generic component.

2. **Private File Durability (`proxylib/privatefile` or `proxylib/storage`)**
   * **Problem**: Both proxies write config files using identical security constraints and atomic staging steps.
   * **Extracted Utilities**:
     * `InspectFile(path string) error`: Validates non-symlink regular file existence.
     * `EnsurePrivateDir(path string) error`: Creates directory with `0700` permissions.
     * `AtomicWrite(path string, data []byte) error`: Temp file in same directory + `chmod 0600` + `Sync()` + `Close()` + `Rename()` + parent directory `fsync`.
     * `CommitError`: Distinguishes successful rename from directory sync warning.
     * `ReadStrictJSON(path string) ([]byte, error)`: Validates owner permissions, non-empty JSON object, and invokes duplicate key rejection.
     * `RejectDuplicateFields(data []byte) error`: Validates single-object top-level casing.
   * **Benefit**: Unifies `codex-proxy/internal/storage` and `antigravity-proxy/internal/atomicfile`.

3. **Config Patching Pipeline (`proxylib/configfile`)**
   * **Problem**: `UpdateAccounts` and `UpdateAdminKey` in both repositories repeat the same 7-step dance:
     1. Read raw JSON bytes.
     2. Unmarshal into `map[string]json.RawMessage`.
     3. Apply typed mutation callback.
     4. Strip legacy flat credential keys case-insensitively.
     5. Marshal indented JSON.
     6. Run `validateAsLoaded` on the candidate.
     7. Commit atomically via atomic write.
   * **Proposed Design**:
     ```go
     func Patch(path string, edit func(fields map[string]json.RawMessage) error, validate func([]byte) error) error
     func StripKeys(fields map[string]json.RawMessage, keysToStrip ...string)
     ```
   * **Benefit**: Eliminates brittle JSON map manipulation duplication in both proxies.

4. **HTTP Helpers (`proxylib/httpx`)**
   * `WriteJSON(w, status, data)`: Clean JSON writing without HTML escaping URLs.
   * `WriteError(w, status, code, message)`: Standard error response envelope.
   * `ReadBounded(reader, limit)`: Bounded body consumption protecting against memory exhaustion.
   * `BearerMatches(r, expectedKey)`: Constant-time bearer token extraction and comparison.
   * `RejectUnreadBody(w, r)`: Rejects unexpected request bodies cleanly.
   * **Benefit**: Deduplicates helper functions scattered across `httputil`, `httpjson`, and `proxy/json.go`.

5. **Resource Lifecycle (`proxylib/lifecycle`)**
   * `Closer`: Idempotent, thread-safe, single-execution closure with error memoization (`sync.Once`).
   * **Benefit**: Avoids re-implementing `closeOnce` + `closeErr` across background services.

---

### Tier 2: Medium Risk, High Architectural Value

6. **Account & Key Model Abstraction (`proxylib/accounts`)**
   * **Observation**: `AccountKey` is 100% identical in both repositories (`ID`, `Label`, `Hash`, `Last4`, `CreatedAt`).
   * `ValidateAccounts`:
     * Checks account ID slug format via `proxylib/accountid`.
     * Checks account ID uniqueness.
     * Checks upstream account identity uniqueness (1:1 mapping between proxy account and upstream account).
     * Checks key ID uniqueness per account.
     * Checks key hash format (`sha256:`) and global uniqueness across all accounts.
   * **Proposed Design**:
     ```go
     type Key struct {
         ID        string    `json:"id"`
         Label     string    `json:"label,omitempty"`
         Hash      string    `json:"hash"`
         Last4     string    `json:"last4"`
         CreatedAt time.Time `json:"createdAt"`
     }

     type BaseAccount struct {
         ID      string `json:"id"`
         Label   string `json:"label,omitempty"`
         Enabled bool   `json:"enabled"`
         Keys    []Key  `json:"keys,omitempty"`
     }

     type Account interface {
         Base() *BaseAccount
         UpstreamIdentity() string
     }

     func Validate[A Account](accounts []A, providerDisplayName string) error
     ```

7. **Generic CLI Subcommands (`proxylib/clicommand`)**
   * `accounts list|enable|disable|remove` and `keys list|create|revoke` and `admin-key set|hash` are virtually character-for-character identical in `cmd/codex-proxy/commands.go` and `cmd/antigravity-proxy/commands.go`.
   * By providing a simple `AccountStore` interface:
     ```go
     type AccountStore[A Account] interface {
         ListAccounts() ([]A, error)
         UpdateAccounts(func(*[]A) error) error
         SetAdminKeyHash(hash string) error
     }
     ```
   * The CLI implementation in both proxies drops from ~289 LOC to ~30 LOC of command registration.

8. **Admin Account & Key HTTP Handlers (`proxylib/adminapi`)**
   * Handlers for `GET /admin/accounts`, `GET /admin/accounts/{id}`, `PATCH /admin/accounts/{id}`, `DELETE /admin/accounts/{id}`, `GET /admin/accounts/{id}/keys`, `POST /admin/accounts/{id}/keys`, and `DELETE /admin/accounts/{id}/keys/{keyId}` share identical logic, error responses, and validation steps.
   * Only the login session endpoints and upstream quota format remain proxy-specific.

---

### Tier 3: Higher Coupling / Lower Return (Leave for Later)

9. **OAuth Session Management State Machine**
   * Shared structs (`SessionOptions`, `SessionInfo`, `SessionStatus`, `IssuedKey`, `AccountConfigResult`) can be defined in `proxylib/oauthsession`.
   * However, the actual exchange flow differs significantly:
     * Codex uses PKCE code challenges, temporary localhost callback listeners, and state verification.
     * Antigravity uses Google OAuth consent URLs, user info fetching (`oauth2/v2/userinfo`), and custom expiry timers.
   * Recommendation: Share only the data contracts; keep execution engines local.

---

### What Should Explicitly Remain Local to Each Proxy

* **Request Translation & Protocol Adaptation**:
  * Codex converts Responses API payloads, instructions, and SSE delta streams for OpenAI backend.
  * Antigravity implements Gemini / Claude / Code Assist API translation and discovery.
* **Upstream Quota Wire Models**:
  * Codex models ChatGPT sliding rate limit windows (`5h`, `weekly`, `credits`).
  * Antigravity models Google Cloud model families and bucket allowances.
* **Routing & Dispatch**:
  * Each proxy has unique HTTP routes, upstream endpoints, headers, and authentication cookies.

---

## 3. Implementation Status

1. **Phase A (Tier 1 Extraction -> `proxylib v0.2.8`) - COMPLETE**:
   * Added `poller`, `privatefile`, `configfile`, `httpx`, and `lifecycle` packages to `proxylib`.
   * Unit tests, race detection, cross-compilation verified.
   * Migrated both `codex-proxy` and `antigravity-proxy`.

2. **Phase B (Tier 2 Extraction -> `proxylib v0.3.1`) - COMPLETE**:
   * Added `accounts` (Key model, BaseAccount, generic Validate).
   * Added `clicommand` (CLI accounts, keys, and admin-key command runners).
   * Added `adminapi` (Account and Key CRUD mutation helpers).
   * Migrated CLI commands and admin endpoints in both proxies.
   * Net reduction: >530 lines of boilerplate removed across both proxies.

3. **Phase C (Tier 3 - Optional / Deferred)**:
   * OAuth Session Management State Machine contracts.
