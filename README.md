# 🛰️ VMware Monitoring Solution (Enterprise Edition)

Bộ giải pháp giám sát VMware vSphere toàn diện, hỗ trợ đa vCenter, theo dõi chi tiết đến từng Port vật lý, VLAN, Snapshot và Firmware. Giải pháp được tối ưu hóa cho môi trường NOC/MSP với hệ thống Dashboard Grafana chuẩn hóa màu sắc và cảnh báo thông minh.

---

## 🚀 Tính năng nổi bật

- **Đa vCenter (Multi-Tenancy)**: Chạy song song nhiều exporter cho các vCenter khác nhau chỉ với một file cấu hình duy nhất.
- **Giám sát Link Vật lý**: Theo dõi trạng thái thực tế của card mạng (NIC) và card quang (FC HBA) để phát hiện lỗi cáp hoặc switch port.
- **Quản lý VLAN & Network**: 
    - Hiển thị VLAN ID từng Port Group trên Host ESXi.
    - Ánh xạ VM đang kết nối vào Network/VLAN nào kèm theo địa chỉ IP và MAC.
- **Giám sát Snapshot thông minh**: 
    - Đếm số lượng Snapshot trên từng máy ảo.
    - Cảnh báo dựa trên **số ngày tồn tại (Age)** của Snapshot cũ nhất.
- **ESXCLI Firmware Insight**: Truy vấn trực tiếp phiên bản Driver và Firmware của phần cứng.
- **Trạng thái nguồn (Power State)**: Giám sát Host ngay cả khi bị tắt nguồn hoặc mất kết nối.

---

## 🛠 Hướng dẫn Cài đặt & Vận hành

### 1. Cấu hình hệ thống (Docker Compose)
Mã nguồn này được thiết kế để chạy dưới dạng Container. File `docker-compose.yml` đã được tối ưu cho việc build image tại chỗ (Local Build) để áp dụng các tùy chỉnh mới nhất.

```yaml
services:
  vcenter-01:
    build: .
    command:
      - "-vmware.vcenter=IP_VCENTER"
      - "-vmware.password=PASSWORD"
      - "-vmware.interval=300" # Quan trọng: Giữ ổn định 5 phút
```

### 2. Triển khai nhanh
```bash
# Tắt hệ thống cũ
docker compose down

# Build lại code mới và khởi chạy
docker compose up -d --build

# Xem log kiểm tra metrics
docker logs -f vmware-exporter-v01 | grep "Exporting"
```

---

## 📊 Danh mục Metrics (Glossary)

### 1. Host & Physical Connectivity
| Metric | Mô tả | Giá trị |
| :--- | :--- | :--- |
| `vmware_host_power_state` | Trạng thái nguồn vật lý | 1: Bật, 0: Tắt, 2: Chờ |
| `vmware_host_nic_link_status` | Kết nối vật lý Card mạng | 1: Up, 0: Down |
| `vmware_host_fc_link_status` | Kết nối vật lý Card quang | 1: Online, 0: Offline |
| `vmware_host_portgroup_vlan` | Cấu hình VLAN trên PortGroup | VLAN ID (1-4094, 4095: Trunk) |

### 2. Virtual Machine & Snapshot
| Metric | Mô tả | Lưu ý |
| :--- | :--- | :--- |
| `vmware_vm_snapshot_count` | Số lượng snapshot của VM | Càng nhiều càng chậm disk |
| `vmware_vm_snapshot_age_days` | Tuổi của snapshot cũ nhất | Dùng để cảnh báo dọn dẹp |
| `vmware_vm_network_info` | Thông tin mạng (IP, MAC, Network) | Yêu cầu cài VMware Tools |

---

## 📈 Hệ thống Grafana Dashboards

Chúng tôi cung cấp bộ Dashboard đã được chuẩn hóa màu sắc NOC (Green: OK, Yellow: Warning, Red: Critical):

1.  **vCenter View Overall**: 
    - Cái nhìn tổng thể toàn bộ Datacenter/Cluster.
    - Tích hợp bảng **Hosts Inventory** "Tất cả trong một": Hiển thị cấu hình phần cứng, OS và trạng thái NIC/FC ngay trên một dòng.
2.  **Host Status Table**: 
    - Bảng danh sách Host chuyên sâu giúp quản lý biết chính xác con nào đang Down, Uptime bao lâu và Model phần cứng là gì.
3.  **ESXCLI Firmware View**: 
    - Dành cho bảo trì: Kiểm tra nhanh phiên bản Driver/Firmware của các Host để đảm bảo tính đồng nhất (Compliance).
4.  **Network & Storage View**:
    - Theo dõi lưu lượng và dung lượng còn trống, đặc biệt là các cảnh báo về Snapshot cũ.

---

## 🚨 Hệ thống Cảnh báo
Dữ liệu từ Exporter được kết nối với Prometheus Alertmanager để gửi cảnh báo qua Telegram/Email:
- **Critical**: Rớt Port mạng/quang, Host Down, Datastore > 95%.
- **Warning**: Snapshot tồn tại > 7 ngày, Datastore > 85%, VM không cài VMware Tools.

---
*Duy trì bởi Antigravity Team - Tối ưu cho giám sát vận hành 24/7.*