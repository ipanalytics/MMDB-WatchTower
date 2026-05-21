# MMDB Watchtower

Safe automatic updates for MaxMind DB files.

MMDB Watchtower is a lightweight agent for production systems that depend on
local `.mmdb` databases. It downloads new database releases, verifies integrity,
runs smoke checks, swaps files atomically, reloads your service, and rolls back
when an update is bad.

License: MIT


## Why

Cron + curl is not a deployment strategy.

Common failure modes include broken downloads, HTML error pages saved as MMDB
files, unexpected schema changes, empty databases, missing fields, services
reading files while they are being overwritten, slow rollback, unknown deployed
versions, and no update metrics.

MMDB Watchtower treats database refreshes like deployments:

```text
download -> verify -> inspect -> smoke test -> atomic swap -> reload hook -> metrics -> rollback if needed
```

## Features

- scheduled MMDB downloads
- gzip support
- sha256 verification
- Sigstore/cosign signature verification
- HTTP ETag/If-None-Match
- mTLS/private registry support
- multi-channel updates: stable/canary/latest
- MMDB metadata validation
- required field checks
- smoke tests for known IPs
- atomic file replacement
- versioned local backups
- automatic rollback
- reload hooks
- Prometheus metrics
- Kubernetes readiness endpoint
- JSON status output
- systemd and Docker examples

## Install

From a local checkout:

```bash
go build -o ./bin/mmdbwatch ./cmd/mmdbwatch
./bin/mmdbwatch --help
```

```bash
go install ./cmd/mmdbwatch
```

## CLI

```bash
mmdbwatch init
mmdbwatch update --config /etc/mmdbwatch.yaml
mmdbwatch update --channel canary --config /etc/mmdbwatch.yaml
mmdbwatch status --config /etc/mmdbwatch.yaml
mmdbwatch rollback vpn --config /etc/mmdbwatch.yaml
mmdbwatch verify /var/lib/mmdb/geo.mmdb
mmdbwatch run --config /etc/mmdbwatch.yaml
```

## Configuration

```yaml
databases:
  - name: geo
    channel: stable
    channels:
      stable:
        url: https://example.com/mmdb/geo/stable.mmdb.gz
        sha256_url: https://example.com/mmdb/geo/stable.sha256
        signature_url: https://example.com/mmdb/geo/stable.mmdb.sig
        cosign_key: /etc/mmdbwatch/cosign.pub
      canary:
        url: https://example.com/mmdb/geo/canary.mmdb.gz
      latest:
        url: https://example.com/mmdb/geo/latest.mmdb.gz
    path: /var/lib/mmdb/geo.mmdb
    interval: 6h
    min_size_mb: 20
    max_size_mb: 500
    tls:
      client_cert: /etc/mmdbwatch/client.crt
      client_key: /etc/mmdbwatch/client.key
      ca_cert: /etc/mmdbwatch/ca.crt
    required_fields:
      - location.country_code
      - location.city_geoname_id
      - confidence
    smoke_tests:
      - ip: 8.8.8.8
        expect:
          location.country_code: US

reload:
  command: systemctl reload my-ip-api
  timeout: 10s

retention:
  keep_versions: 5

metrics:
  listen: 127.0.0.1:9417

readiness:
  listen: 127.0.0.1:9418
```

## Docker

```bash
docker run -d \
  --name mmdbwatch \
  -v /etc/mmdbwatch.yaml:/etc/mmdbwatch.yaml:ro \
  -v /var/lib/mmdb:/var/lib/mmdb \
  mmdb-watchtower:latest \
  run --config /etc/mmdbwatch.yaml
```

## systemd

```ini
[Unit]
Description=MMDB Watchtower
After=network-online.target

[Service]
ExecStart=/usr/local/bin/mmdbwatch run --config /etc/mmdbwatch.yaml
Restart=always
User=mmdbwatch
Group=mmdbwatch

[Install]
WantedBy=multi-user.target
```

## Kubernetes sidecar

```text
app container:
  reads /data/mmdb/vpn.mmdb

mmdbwatch container:
  updates /data/mmdb/vpn.mmdb atomically
  exposes /metrics
  exposes /readyz
  optionally calls app reload endpoint
```

## Metrics

```text
mmdbwatch_update_success_total{name="geo"} 42
mmdbwatch_update_failure_total{name="vpn",reason="smoke_failed"} 3
mmdbwatch_database_age_seconds{name="geo"} 1800
mmdbwatch_database_size_bytes{name="geo"} 104857600
mmdbwatch_current_build_epoch{name="geo"} 1779364800
mmdbwatch_rollback_total{name="vpn"} 1
```

## Status

```bash
mmdbwatch status --config /etc/mmdbwatch.yaml
```

```json
{
  "databases": [
    {
      "name": "geo",
      "path": "/var/lib/mmdb/geo.mmdb",
      "channel": "stable",
      "status": "healthy",
      "database_type": "ipanalytics-geo",
      "build_epoch": 1779364800,
      "sha256": "abc123...",
      "etag": "\"release-2026.05.21\"",
      "last_update_at": "2026-05-21T10:00:00Z",
      "last_success_at": "2026-05-21T10:00:02Z",
      "previous_versions": 5
    }
  ]
}
```

## License

MIT. See [LICENSE](LICENSE).
