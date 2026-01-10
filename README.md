# 🛰️ VMware Prometheus Exporter (Multi-vCenter Edition)

Bộ công cụ giám sát VMware vSphere chuyên nghiệp, hỗ trợ giám sát đa vCenter, theo dõi trạng thái Port mạng (NIC), Port quang (FC) và tích hợp hệ thống cảnh báo tự động.

---

## ✨ Tính năng nổi bật

- 🏢 **Multi-vCenter**: Giám sát nhiều vCenter cùng lúc bằng Docker Compose.
- 🔌 **Link Status Monitoring**: 
  - Theo dõi trạng thái kết nối vật lý của **NIC (vmnic)**.
  - Theo dõi trạng thái **Fibre Channel (FC HBA)** kèm mã **WWN**.
- 🛠 **ESXCLI Integration**: Thu thập thông tin Driver và Firmware của thiết bị lưu trữ và mạng.
- ⏱️ **Zero Timeout**: Cấu hình tối ưu (300s) giúp tránh lỗi timeout khi lấy dữ liệu từ các hệ thống ESXi chậm.
- 🚨 **Alerting Ready**: Đi kèm bộ quy tắc cảnh báo (Alert Rules) cho Prometheus.
- 📊 **Dashboard Toàn diện**: Hỗ trợ đầy đủ các View cho vCenter, Cluster, Host, Datastore và VM.

---

## 🚀 Hướng dẫn nhanh

### 1. Chuẩn bị cấu hình
Mở file `docker-compose.yml` và cập nhật thông tin vCenter của bạn:

```yaml
      - "-vmware.vcenter=vcenter-01.example.com"
      - "-vmware.username=administrator@vsphere.local"
      - "-vmware.password=MẬT_KHẨU_CỦA_BẠN"
```

### 2. Khởi chạy với Docker
```bash
docker compose up -d
```

### 3. Kiểm tra dữ liệu
Các dịch vụ sẽ chạy trên các cổng mặc định:
- **vCenter 01**: `http://<Server_IP>:9169/metrics`
- **vCenter 02**: `http://<Server_IP>:9170/metrics`

---

## 🚨 Hệ thống Cảnh báo (Alerting)

Tôi đã chuẩn bị sẵn file `prometheus-alerts.yml`. Bạn hãy thêm vào cấu hình Prometheus để nhận cảnh báo khi:
- **Host Down**: Host mất kết nối hoặc bị tắt.
- **Port Down (NIC/FC)**: Rớt cáp mạng hoặc mất kết nối quang SAN (Cực kỳ quan trọng).
- **Datastore Full**: Ổ cứng đầy trên 90%.
- **Snapshot Overload**: Máy ảo có quá nhiều snapshot gây chậm hệ thống.

---

## 📊 Grafana Dashboards

Mọi dữ liệu thu thập được có thể hiển thị sinh động qua các Dashboard trong thư mục `/dashboards`:
1.  **Host View**: Xem chi tiết tài nguyên, CPU Overcommit và trạng thái Port.
2.  **ESXCLI View**: Xem phiên bản Driver/Firmware của card mạng và card quang.
3.  **VM View**: Theo dõi hiệu năng chi tiết từng máy ảo.

---

## 🛠 Các Metric Quan trọng mới add

| Metric | Ý nghĩa | Trạng thái |
| :--- | :--- | :--- |
| `vmware_host_nic_link_status` | Trạng thái port mạng | 1: Up, 0: Down |
| `vmware_host_fc_link_status` | Trạng thái port quang FC | 1: Online, 0: Offline |
| `vmware_host_fc_link_status{wwn="..."}` | WWN của port quang | Giúp đối chiếu SAN Switch |

---

## 📝 Lưu ý kỹ thuật
- **Interval**: Đã cấu hình cố định **300s** để đảm bảo tương thích với vCenter Performance Manager. Đừng thay đổi số này sang các giá trị lạ (như 100, 600) để tránh lỗi `querySpec.interval`.
- **Prometheus Timeout**: Hãy đặt `scrape_timeout: 290s` trong Prometheus để đồng bộ với Exporter.

---
*Phát triển và tối ưu bởi Antigravity Team.*