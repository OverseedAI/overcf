# overcf

Cloudflare CLI for managing zones and DNS records.

`overcf` is built for both humans and automation: table output by default, JSON/CSV output for scripts, semantic exit codes, domain-name-to-zone-ID resolution, and confirmation prompts for destructive operations.

## Install

With Homebrew:

```sh
brew tap OverseedAI/tap
brew install overcf
```

From source:

```sh
go install github.com/OverseedAI/overcf/cmd/overcf@latest
```

Or build locally:

```sh
make build
./bin/overcf --version
```

## Authentication

Set a Cloudflare API token in the environment:

```sh
export CLOUDFLARE_API_TOKEN=your_token_here
```

For DNS management, the token needs:

- `Zone:Zone:Read`
- `Zone:DNS:Edit`

You can print the auth setup instructions with:

```sh
overcf auth login
```

## Usage

List zones:

```sh
overcf zone list
overcf zone list --json
```

Get zone details by domain name or zone ID:

```sh
overcf zone get example.com
overcf zone get 023e105f4ecef8ad9ca31a8372d0c353 --json
```

Manage DNS records:

```sh
overcf dns list example.com
overcf dns create example.com --type A --name www --content 192.0.2.1
overcf dns update example.com <record-id> --content 192.0.2.2
overcf dns delete example.com <record-id> --yes
```

Create records from JSON:

```sh
overcf dns create example.com --from-json '{"type":"TXT","name":"_verify","content":"token=abc123"}'
echo '{"type":"A","name":"api","content":"192.0.2.10"}' | overcf dns create example.com --stdin
```

## Import And Export

Export records as JSON or CSV:

```sh
overcf dns export example.com --json > records.json
overcf dns export example.com --format csv > records.csv
```

Import records from JSON or CSV:

```sh
overcf dns import example.com --file records.json
overcf dns import example.com --file records.csv --input-format csv
cat records.json | overcf dns import example.com --stdin
```

Use `--replace` to delete existing records that are not present in the import payload. Because this is destructive, pass `--yes` in non-interactive scripts:

```sh
overcf dns import example.com --file records.json --replace --yes
```

JSON imports accept either a raw array of DNS records or an object containing `data` or `records`. CSV imports accept:

```text
id,type,name,content,ttl,proxied,priority,port,weight,target,flags,tag,value
```

Leave `id` empty to create records. Include `id` to update an existing record.

## Output Formats

Use `--json` or `--format json` for structured output:

```sh
overcf dns list example.com --json
```

Use `--format csv` for list/export commands:

```sh
overcf dns export example.com --format csv
```

## Exit Codes

| Code | Name | Meaning |
| ---: | --- | --- |
| 0 | `SUCCESS` | Command completed successfully |
| 1 | `GENERAL_ERROR` | Unspecified error |
| 2 | `AUTH_ERROR` | Missing or invalid authentication |
| 3 | `NOT_FOUND` | Requested resource does not exist |
| 4 | `VALIDATION_ERROR` | Invalid input |
| 5 | `RATE_LIMITED` | API rate limit exceeded |
| 6 | `CONFLICT` | Resource conflict |
| 7 | `NETWORK_ERROR` | Network connectivity issue |
| 8 | `CANCELLED` | Destructive operation was refused or not confirmed |

## Development

Run tests with a local Go toolchain:

```sh
make test
```

On machines without Go installed, run the Docker-backed suite:

```sh
make test-docker
```

The integration tests build the real `overcf` binary and run it against an in-memory fake Cloudflare API. No real Cloudflare token is required for the test suite.

## License

MIT. See [LICENSE](LICENSE).
