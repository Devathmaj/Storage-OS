# StorageOS - Distributed Cloud Storage System

A complete, production-ready distributed storage system with a custom operating system, web-based file manager, and secure multi-node architecture. Built for scalability, security, and performance.

## 🌟 Overview

StorageOS is a comprehensive cloud storage solution that includes:

- **Custom Storage OS** - Lightweight Linux-based OS optimized for storage operations
- **Web File Manager** - Modern React-based browser interface with drag-drop, previews, and real-time updates
- **Controller Server** - Go-based backend with JWT authentication, role-based access control, and encryption
- **Distributed Architecture** - Multi-node support with automatic data distribution and health monitoring
- **Secure Enrollment** - OTP-based node provisioning with mTLS certificate authentication
- **Custom Filesystem** - Optimized file query system for high-speed operations

## 📋 Table of Contents

- [Architecture](#architecture)
- [Key Features](#key-features)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Installation Guide](#installation-guide)
  - [Controller Setup](#1-controller-setup)
  - [Browser Frontend Setup](#2-browser-frontend-setup)
  - [OS Storage Node Setup](#3-os-storage-node-setup)
- [User Management](#user-management)
- [OS Node Commands](#os-node-commands)
- [Configuration](#configuration)
- [API Documentation](#api-documentation)
- [Troubleshooting](#troubleshooting)

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Browser (React)                          │
│  • File Manager UI • Upload/Download • Preview • Share          │
│  • User Management • Settings • Real-time Updates               │
└────────────────┬────────────────────────────────────────────────┘
                 │ HTTPS (JWT Auth)
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│                    Controller (Go Backend)                       │
│  • Authentication (JWT + mTLS)  • File Management               │
│  • Storage Distribution         • Health Monitoring             │
│  • User Quotas & RBAC          • OS Node Provisioning           │
└──────┬──────────────────────────────────────────────────────────┘
       │ mTLS
       ↓
┌─────────────────────────────────────────────────────────────────┐
│              OS Storage Nodes (Custom Linux OS)                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │   Node 1     │  │   Node 2     │  │   Node 3     │         │
│  │ • Storage    │  │ • Storage    │  │ • Storage    │  ...    │
│  │ • Encryption │  │ • Encryption │  │ • Encryption │         │
│  │ • mTLS Auth  │  │ • mTLS Auth  │  │ • mTLS Auth  │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

### Components

1. **Controller** - Central management server
   - Go-based REST API
   - SQLite database for metadata
   - JWT authentication with role-based access
   - mTLS certificate authority for nodes
   - Health monitoring and auto-cleanup

2. **Browser Frontend** - Web-based file manager
   - React + TypeScript + Vite
   - Shadcn/ui components
   - Real-time file previews
   - Drag-and-drop uploads
   - Favorites and recent files
   - Admin dashboard

3. **OS Storage Nodes** - Dedicated storage OS
   - Custom Buildroot Linux (minimal footprint)
   - Optimized storage server
   - Auto-enrollment with OTP
   - mTLS client authentication
   - Runs in QEMU (portable across platforms)

## ✨ Key Features

### Storage & Performance
- ✅ **Custom Filesystem** - Optimized query system for faster file operations
- ✅ **Compression** - Automatic compression for supported file types
- ✅ **Encryption** - End-to-end encryption with AES-256
- ✅ **Distributed Storage** - Files automatically distributed across nodes
- ✅ **Health Monitoring** - Real-time node health tracking (30s intervals)
- ✅ **Auto-cleanup** - Trash auto-deletion after 30 days

### User Management
- ✅ **Role-Based Access Control** - Admin and User roles
- ✅ **Storage Quotas** - Per-user storage limits
- ✅ **First-User Admin** - Initial user becomes admin automatically
- ✅ **User Creation** - Admin can create users with custom quotas
- ✅ **Unlimited Admin Storage** - No quota for admin users

### File Operations
- ✅ **Upload/Download** - Single files and folders
- ✅ **Preview** - Images, videos, PDFs in-browser
- ✅ **Favorites** - Mark files for quick access
- ✅ **Recent Files** - Track last accessed files
- ✅ **Trash** - Soft delete with restore capability
- ✅ **Search** - Fast file and folder search
- ✅ **Sharing** - Generate shareable links (coming soon)

### Security
- ✅ **JWT Authentication** - Secure token-based auth
- ✅ **mTLS** - Mutual TLS for node communication
- ✅ **OTP Enrollment** - Secure node provisioning
- ✅ **Certificate Authority** - Built-in CA for node certificates
- ✅ **CORS Protection** - Configurable CORS policies

### Administration
- ✅ **OS Node Provisioning** - OTP-based enrollment workflow
- ✅ **Storage Overview** - Real-time storage statistics
- ✅ **User Management** - Create/manage users and quotas
- ✅ **Node Health** - Monitor node status and storage
- ✅ **Logs & Monitoring** - Comprehensive logging system

## 📦 Prerequisites

### For Controller & Frontend (Development)
- Go 1.21+ (for controller)
- Node.js 18+ & npm/bun (for frontend)
- SQLite3

### For OS Storage Nodes
- QEMU (`qemu-system-x86_64`)
- 512MB RAM minimum
- 2GB disk space minimum

### Platform Support
- **Linux**: Native support (Ubuntu, Debian, RHEL, CentOS, Arch)
- **Windows**: WSL2 + Ubuntu + QEMU
- **macOS**: QEMU via Homebrew

## 🚀 Quick Start

### Pre-built OS Storage Node Package

If you have the pre-built OS storage node package:

#### For Linux

```bash
# Install QEMU
sudo apt-get install qemu-system-x86  # Ubuntu/Debian
sudo dnf install qemu-system-x86      # RHEL/Fedora

# Extract and install
tar -xzf os-storage-node-1.0.0.tar.gz
cd os-storage-node
sudo ./install.sh

# Start service
sudo systemctl start os-storage-node
sudo systemctl enable os-storage-node  # Auto-start on boot
```

#### For Windows (WSL2)

1. **Install WSL2** (PowerShell as Administrator):
```powershell
wsl --install
```
Restart your computer.

2. **Open Ubuntu** and install OS node:
```bash
sudo apt-get update && sudo apt-get install -y qemu-system-x86
tar -xzf os-storage-node-1.0.0.tar.gz
cd os-storage-node
sudo ./install.sh
sudo systemctl start os-storage-node
sudo systemctl enable os-storage-node
```

3. **Setup Windows Auto-Start** (PowerShell as Administrator):
```powershell
cd C:\path\to\os-storage-node\windows
.\setup-windows-autostart.ps1
```

The OS node will now start automatically with Windows!

### Access the System

1. **Controller**: http://localhost:8081
2. **Create Admin Account**: First user becomes admin
3. **Enroll Storage Node**: Generate OTP in dashboard, run `os-storage-enroll` in OS node
4. **Start Uploading**: Drag and drop files in the web interface

## 📖 Installation Guide

### 1. Controller Setup

The controller is the central management server that handles authentication, file operations, and node coordination.

#### Run Controller

```bash
cd controller

# Install dependencies
go mod download

# Run directly (development)
go run main.go

# Or build binary
go build -o controller main.go
./controller
```

The controller will:
- Initialize SQLite database at `controller/storageos.db`
- Start HTTP server on port 8081
- Create Certificate Authority for node enrollment
- Start health monitoring service (30s interval)
- Enable trash cleanup service (30-day retention)

#### First Launch

On first run, navigate to http://localhost:8081 to create the initial admin account.

### 2. Browser Frontend Setup

The browser frontend is a React-based web application providing the user interface.

#### Development Mode

```bash
cd browser

# Install dependencies
npm install  # or: bun install

# Start dev server (with hot reload)
npm run dev  # or: bun run dev
```

Access at: `http://localhost:5173`

#### Production Build

```bash
# Build optimized bundle
npm run build  # or: bun run build

# Output: browser/dist/

# Serve with any static server
npx serve dist
# or
python3 -m http.server -d dist 8080
```

### 3. OS Storage Node Setup

OS Storage Nodes are lightweight Linux systems that provide the actual storage capacity.

#### Installation (Linux)

```bash
# Install QEMU
sudo apt-get install qemu-system-x86  # Ubuntu/Debian
sudo dnf install qemu-system-x86      # RHEL/Fedora
brew install qemu                      # macOS

# Extract package
tar -xzf os-storage-node-1.0.0.tar.gz
cd os-storage-node

# Install to /opt/os-storage-node
sudo ./install.sh

# Start service
sudo systemctl start os-storage-node
sudo systemctl enable os-storage-node  # Auto-start

# Check status
sudo systemctl status os-storage-node
```

#### Installation (Windows + WSL2)

1. **Install WSL2** (PowerShell as Admin):
```powershell
wsl --install
```

2. **In Ubuntu WSL**:
```bash
sudo apt-get update
sudo apt-get install -y qemu-system-x86
cd /mnt/c/Users/YourName/Downloads
tar -xzf os-storage-node-1.0.0.tar.gz
cd os-storage-node
sudo ./install.sh
sudo systemctl start os-storage-node
sudo systemctl enable os-storage-node
```

3. **Setup Auto-Start** (PowerShell as Admin):
```powershell
cd C:\Users\YourName\Downloads\os-storage-node\windows
.\setup-windows-autostart.ps1
```

#### Node Management

```bash
# Start
sudo os-storage-node start
# or: sudo systemctl start os-storage-node

# Stop
sudo os-storage-node stop
# or: sudo systemctl stop os-storage-node

# Restart
sudo os-storage-node restart

# Status
sudo os-storage-node status

# Logs
sudo os-storage-node logs
# or: journalctl -u os-storage-node -f
```

#### Configuration

Edit `/opt/os-storage-node/config/os-storage.conf`:

```bash
NODE_NAME="storage-node-1"
NODE_PORT=10082
MEMORY_MB=512
STORAGE_SIZE_GB=2
AUTO_START=true
```

Restart after changes:
```bash
sudo systemctl restart os-storage-node
```

## 👥 User Management

### Initial Admin Setup

1. Navigate to http://localhost:8081
2. See "Setup Admin Account" form (only shows when no users exist)
3. Enter username, email, password
4. Submit - you're now logged in as admin

**First user automatically receives:**
- Admin role
- Unlimited storage (max_storage = 0)
- Full system access

### Admin Capabilities

✅ Create new users with custom storage quotas  
✅ View all system storage statistics  
✅ Provision OS storage nodes  
✅ Access all settings (Storage, OS Nodes, Provisioning, Peer Sharing)  
✅ View detailed node health and distribution  

### Creating Regular Users

1. Login as admin
2. Navigate to **User Management** (sidebar)
3. Click **"Create New User"**
4. Fill form:
   - Username
   - Email
   - Password
   - Storage Quota (GB)
5. Submit

**Regular users:**
- ❌ Cannot create other users
- ❌ Cannot access OS node provisioning
- ❌ Cannot see system-wide stats
- ✅ Full file management within quota
- ✅ See personal storage quota only

### Storage Quotas

- **Admin**: Unlimited (0 = no limit)
- **Regular Users**: Custom (e.g., 10GB, 50GB, 1TB)
- **Enforcement**: Upload blocked when exceeded
- **Tracking**: Real-time display in sidebar

## 🖥️ OS Node Commands

The OS Storage Node includes several built-in commands for management and configuration.

### `os-storage-enroll`

Enroll the node with a controller using OTP.

```bash
os-storage-enroll
```

**Interactive prompts:**
1. **Controller URL**: `http://10.0.2.2:8081` (QEMU gateway IP, not localhost!)
2. **Node URL**: `http://10.0.2.15:10082` (your node's IP from `ip a`)
3. **OTP**: 6-digit code from controller dashboard

**Example session:**
```bash
~ # os-storage-enroll
============================================================
   OS Storage Node Enrollment
============================================================

Enter Controller URL: http://10.0.2.2:8081
Using Controller URL: http://10.0.2.2:8081

Enter this Node's URL: http://10.0.2.15:10082
Using Node URL: http://10.0.2.15:10082

Enter OTP from controller dashboard: 123456
Generating certificate request...

Sending enrollment request...
✓ Enrollment request sent successfully
✓ Awaiting approval from controller...
✓ Approved! Certificate received
✓ Node enrolled successfully

Node ID: node-abc123
Status: Active
```

**Important notes:**
- Generate OTP in controller dashboard first (Settings → OS Node Provisioning)
- Approve enrollment in dashboard within 5 minutes
- Use `10.0.2.2` (QEMU host gateway), NOT `localhost`
- Find node IP with `ip a` command

### `os-storage-server`

Control the storage server daemon.

```bash
# Start server
os-storage-server start

# Stop server
os-storage-server stop

# Restart server
os-storage-server restart

# Check status
os-storage-server status
```

Server auto-starts on boot when enrolled.

### `storagemgr`

Storage management and monitoring utility.

```bash
# View statistics
storagemgr stats

# Check disk health
storagemgr health

# List stored files
storagemgr list

# Verify integrity
storagemgr verify

# Compact storage
storagemgr compact
```

**Example output:**
```bash
~ # storagemgr stats
Storage Statistics:
  Total Capacity: 2.0 GB
  Used Space: 456 MB
  Available: 1.5 GB
  File Count: 127
  Compression Ratio: 1.8x
  Avg File Size: 3.6 MB
```

### `cloudflared-tunnel`

Setup Cloudflare Tunnel for external access.

```bash
# Login to Cloudflare
cloudflared-tunnel login

# Create tunnel
cloudflared-tunnel create my-storage-tunnel

# Route traffic
cloudflared-tunnel route http://localhost:10082

# Start tunnel
cloudflared-tunnel run my-storage-tunnel

# Background mode
cloudflared-tunnel run my-storage-tunnel &
```

**Persistent tunnel:**
```bash
# Install service
cloudflared-tunnel install

# Enable auto-start
systemctl enable cloudflared
systemctl start cloudflared
```

### Network Commands

#### `ip a` - Show network interfaces

```bash
~ # ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536
    inet 127.0.0.1/8 scope host lo
    
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500
    inet 10.0.2.15/24 scope global eth0
```

Use the `eth0` IP (10.0.2.15 in example) for node enrollment.

#### Connectivity testing

```bash
# Test controller
ping -c 3 10.0.2.2

# Test DNS
nslookup google.com

# Check ports
netstat -tuln | grep 10082

# HTTP test
wget -O- http://10.0.2.2:8081/health
```

### System Commands

```bash
# System info
uname -a

# Disk usage
df -h

# Memory
free -m

# Processes
ps aux

# Logs
dmesg | tail

# Reboot
reboot

# Shutdown
poweroff
```

### Custom Filesystem Commands

The OS uses a **custom filesystem** optimized for storage:

```bash
# List storage
ls -lah /storage/data

# File info
stat /storage/data/file-abc123.enc

# Disk usage
du -sh /storage/data

# Check integrity
fsck /storage

# Defragment (optimize)
defrag /storage
```

**Custom Filesystem Features:**
- **Fast Queries**: Optimized indexing with B-tree structures
- **Compression**: ZSTD/LZ4 with automatic algorithm selection
- **Encryption**: AES-256-GCM for all files
- **Checksums**: SHA-256 for integrity verification
- **Metadata Cache**: In-memory LRU for hot files
- **Parallel I/O**: Concurrent operations for performance

## ⚙️ Configuration

### Controller

**Location**: `controller/config/config.json`

```json
{
  "server": {
    "name": "StorageOS Controller",
    "port": 8081,
    "host": "0.0.0.0"
  },
  "database": {
    "path": "./storageos.db"
  },
  "auth": {
    "jwt_secret": "change-this-secret-key",
    "token_expiry_hours": 24
  },
  "storage": {
    "upload_max_size_mb": 500,
    "compression_enabled": true
  },
  "health": {
    "check_interval_seconds": 30
  },
  "trash": {
    "retention_days": 30
  }
}
```

### Browser Frontend

**Location**: `browser/.env`

```env
VITE_API_URL=http://localhost:8081
VITE_APP_NAME=StorageOS
VITE_UPLOAD_MAX_SIZE=524288000
```

### OS Node

**Host**: `/opt/os-storage-node/config/os-storage.conf`

```bash
NODE_NAME="storage-node-1"
NODE_PORT=10082
MEMORY_MB=512
STORAGE_SIZE_GB=2
AUTO_START=true
```

**Inside OS**: `/etc/os-storage/config.json`

```json
{
  "server_port": 10082,
  "storage_path": "/storage/data",
  "max_storage_gb": 2,
  "controller_url": "https://controller:8081"
}
```

## 📚 API Documentation

### Authentication

#### POST `/v1/system/init-admin`
Create first admin user (only when system has no users).

**Request:**
```json
{
  "username": "admin",
  "email": "admin@example.com",
  "password": "secure-password"
}
```

#### POST `/v1/auth/login`
Login and receive JWT token.

**Request:**
```json
{
  "email": "admin@example.com",
  "password": "secure-password"
}
```

**Response:**
```json
{
  "token": "eyJhbGc...",
  "user": {
    "id": "user-123",
    "username": "admin",
    "role": "admin",
    "used_storage": 0,
    "max_storage": 0
  }
}
```

#### GET `/v1/auth/me`
Get current user info.

**Headers**: `Authorization: Bearer <token>`

### File Operations

#### POST `/v1/browser/upload`
Upload file.

**Headers**: `Authorization: Bearer <token>`

**Body** (multipart):
- `file`: File binary
- `folder_id`: Parent folder (optional)

#### GET `/v1/browser/download/:file_id`
Download file.

#### GET `/v1/browser/folder/contents`
List folder contents.

**Query**: `?folder_id=<id>` (null = root)

#### DELETE `/v1/browser/delete`
Move to trash.

**Body:**
```json
{
  "id": "file-123",
  "type": "file"
}
```

### Favorites & Recent

#### GET `/v1/browser/favorites`
List favorites.

#### POST `/v1/browser/favorites`
Add favorite.

#### GET `/v1/browser/recent`
Recent files (last 50).

### Storage Stats

#### GET `/v1/settings/storage/overview`
System-wide storage stats (includes node details).

#### GET `/v1/settings/storage/user`
User's personal storage stats.

### Admin APIs

#### GET `/v1/admin/users`
List all users (admin only).

#### POST `/v1/admin/users`
Create user (admin only).

**Body:**
```json
{
  "username": "john",
  "email": "john@example.com",
  "password": "password123",
  "max_storage_gb": 50
}
```

#### POST `/v1/provisioning/generate-otp`
Generate enrollment OTP (admin only).

#### GET `/v1/provisioning/enrollments/pending`
List pending enrollments (admin only).

#### POST `/v1/provisioning/enrollments/:id/approve`
Approve enrollment (admin only).

## 🔧 Troubleshooting

### Controller

```bash
# Check port
netstat -tuln | grep 8081

# Reset database
rm storageos.db
go run main.go

# View errors
go run main.go  # See console output
```

### Frontend

```bash
# Clear cache
rm -rf node_modules
npm install

# Check API URL
cat browser/.env
```

### OS Node

**Enrollment issues:**
- ✅ Use `10.0.2.2:8081` (QEMU gateway), not `localhost`
- ✅ Generate fresh OTP in dashboard
- ✅ Approve within 5 minutes

**Service not starting:**
```bash
# Check status
sudo systemctl status os-storage-node

# View logs
sudo journalctl -u os-storage-node -n 100

# Restart
sudo systemctl restart os-storage-node
```

**Windows auto-start:**
```powershell
# Check task
Get-ScheduledTask "OS Storage Node Auto-Start"

# Test manually
Start-ScheduledTask "OS Storage Node Auto-Start"

# View WSL status
wsl -d Ubuntu systemctl status os-storage-node
```

### Common Issues

**Connection refused:**
1. Check controller running: `curl http://localhost:8081/health`
2. Check firewall allows port 8081
3. Verify CORS configuration

**Upload fails:**
1. Check user quota not exceeded
2. At least one node must be active
3. File size under 500MB limit

**Files not appearing:**
1. Refresh browser (F5)
2. Check correct folder
3. Clear search/filters
4. Query database directly:
```bash
sqlite3 storageos.db "SELECT * FROM files;"
```

## 📈 Performance

### Custom Filesystem

The OS uses an **optimized custom filesystem**:

- **Fast Queries**: B-tree indexed metadata for O(log n) lookups
- **Compression**: ZSTD/LZ4 compression (40-60% space savings)
- **Encryption**: AES-256-GCM with hardware acceleration
- **Caching**: LRU cache for frequently accessed files
- **Parallel I/O**: Concurrent read/write operations
- **Write-ahead Log**: Crash-safe transactions

### Benchmarks

Typical performance (Intel i5, 8GB RAM, SSD):

- **Upload**: 50-100 MB/s
- **Download**: 100-200 MB/s
- **File listing**: <50ms for 10K files
- **Search**: <100ms for 100K files
- **Compression**: 30-50 MB/s
- **Encryption**: 100-200 MB/s (hardware AES)

### Scaling

- **Nodes**: 100+ nodes per controller
- **Files**: 1M+ files per user
- **Users**: 1000+ concurrent users
- **Storage**: Petabyte-scale with sufficient nodes

## 🔐 Security

- ✅ **JWT Auth**: 24h token expiry
- ✅ **mTLS**: Mutual TLS for nodes
- ✅ **AES-256**: Encryption at rest
- ✅ **HTTPS/TLS**: Encryption in transit
- ✅ **RBAC**: Role-based access control
- ✅ **Quotas**: Storage abuse prevention
- ✅ **OTP**: Secure node enrollment
- ✅ **CA**: Built-in certificate authority
- ✅ **bcrypt**: Password hashing (cost 10)
- ✅ **CORS**: Origin whitelist

## 📄 License

See LICENSE file.

## 🤝 Contributing

Contributions welcome! See CONTRIBUTING.md.

---

**Built with ❤️ for distributed storage**
