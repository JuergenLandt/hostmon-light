# hostmon-light

**Docker Image: 6.71 MB**

[Deutsch](README_de.md) | [English](README_en.md)

A lightweight host-monitoring agent for Linux: one container, a straightforward configuration, and an HTTP API for essential resource metrics.

hostmon-light is designed to be quick to deploy and easy to move between hosts, without a large monitoring stack or excessive configuration. The agent supplies metrics. External tools or scripts can handle evaluation, storage, and notifications.

The size refers to the local version 0.1.0 image for linux/amd64, not the compressed download size.

## Features

- One container with no additional services.
- Host CPU utilization, RAM usage, and swap usage.
- Space and inode usage for configured filesystems.
- JSON API with bearer-token authentication.
- Token loaded from a separate file rather than `.env`.
- Configurable sampling interval and stale-sample detection.
- Built-in container healthcheck.
- Non-root execution; UID:GID `65532:65532` by default.
- Statically built Go application in a small `scratch` image.
- No Docker socket or Docker API required.
- Read-only host mounts and configurable container resource limits.
- Commented configuration and Docker log rotation.

## Deliberate scope

The agent monitors host resources, not individual Docker containers. A database, metric history, dashboards, resource thresholds, notifications, and a general plugin platform are outside this project's scope.

## Requirements

- Linux host with Docker Engine and Docker Compose.
- Tested platform: linux/amd64. The build configuration explicitly targets it.
- An existing source directory on each filesystem to be monitored.
- Bash and OpenSSL for the token-generation example below.
- Optionally, `curl` supporting `--fail-with-body` and `jq` for querying the API.
- A suitable image from your own registry or a local build.

The examples assume that you have already cloned or downloaded the repository and run commands from its root directory.

## Getting started

### 1. Prepare the configuration

Copy only during initial installation; do not overwrite an existing `.env`:

```bash
cp .env.example.en .env
```

All parameters are documented in the example configuration. In particular, check:

| Parameter | Purpose |
|---|---|
| `IMAGE_REPOSITORY` | Image repository without a tag |
| `IMAGE_TAG` | Desired version |
| `HOST_LAN_IP` | Local LAN address of the monitored host |
| `MONITOR_PORT` | API port; default `9109` |
| `MONITOR_HOSTNAME` | Host identifier in API responses |
| `MONITOR_DISK_ROOT_SOURCE` | Existing source directory on the filesystem to measure |
| `MONITOR_TOKEN_HOST_FILE` | Token file; default `./secrets/api_token` |

Use one active `.env` per host. No separate `.env.build` is needed.

### 2. Obtain or build an image

Example configuration for your own repository:

```dotenv
IMAGE_REPOSITORY=registry.example.com/home/hostmon-light
IMAGE_TAG=0.1.0
```

This address is a placeholder, not a public image download location. Replace it with your repository. If needed, authenticate using `docker login`; do not put registry credentials in `.env`.

For a local build, check `GO_VERSION` in `.env`, then run:

```bash
docker compose -f compose.build.yaml config --quiet
docker compose -f compose.build.yaml build hostmon-light
```

You can use the local image on the same host without pushing it. To deploy it on another host, optionally push it to your registry:

```bash
docker compose -f compose.build.yaml push hostmon-light
```

### 3. Prepare the filesystem source

For example, create an empty directory in the project:

```bash
mkdir -p disk-root
findmnt -T ./disk-root
realpath ./disk-root
```

Set `MONITOR_DISK_ROOT_SOURCE` to the absolute path shown. To measure the root filesystem, this directory must reside on the filesystem mounted at `/`.

The agent measures the underlying filesystem, not the directory size. An empty directory avoids mounting the entire host directory tree.

Additional filesystems require their own bind mounts in `compose.yaml` and matching entries in `MONITOR_FILESYSTEMS`. Adding a configuration entry alone is not sufficient. An API path such as `/host/disks/root` is a path inside the container.

### 4. Generate the token file

Run only during initial installation. Do not replace an existing token file:

```bash
mkdir -p secrets

(
  set -eu
  set -o noclobber
  umask 077

  openssl rand -hex 32 > secrets/api_token

  sudo chgrp 65532 secrets/api_token
  chmod 640 secrets/api_token
)
```

OpenSSL generates 32 random bytes, represented as 64 hexadecimal characters. The token is not printed to the terminal. The overwrite protection prevents accidentally replacing an existing file.

The invoking user remains the file owner; group `65532` receives read access. Adjust the group number if you change the container GID. The container must be able to read the file.

The file contains only the token, without a `Bearer ` prefix or quotation marks. Put only its path in `.env`:

```dotenv
MONITOR_TOKEN_HOST_FILE=./secrets/api_token
```

Do not commit the local `.env` or `secrets/`; ensure that `.gitignore` excludes them.

### 5. Start and verify

First validate the runtime configuration:

```bash
docker compose -f compose.yaml config --quiet
```

When using a registry, download the image:

```bash
docker compose -f compose.yaml pull hostmon-light
```

Skip the pull if you built the image locally on the same host. Then run:

```bash
docker compose -f compose.yaml up -d --no-build --pull never hostmon-light
docker compose -f compose.yaml ps
docker compose -f compose.yaml logs --tail 50 hostmon-light
```

CPU collection initially requires two measurement points. After warm-up, the container should report `healthy`. Account for longer sampling intervals when setting the healthcheck start period.

## Query the API remotely

Endpoint: `http://<HOST-IP>:9109/api/v1/metrics`.

Run this in Bash on an authorized remote machine. Replace the example address:

```bash
(
  set -o pipefail

  HOSTMON_URL='http://192.0.2.10:9109'

  read -rsp 'API token: ' HOSTMON_TOKEN
  printf '\n'

  curl --fail-with-body \
    --silent \
    --show-error \
    --connect-timeout 3 \
    --max-time 10 \
    --header "Authorization: Bearer ${HOSTMON_TOKEN}" \
    "${HOSTMON_URL}/api/v1/metrics" |
    jq .
)
```

The token is entered without echo and is not saved as a literal token value in shell history. `jq` formats the response; this command does not evaluate resource thresholds.

### Example response

Successful response from version 0.1.0 with a neutral host identifier. The timestamp and metric values are examples:

```json
{
  "schema_version": 1,
  "host": "example-host",
  "sampled_at": "2026-11-16T15:38:13.822603879Z",
  "ready": true,
  "cpu_usage_percent": 3.327451474817052,
  "cpu_sample_seconds": 9.999771396,
  "memory": {
    "total_bytes": 16542408704,
    "available_bytes": 6487289856,
    "used_percent": 60.783885998226154
  },
  "swap": {
    "total_bytes": 0,
    "used_bytes": 0,
    "used_percent": 0
  },
  "filesystems": {
    "root": {
      "path": "/host/disks/root",
      "total_bytes": 503959781376,
      "available_bytes": 365140598784,
      "used_bytes": 113201098752,
      "used_percent": 23.66532111566136,
      "inodes_total": 31252480,
      "inodes_available": 29031968,
      "inodes_used_percent": 7.105074541284404
    }
  },
  "errors": {}
}
```

### Understanding the metrics

- `schema_version`: response schema version.
- `sampled_at`: timestamp of the metric snapshot; `Z` denotes UTC.
- `cpu_usage_percent`: CPU utilization over the interval reported as `cpu_sample_seconds`.
- Byte fields contain bytes; percentage fields use a scale from 0 to 100.
- RAM usage: `100 × (1 − available_bytes / total_bytes)`.
- Filesystem usage: `used_percent` relates to used and available space; reserved space can explain why `used_bytes + available_bytes` does not equal `total_bytes`.
- `inodes_used_percent`: inode usage, separate from storage-space usage.
- Swap with `total_bytes: 0` indicates that no swap is available in this example.
- `ready` and `errors` describe collection readiness and collection errors, not resource alarms.

A healthy agent may report high resource usage. `healthy` means collection is ready and current, not that CPU, RAM, or filesystem usage is below an alert threshold.

## Networking and security

The runtime configuration uses host networking and binds the API to `HOST_LAN_IP:MONITOR_PORT`. No Docker port publishing is involved. Binding to an address does not replace a firewall.

Allow access to the API port in the host firewall only from the required monitoring hosts or administration networks; block other sources. Review the rules when changing the host address, port, or monitoring host. Exact firewall commands depend on the existing ruleset.

The API uses HTTP without TLS. Bearer authentication does not encrypt the token or metrics. Use protected transport, such as a VPN or TLS reverse proxy, across untrusted networks. Do not expose the service directly to the internet without protection.

Keep the token confidential. When rotating it, update both the agent and API clients to use the new token, and restart the service after changing it.

## Monitoring tools and scripts

Use the API with scripts or tools that support HTTP requests with Authorization headers and suitable JSON processing.

I use hostmon-light as a metric source in conjunction with Uptime Kuma. Configuration and evaluation within Uptime Kuma or other tools are outside this repository's scope. This includes monitors, thresholds, polling intervals, retries, storage, and notifications.

## Updates

Set the desired release tag in `.env`. When using a registry, run:

```bash
docker compose -f compose.yaml config --quiet
docker compose -f compose.yaml pull hostmon-light
docker compose -f compose.yaml up -d --no-build --pull never hostmon-light
docker compose -f compose.yaml ps
```

For local builds, build the new image using `compose.build.yaml` first and skip the pull. Do not overwrite published release tags with changed builds. Do not replace your active `.env` with a new example; incorporate new parameters selectively as needed.

## Troubleshooting

| Symptom | Check |
|---|---|
| Cannot connect | Host address, port, container status, routing, and firewall |
| HTTP `401` | Token and Authorization header |
| HTTP `503` | Warm-up, sample age, and collection errors |
| Container fails to start | Logs, configuration, existing mount sources, and token read permissions |
| Wrong filesystem measured | Check the mount source using `findmnt -T` |

## License

MIT License; see [LICENSE](LICENSE). Notices for third-party dependencies remain applicable.

Copyright (c) 2026 [Jürgen Landt](https://jlandt.de)
