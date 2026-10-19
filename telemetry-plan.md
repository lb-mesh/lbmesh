# LBMesh Telemetry Architecture Plan (Client & Server)

This document details the architectural specifications and step-by-step implementation plan for introducing telemetry to LBMesh. It covers the offline-first Client-Side Analytics subsystem and the high-throughput Server-Side Ingestion Engine (`telemetry.lbmesh.io`) backed by MySQL and an insights dashboard.

---

## Part 1: Client-Side Telemetry Plan

The client-side telemetry module is designed with an **offline-first approach** to guarantee zero execution latency for terminal commands.

### 1. Architectural Highlights
- **Privacy & Consent**: Includes an opt-out configuration flag in `~/.lbmesh/config.yaml` (`core.telemetry_enabled: true`). Respects standard shell environment variables `LBMESH_NO_TELEMETRY=1` and `DO_NOT_TRACK=1`.
- **Anonymity**: No directory paths, private volume paths, database values, environment secrets, or prompt strings are ever transmitted. The node is identified using a stable, anonymous SHA-256 hash of machine-specific hardware configurations.
- **Offline-First SQLite Queue**: To prevent blocking local terminal executions, events are written immediately to a local SQLite table (`telemetry_queue`) in `< 1ms`.
- **Background Flusher**: Payloads are queued locally and flushed to `telemetry.lbmesh.io` asynchronously in batches of up to 100 entries when long-running commands like `lbmesh dashboard` or `lbmesh catalog sync` are active.

### 2. Client-Side Sub-Tasks

#### Sub-Task C1: Telemetry Config & DB Queue Schema
- **Intent**: Prepare the local configuration defaults and set up the SQLite database analytics queue to buffer events.
- **Expected Outcomes**:
  - `telemetry_enabled: true` in default config.yaml.
  - New `telemetry_queue` table created via SQLite migration.
- **Todo List**:
  1. [ ] Update `pkg/config/config.go` with `TelemetryEnabled bool` inside `CoreConfig`.
  2. [ ] Add migration `003_telemetry` inside `pkg/db/migrations.go` defining the `telemetry_queue` table:
     ```sql
     CREATE TABLE IF NOT EXISTS telemetry_queue (
         id INTEGER PRIMARY KEY AUTOINCREMENT,
         payload TEXT NOT NULL,
         created_at DATETIME DEFAULT CURRENT_TIMESTAMP
     );
     ```
  3. [ ] Implement database write wrapper `pkg/db/telemetry.go` to insert marshaled events into the queue.

#### Sub-Task C2: Cobra post-command Hooks & Hashed ID
- **Intent**: Gather system metadata, construct anonymous hashed Node IDs, and capture command metrics automatically after CLI executions.
- **Expected Outcomes**:
  - Stable, anonymous client ID generated on first init.
  - Execution duration and command metrics recorded at post-execution.
- **Todo List**:
  1. [ ] Implement hashed client ID generator in `pkg/telemetry/identity.go` hashing system identifiers (e.g., hostnames, hardware UUIDs) using SHA-256.
  2. [ ] Create event structure models mapping the client-side telemetry payload.
  3. [ ] Integrate Cobra's `PersistentPreRun` and `PersistentPostRun` hooks inside `internal/cli/root.go` to capture:
     - Name of command executed (e.g., `up`, `down`, `init`, `ask`).
     - Target recipe names and categories if applicable.
     - Elapsed milliseconds.
     - Execution success status (error message if failed).

#### Sub-Task C3: Asynchronous Batch Flusher
- **Intent**: Safely dispatch queued analytics batches to the server during active online sessions without interfering with the CLI's responsiveness.
- **Expected Outcomes**:
  - Background worker thread flushing payloads during `lbmesh dashboard` or `catalog sync` processes.
- **Todo List**:
  1. [ ] Write background worker `pkg/telemetry/flusher.go` which runs as a goroutine with an aggressive HTTP context timeout (e.g., 2 seconds).
  2. [ ] Fetch up to 100 queued items from the SQLite database.
  3. [ ] Pack them into a single HTTP POST request to `https://telemetry.lbmesh.io/v1/batch`.
  4. [ ] If the transmission succeeds, delete flushed rows from the SQLite queue. If offline, keep the rows in SQLite for the next cycle.

---

## Part 2: Server-Side Telemetry Plan (`telemetry.lbmesh.io`)

The server-side ingestion engine must be optimized for **maximum throughput, high concurrency, and low resource overhead**. It will run as a lightweight, compiled Go microserver.

```
                      Server Ingestion Architecture
                      
   Client Payloads      Go Ingestion Server        Buffer Channel     MySQL DB
  ──────────────────►  [ /v1/batch Handler ] ───►  [chan size:10k] ──► [ Batch Insert ]
```

### 1. Ingestion Microserver Specs
- **Engine**: Go (Golang) compiled static binary utilizing native, high-performance multiplex routers (`net/http`).
- **Buffer & Worker Pool**: To absorb peak traffic spikes and protect the database, requests are immediately written to an in-memory buffered channel of size `10,000`. A pool of background workers drains this channel, batching rows into single `INSERT INTO ... VALUES` transactions against MySQL every 2 seconds.
- **Stateless scale**: The microserver remains 100% stateless, allowing you to run multiple instances behind a load balancer (like Nginx, Cloudflare, or AWS ALB) as usage grows.

### 2. Relational MySQL Schema Design
The datastore of choice is **MySQL** for robust analytical aggregation, structured querying, and horizontal scalability.

```sql
CREATE TABLE IF NOT EXISTS clients (
    client_id VARCHAR(64) PRIMARY KEY, -- Anonymous hashed machine ID
    os VARCHAR(32) NOT NULL,
    arch VARCHAR(32) NOT NULL,
    cpu_cores INT NOT NULL,
    memory_total_mb INT NOT NULL,
    first_seen DATETIME NOT NULL,
    last_seen DATETIME NOT NULL,
    INDEX idx_last_seen (last_seen)
);

CREATE TABLE IF NOT EXISTS telemetry_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    client_id VARCHAR(64) NOT NULL,
    command VARCHAR(64) NOT NULL,
    recipe_name VARCHAR(128) DEFAULT NULL,
    recipe_category VARCHAR(64) DEFAULT NULL,
    duration_ms INT NOT NULL,
    success BOOLEAN NOT NULL,
    error_class TEXT DEFAULT NULL,
    engine VARCHAR(32) NOT NULL,
    engine_version VARCHAR(32) NOT NULL,
    lbmesh_version VARCHAR(32) NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (client_id) REFERENCES clients(client_id) ON DELETE CASCADE,
    INDEX idx_created_at (created_at),
    INDEX idx_command (command),
    INDEX idx_recipe (recipe_name)
);
```

### 3. Server-Side Sub-Tasks

#### Sub-Task S1: High-Performance Go Ingestion Server
- **Intent**: Construct the Go microserver to receive HTTP POST payloads and buffer them instantly in memory.
- **Expected Outcomes**:
  - Extremely responsive `/v1/batch` JSON POST ingestion endpoint.
  - In-memory channel buffer and background queue writer.
- **Todo List**:
  1. [ ] Initialize a new Go microserver repository `github.com/lbmesh/telemetry-server`.
  2. [ ] Define structural models for telemetry batches matching the client payloads.
  3. [ ] Set up the memory channel buffer:
     ```go
     var EventQueue = make(chan TelemetryEvent, 10000)
     ```
  4. [ ] Implement the HTTP ingestion handler: Validates request format, immediately pushes parsed events onto `EventQueue`, and responds with `202 Accepted` inside `3ms` to minimize HTTP connection times.

#### Sub-Task S2: MySQL Database Broker & Batch Writer
- **Intent**: Establish connection pooling to MySQL and execute batch inserts on intervals to maximize write throughput.
- **Expected Outcomes**:
  - Connection pool configuration with automatic reconnect loops.
  - Aggregated batch inserts executing every 2 seconds or when the batch size hits 200.
- **Todo List**:
  1. [ ] Connect to MySQL database using standard pure-Go driver (`github.com/go-sql-driver/mysql`).
  2. [ ] Write background database batcher: Drains `EventQueue`, pools records into a list, and constructs multi-row bulk insert queries:
     ```sql
     INSERT INTO telemetry_events (client_id, command, ..., created_at) VALUES (?, ?, ..., ?), (?, ?, ..., ?), ...
     ```
  3. [ ] Wrap bulk operations inside ACID database transactions to prevent half-written event chains on service restarts.

#### Sub-Task S3: Command-Init Activation (Downloads / Installations)
- **Intent**: Capture unique installations/downloads by tracking the initial `lbmesh init` command executions.
- **Expected Outcomes**:
  - Explicit metrics on unique node installations.
- **Todo List**:
  1. [ ] In client-side execution, guarantee that `lbmesh init` issues an immediate event record indicating the command: `init`.
  2. [ ] On the MySQL server-side, when an event comes in with the command `init`, perform an upsert (`INSERT ... ON DUPLICATE KEY UPDATE`) on the `clients` table, logging the first-seen activation timestamp.
  3. [ ] This counts as a **"unique installation activate"** which maps directly to downloads.

#### Sub-Task S4: Insights Dashboard Server
- **Intent**: Render a secure web dashboard inside the microserver to display graphical analytics summaries.
- **Expected Outcomes**:
  - Secure `/admin/dashboard` endpoint presenting key telemetry metrics.
- **Todo List**:
  1. [ ] Write SQL aggregation helpers inside the microserver:
     - **Unique Nodes / Total Activations**: Count of distinct `client_id`s with command `init`.
     - **Active Node Demographics**: OS (macOS vs Linux vs Windows) and Architecture (ARM64 vs AMD64) distributions.
     - **Hourly Command Frequency**: Timelines for commands (`up`, `down`, `ask`).
     - **Failure Rates**: Frequency of OOM or Docker connection failures.
     - **Top Stacks**: Most frequently deployed recipes (e.g. `postgres`, `redis`).
  2. [ ] Implement a lightweight dashboard template using server-rendered charts (using Chart.js or Tailwind CDN layout) or compile a simple React Carbon SPA to run at `/admin/dashboard` behind standard Basic Authentication.

---

## Part 3: Nginx Gateway Configuration & Security Hardening

The Nginx reverse proxy serves as the gateway security shielding `telemetry.lbmesh.io`. It guarantees that **global clients can post analytics metrics** while protecting the server against volume flood abuse and keeping the administrative dashboard strictly private.

### 1. Hardening Architecture
- **In-Memory Rate Limiting**: Limit individual client IP addresses to `5 requests per second` (using leaky bucket algorithm) to prevent script abuse, with a buffer burst rate of `10`.
- **Allowed HTTP Method Enforcement**: The `/v1/batch` route only permits `POST` requests. All other methods are rejected at the Nginx edge level.
- **Aggressive Payload Caps**: Enforces `client_max_body_size 128k` to protect the Go microserver from memory-exhaustion attempts via oversized HTTP request bodies.
- **Dashboard Isolation**: Restricts the `/admin` path behind HTTP Basic Authentication (`auth_basic`) and enables optional IP whitelisting.

### 2. Nginx Production Configuration File (`telemetry.lbmesh.io.conf`)

```nginx
# Rate Limiting Zone: Limit by client IP address.
# 10MB memory zone holds ~160,000 distinct IP addresses in state.
# Configures a limit of 5 requests per second (r/s) per IP.
limit_req_zone $binary_remote_addr zone=telemetry_limit:10m rate=5r/s;

server {
    listen 80;
    server_name telemetry.lbmesh.io;
    
    # Force HTTP to SSL/HTTPS redirect
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name telemetry.lbmesh.io;

    # SSL / TLS Hardening (Enforce TLS 1.2 & TLS 1.3 only, high-grade ciphers)
    ssl_certificate /etc/letsencrypt/live/telemetry.lbmesh.io/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/telemetry.lbmesh.io/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384';
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    # Standard Security Headers
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline';" always;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    # Aggressive Request Body Capping
    client_max_body_size 128k;

    # 1. Ingestion Endpoint: Publicly open to all users globally.
    # Restricts methods to POST only and enforces active rate limiting.
    location /v1/batch {
        limit_req zone=telemetry_limit burst=10 nodelay; # Apply IP rate limits
        
        limit_except POST {
            deny all; # Reject GET, PUT, DELETE, etc., immediately
        }

        proxy_pass http://127.0.0.1:8081; # Proxy to local Go microserver
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 2. Administrative Insights Dashboard: Private/Restricted.
    # Enforces Basic Authentication and restricts access to admin ranges.
    location /admin {
        auth_basic "LBMesh Admin Access Restricted";
        auth_basic_user_file /etc/nginx/.htpasswd; # htpasswd secure file path

        # Optional IP Whitelist restriction
        # allow 192.168.1.0/24; # Corporate range
        # allow <admin-office-ip>;
        # deny all;

        proxy_pass http://127.0.0.1:8081;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 3. Default fallback: reject all other arbitrary paths
    location / {
        return 404 "Resource not found";
    }
}
```
