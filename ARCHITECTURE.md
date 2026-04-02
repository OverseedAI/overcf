# overcf Architecture Document

A CLI wrapper around the Cloudflare Go SDK, designed for both human and AI agentic use, with a focus on DNS management.

## Design Principles

1. **Dual-mode output**: Human-readable tables by default, `--json` for agents
2. **Predictable behavior**: Semantic exit codes, structured errors, consistent patterns
3. **Ergonomic input**: Flags for humans, JSON/stdin for programmatic use
4. **Safe by default**: Confirm destructive operations, `--yes` to bypass
5. **Smart resolution**: Accept domain names or zone IDs transparently

---

## Project Structure

```
overcf/
├── cmd/
│   └── overcf/
│       └── main.go                 # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go                 # Root command, global flags, config
│   │   ├── auth.go                 # auth login, whoami
│   │   ├── zone.go                 # zone list, get
│   │   ├── dns.go                  # dns list, get, create, update, delete
│   │   ├── dns_import.go           # dns import
│   │   └── dns_export.go           # dns export
│   ├── client/
│   │   └── client.go               # Cloudflare SDK wrapper, singleton
│   ├── config/
│   │   ├── config.go               # Configuration loading
│   │   └── env.go                  # Environment variable handling
│   ├── output/
│   │   ├── formatter.go            # Formatter interface
│   │   ├── table.go                # Table output (human)
│   │   ├── json.go                 # JSON output (agents)
│   │   ├── csv.go                  # CSV output
│   │   └── response.go             # Unified response types
│   ├── resolver/
│   │   └── zone.go                 # Domain name → Zone ID resolution
│   ├── confirm/
│   │   └── confirm.go              # Interactive confirmation prompts
│   ├── exitcode/
│   │   └── codes.go                # Exit code constants
│   └── types/
│       ├── dns.go                  # DNS record types
│       └── zone.go                 # Zone types
├── go.mod
├── go.sum
└── Makefile
```

---

## Implementation Files

### 1. cmd/overcf/main.go

Entry point. Minimal - just calls the CLI.

```go
package main

import (
    "os"
    "github.com/OverseedAI/overcf/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### 2. internal/exitcode/codes.go

Semantic exit codes for agent parsing.

```go
package exitcode

const (
    Success         = 0
    GeneralError    = 1
    AuthError       = 2
    NotFound        = 3
    ValidationError = 4
    RateLimited     = 5
    Conflict        = 6
    NetworkError    = 7
)
```

### 3. internal/config/config.go

Configuration from environment variables.

```go
package config

type Config struct {
    APIToken   string
    BaseURL    string // Optional, for testing
    Debug      bool
}

func Load() (*Config, error) {
    token := os.Getenv("CLOUDFLARE_API_TOKEN")
    if token == "" {
        return nil, ErrNoToken
    }
    return &Config{
        APIToken: token,
        BaseURL:  os.Getenv("CLOUDFLARE_API_URL"),
        Debug:    os.Getenv("OVERCF_DEBUG") == "1",
    }, nil
}
```

### 4. internal/client/client.go

Cloudflare SDK client wrapper.

```go
package client

import (
    "sync"
    "github.com/cloudflare/cloudflare-go/v4"
    "github.com/cloudflare/cloudflare-go/v4/option"
)

var (
    instance *cloudflare.Client
    once     sync.Once
)

func Get(cfg *config.Config) *cloudflare.Client {
    once.Do(func() {
        opts := []option.RequestOption{
            option.WithAPIToken(cfg.APIToken),
        }
        if cfg.BaseURL != "" {
            opts = append(opts, option.WithBaseURL(cfg.BaseURL))
        }
        instance = cloudflare.NewClient(opts...)
    })
    return instance
}
```

### 5. internal/resolver/zone.go

Smart zone resolution (domain name or ID).

```go
package resolver

import (
    "context"
    "regexp"
)

var zoneIDRegex = regexp.MustCompile(`^[a-f0-9]{32}$`)

type ZoneResolver struct {
    client *cloudflare.Client
    cache  map[string]string
}

func (r *ZoneResolver) Resolve(ctx context.Context, input string) (string, error) {
    // Direct zone ID
    if zoneIDRegex.MatchString(input) {
        return input, nil
    }

    // Check cache
    if id, ok := r.cache[input]; ok {
        return id, nil
    }

    // Look up by domain name
    zones, err := r.client.Zones.List(ctx, zones.ZoneListParams{
        Name: cloudflare.F(input),
    })
    if err != nil {
        return "", fmt.Errorf("failed to look up zone: %w", err)
    }

    if len(zones.Result) == 0 {
        return "", fmt.Errorf("zone not found: %s", input)
    }

    id := zones.Result[0].ID
    r.cache[input] = id
    return id, nil
}
```

### 6. internal/output/formatter.go

Output formatting interface.

```go
package output

import "io"

type Formatter interface {
    // Format outputs the data
    Format(w io.Writer, data any) error

    // FormatError outputs an error
    FormatError(w io.Writer, err error, code string) error
}

type Config struct {
    Format  string // "table", "json", "csv"
    Quiet   bool
    NoColor bool
}

func NewFormatter(cfg Config) Formatter {
    switch cfg.Format {
    case "json":
        return &JSONFormatter{}
    case "csv":
        return &CSVFormatter{}
    default:
        return &TableFormatter{NoColor: cfg.NoColor}
    }
}
```

### 7. internal/output/response.go

Unified response structure for JSON output.

```go
package output

type Response[T any] struct {
    Success bool       `json:"success"`
    Data    T          `json:"data,omitempty"`
    Error   *ErrorInfo `json:"error,omitempty"`
}

type ErrorInfo struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
}

type ListResponse[T any] struct {
    Success bool   `json:"success"`
    Data    []T    `json:"data"`
    Count   int    `json:"count"`
}
```

### 8. internal/confirm/confirm.go

Confirmation prompts for destructive operations.

```go
package confirm

import (
    "bufio"
    "fmt"
    "os"
    "strings"

    "golang.org/x/term"
)

func Destructive(action string, target string, skipConfirm bool) bool {
    if skipConfirm {
        return true
    }

    if !term.IsTerminal(int(os.Stdin.Fd())) {
        // Non-interactive: fail safe, require --yes
        fmt.Fprintf(os.Stderr, "Error: destructive operation requires --yes flag in non-interactive mode\n")
        return false
    }

    fmt.Printf("This will %s: %s\n", action, target)
    fmt.Print("Proceed? [y/N]: ")

    reader := bufio.NewReader(os.Stdin)
    response, _ := reader.ReadString('\n')
    response = strings.TrimSpace(strings.ToLower(response))

    return response == "y" || response == "yes"
}
```

---

## CLI Commands Specification

### Global Flags (all commands)

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--json` | | bool | false | Output in JSON format |
| `--quiet` | `-q` | bool | false | Minimal output |
| `--yes` | `-y` | bool | false | Skip confirmation prompts |
| `--no-color` | | bool | false | Disable colored output |
| `--format` | `-f` | string | "table" | Output format: table, json, csv |

### auth login

Interactive token setup. Saves to environment suggestion or config file.

```
Usage: overcf auth login

Flags:
  --token <token>    Provide token directly (non-interactive)
```

**Behavior:**
- Prompts for API token if not provided
- Validates token by calling whoami endpoint
- Displays instructions for setting CLOUDFLARE_API_TOKEN

### auth whoami

Show current authentication context.

```
Usage: overcf auth whoami
```

**Output (table):**
```
Account:  Example Inc
Email:    user@example.com
Token:    ****xxxx (scoped)
```

**Output (json):**
```json
{
  "success": true,
  "data": {
    "account_id": "abc123",
    "account_name": "Example Inc",
    "email": "user@example.com",
    "token_type": "scoped"
  }
}
```

### zone list

List all zones in the account.

```
Usage: overcf zone list

Flags:
  --status <status>    Filter by status: active, pending, moved
  --name <name>        Filter by name (partial match)
```

**Output (table):**
```
ID                                NAME           STATUS   PLAN
023e105f4ecef8ad9ca31a8372d0c353  example.com    active   pro
def456...                         example.org    active   free
```

### zone get

Get details for a specific zone.

```
Usage: overcf zone get <zone>

Arguments:
  zone    Zone ID or domain name
```

### dns list

List DNS records for a zone.

```
Usage: overcf dns list <zone>

Arguments:
  zone    Zone ID or domain name

Flags:
  --type <type>        Filter by record type (A, AAAA, CNAME, etc.)
  --name <name>        Filter by record name
  --content <content>  Filter by content
```

**Output (table):**
```
ID        TYPE   NAME                 CONTENT          TTL   PROXIED
abc123    A      www.example.com      192.0.2.1        3600  yes
def456    CNAME  blog.example.com     example.com      auto  yes
ghi789    MX     example.com          mail.example.com 3600  no
```

### dns get

Get a specific DNS record.

```
Usage: overcf dns get <zone> <record-id>

Arguments:
  zone        Zone ID or domain name
  record-id   The DNS record ID
```

### dns create

Create a new DNS record.

```
Usage: overcf dns create <zone> [flags]

Arguments:
  zone    Zone ID or domain name

Flags:
  --type <type>          Record type (required): A, AAAA, CNAME, MX, TXT, etc.
  --name <name>          Record name (required): @ for root, or subdomain
  --content <content>    Record content (required): IP, hostname, or text
  --ttl <seconds>        TTL in seconds (default: auto/1)
  --proxied              Enable Cloudflare proxy (default: false)
  --priority <int>       Priority for MX/SRV records
  --from-json <json>     Create from JSON string
  --stdin                Read record from stdin as JSON
```

**Examples:**
```bash
# Create A record
overcf dns create example.com --type A --name www --content 192.0.2.1 --proxied

# Create MX record
overcf dns create example.com --type MX --name @ --content mail.example.com --priority 10

# Create from JSON
overcf dns create example.com --from-json '{"type":"A","name":"api","content":"10.0.0.1"}'

# Create from stdin (for agents)
echo '{"type":"TXT","name":"_verify","content":"abc123"}' | overcf dns create example.com --stdin
```

### dns update

Update an existing DNS record.

```
Usage: overcf dns update <zone> <record-id> [flags]

Arguments:
  zone        Zone ID or domain name
  record-id   The DNS record ID

Flags:
  --type <type>          Record type
  --name <name>          Record name
  --content <content>    Record content
  --ttl <seconds>        TTL in seconds
  --proxied              Enable Cloudflare proxy
  --no-proxied           Disable Cloudflare proxy
  --priority <int>       Priority for MX/SRV records
  --from-json <json>     Update from JSON string
  --stdin                Read updates from stdin as JSON
```

### dns delete

Delete a DNS record.

```
Usage: overcf dns delete <zone> <record-id>

Arguments:
  zone        Zone ID or domain name
  record-id   The DNS record ID
```

**Behavior:**
- Shows record details before deletion
- Prompts for confirmation (unless `--yes`)

### dns export

Export all DNS records from a zone.

```
Usage: overcf dns export <zone>

Arguments:
  zone    Zone ID or domain name

Flags:
  --format <format>    Output format: yaml, json (default: yaml)
```

**Output (yaml):**
```yaml
zone: example.com
zone_id: 023e105f4ecef8ad9ca31a8372d0c353
exported_at: "2026-01-20T17:30:00Z"
records:
  - id: abc123
    type: A
    name: www.example.com
    content: 192.0.2.1
    ttl: 3600
    proxied: true
  - id: def456
    type: MX
    name: example.com
    content: mail.example.com
    ttl: 3600
    proxied: false
    priority: 10
```

### dns import

Import DNS records from a file.

```
Usage: overcf dns import <zone> <file>

Arguments:
  zone    Zone ID or domain name
  file    Path to import file (yaml or json)

Flags:
  --dry-run    Show what would be created without applying
```

**Behavior:**
1. Parse the import file
2. Show summary of records to create
3. Prompt for confirmation (unless `--yes`)
4. Create records one by one
5. Report results

---

## DNS Record Types

Supported record types and their required/optional fields:

| Type | Required Fields | Optional Fields |
|------|-----------------|-----------------|
| A | name, content (IPv4) | ttl, proxied |
| AAAA | name, content (IPv6) | ttl, proxied |
| CNAME | name, content (hostname) | ttl, proxied |
| MX | name, content, priority | ttl |
| TXT | name, content | ttl |
| NS | name, content | ttl |
| SRV | name, content, priority, weight, port | ttl |
| CAA | name, content, flags, tag | ttl |
| PTR | name, content | ttl |

### Type-specific validation

```go
package types

type RecordType string

const (
    RecordTypeA     RecordType = "A"
    RecordTypeAAAA  RecordType = "AAAA"
    RecordTypeCNAME RecordType = "CNAME"
    RecordTypeMX    RecordType = "MX"
    RecordTypeTXT   RecordType = "TXT"
    RecordTypeNS    RecordType = "NS"
    RecordTypeSRV   RecordType = "SRV"
    RecordTypeCAA   RecordType = "CAA"
    RecordTypePTR   RecordType = "PTR"
)

func (t RecordType) RequiresPriority() bool {
    return t == RecordTypeMX || t == RecordTypeSRV
}

func (t RecordType) SupportsProxy() bool {
    return t == RecordTypeA || t == RecordTypeAAAA || t == RecordTypeCNAME
}

func (t RecordType) ValidateContent(content string) error {
    switch t {
    case RecordTypeA:
        if net.ParseIP(content) == nil || strings.Contains(content, ":") {
            return fmt.Errorf("invalid IPv4 address: %s", content)
        }
    case RecordTypeAAAA:
        if net.ParseIP(content) == nil || !strings.Contains(content, ":") {
            return fmt.Errorf("invalid IPv6 address: %s", content)
        }
    // ... other validations
    }
    return nil
}
```

---

## Error Codes

Standard error codes for JSON output:

| Code | Exit Code | Description |
|------|-----------|-------------|
| `AUTH_REQUIRED` | 2 | No API token configured |
| `AUTH_INVALID` | 2 | Invalid or expired token |
| `ZONE_NOT_FOUND` | 3 | Zone does not exist |
| `RECORD_NOT_FOUND` | 3 | DNS record does not exist |
| `VALIDATION_ERROR` | 4 | Invalid input parameters |
| `RATE_LIMITED` | 5 | API rate limit exceeded |
| `RECORD_EXISTS` | 6 | Record already exists |
| `NETWORK_ERROR` | 7 | Network connectivity issue |

---

## Implementation Order

1. **Phase 1: Foundation**
   - [ ] go.mod, project structure
   - [ ] exitcode package
   - [ ] config package
   - [ ] client package
   - [ ] output package (json, table)

2. **Phase 2: Core Commands**
   - [ ] root command with global flags
   - [ ] auth whoami
   - [ ] zone list
   - [ ] zone get

3. **Phase 3: DNS CRUD**
   - [ ] dns list
   - [ ] dns get
   - [ ] dns create
   - [ ] dns update
   - [ ] dns delete

4. **Phase 4: Bulk Operations**
   - [ ] dns export
   - [ ] dns import

5. **Phase 5: Polish**
   - [ ] Comprehensive error handling
   - [ ] Help text and examples
   - [ ] Makefile for builds
   - [ ] Shell completions
