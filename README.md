# VMware Exporter for Prometheus

A comprehensive Prometheus exporter for VMware vCenter metrics including VMs, Hosts, Datastores, Clusters, and more.

## Features

- Multi-vCenter support via `/probe` endpoint
- Comprehensive VM, Host, Datastore, and Cluster metrics
- **Snapshot monitoring** (count, size, age, names)
- **Active alarm tracking** from vCenter
- **Physical hardware monitoring** (NIC/FC link status)
- **Network information** (IP, MAC, VLAN)
- Docker and Docker Compose ready

---

## Quick Start with Docker Compose

1. Edit `docker-compose.yml` with your vCenter credentials:
   ```yaml
   environment:
     - VMWARE_VCENTER=your-vcenter.local:443
     - VMWARE_USERNAME=admin@vsphere.local
     - VMWARE_PASSWORD=your-password
   ```

2. Start the exporter:
   ```bash
   docker compose up -d --build
   ```

3. Access metrics at: `http://localhost:9169/metrics`

---

## Configuration

Configure via **environment variables**, **YAML file**, or **command line flags**.

| Flag | Description | Default |
|------|-------------|---------|
| `-http.address` | Exporter bind address | `:9169` |
| `-vmware.vcenter` | vCenter address (host:port) | - |
| `-vmware.username` | vCenter username | - |
| `-vmware.password` | vCenter password | - |
| `-vmware.insecureTLS` | Skip TLS verification | `false` |
| `-vmware.interval` | Collection interval (seconds) | `20` |
| `-collector.datacenter` | Enable Datacenter metrics | `true` |
| `-collector.cluster` | Enable Cluster metrics | `true` |
| `-collector.datastore` | Enable Datastore metrics | `true` |
| `-collector.host` | Enable Host metrics | `true` |
| `-collector.vm` | Enable VM metrics | `true` |

---

## Metrics Reference

### VM Snapshot Metrics

| Metric | Description | Notes |
|--------|-------------|-------|
| `vmware_vm_snapshot_count` | Number of snapshots per VM | More snapshots = slower disk |
| `vmware_vm_snapshot_size_gb` | Total snapshot size (GB) | For disk capacity planning |
| `vmware_vm_snapshot_info` | Snapshot names list | Details each snapshot |
| `vmware_vm_snapshot_age_days` | Oldest snapshot age (days) | For cleanup alerts |

### VM Metrics

| Metric | Description | Notes |
|--------|-------------|-------|
| `vmware_vm_power_state` | VM power state | 1: On, 0: Off, 2: Suspended |
| `vmware_vm_cpu_usage_percent` | CPU usage percentage | - |
| `vmware_vm_memory_usage_percent` | Memory usage percentage | - |
| `vmware_vm_uptime_seconds` | VM uptime in seconds | - |
| `vmware_vm_tools_running_status` | VMware Tools status | 1: Running, 0: Not running |
| `vmware_vm_network_info` | Network info (IP, MAC, Network) | Requires VMware Tools |

### vCenter Alarm Metrics

| Metric | Description | Value |
|--------|-------------|-------|
| `vmware_alarm_triggered` | Active alarms from vCenter | 1: Yellow, 2: Red |

### Host Metrics

| Metric | Description | Value |
|--------|-------------|-------|
| `vmware_host_power_state` | Physical power state | 1: On, 0: Off, 2: Standby |
| `vmware_host_cpu_usage_percent` | CPU usage percentage | - |
| `vmware_host_memory_usage_percent` | Memory usage percentage | - |
| `vmware_host_nic_link_status` | Physical NIC connection | 1: Up, 0: Down |
| `vmware_host_nic_speed_mbps` | NIC speed in Mbps | - |
| `vmware_host_fc_link_status` | Fibre Channel status | 1: Online, 0: Offline |
| `vmware_host_portgroup_vlan` | PortGroup VLAN config | VLAN ID (1-4094, 4095: Trunk) |

### Datastore Metrics

| Metric | Description |
|--------|-------------|
| `vmware_datastore_capacity_bytes` | Total capacity |
| `vmware_datastore_free_bytes` | Free space |
| `vmware_datastore_accessible` | Accessibility status |

---

## Prometheus Configuration

```yaml
scrape_configs:
  # Single vCenter
  - job_name: 'vmware'
    static_configs:
      - targets: ['vmware-exporter:9169']

  # Multi-vCenter using /probe
  - job_name: 'vmware-probe'
    static_configs:
      - targets:
        - vcenter1.local:443
        - vcenter2.local:443
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: vmware-exporter:9169
    metrics_path: /probe
```

---

## Grafana Dashboards

Pre-built dashboards available in `/dashboards`:
- `vmware-snapshot-analysis.json` - Snapshot monitoring
- `vmware-unified-view.json` - Overview dashboard

---

## License

Made by THDIEP16