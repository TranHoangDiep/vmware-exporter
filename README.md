# 🛰️ VMware Prometheus Exporter (Custom Version)

Phiên bản tùy chỉnh của VMware Exporter hỗ trợ giám sát đa vCenter, theo dõi trạng thái Port mạng (NIC), Port quang (FC) và tích hợp ESXCLI Firmware.

---

## 🛠 Hướng dẫn Build và Chạy (Local Build)

Vì mã nguồn đã được tùy chỉnh để thêm các tính năng giám sát Port vật lý, bạn cần build image trực tiếp từ thư mục này thay vì dùng image trên Docker Hub.

### 1. Cấu hình vCenter
Mở file `docker-compose.yml` và điền thông tin vCenter cũng như mật khẩu của bạn.

### 2. Build và Khởi động
Sử dụng lệnh sau để Docker tự động biên dịch lại code Go và khởi chạy container:

```bash
docker compose up -d --build
```

- `--build`: Ép Docker build lại image từ source code hiện tại (Dockerfile).
- `-d`: Chạy dưới nền (detached mode).

### 3. Kiểm tra trạng thái
```bash
docker compose ps
```

---

## 📊 Danh sách Metrics chính

Dưới đây là các thông số quan trọng nhất mà bộ sưu tập này cung cấp:

### 1. Giám sát Port vật lý (Mới cập nhật)
| Metric | Ý nghĩa | Trạng thái |
| :--- | :--- | :--- |
| `vmware_host_nic_link_status` | Trạng thái vật lý của Card mạng (vmnic) | 1: Up, 0: Down |
| `vmware_host_fc_link_status` | Trạng thái vật lý của Card quang (FC HBA) | 1: Online, 0: Offline |

### 2. Giám sát Host (ESXi)
| Metric | Ý nghĩa |
| :--- | :--- |
| `vmware_host_power_state` | Trạng thái nguồn (1: On, 0: Off, 2: Standby) |
| `vmware_host_cpu_usagemhz_average` | Lượng CPU đang sử dụng (MHz) |
| `vmware_host_mem_consumed_average` | Lượng RAM đang sử dụng (KB) |
| `vmware_host_cpu_capacity` | Tổng tần số CPU của Host |
| `vmware_host_info` | Thông tin chung (Version, Model, Vendor) |

### 3. Giám sát Datastore (Storage)
| Metric | Ý nghĩa |
| :--- | :--- |
| `vmware_datastore_capacity` | Tổng dung lượng ổ đĩa (Bytes) |
| `vmware_datastore_free` | Dung lượng còn trống (Bytes) |
| `vmware_datastore_uncommitted` | Dung lượng đã cấp phát nhưng chưa dùng hết (Thin provisioning) |

### 4. Giám sát Máy ảo (VM)
| Metric | Ý nghĩa |
| :--- | :--- |
| `vmware_vm_cpu_usagemhz_average` | CPU VM đang dùng |
| `vmware_vm_mem_consumed_average` | RAM VM đang dùng thực tế |
| `vmware_vm_sys_uptime_latest` | Thời gian máy ảo đã chạy liên tục |

### 5. Thông tin Firmware (ESXCLI)
| Metric | Ý nghĩa |
| :--- | :--- |
| `vmware_esxcli_host_nic_driver` | Thông tin Driver và Firmware của Card mạng |
| `vmware_esxcli_storage_driver` | Thông tin Driver và Firmware của Card Storage |

---

## 🚨 Hệ thống Cảnh báo (Prometheus Alerts)

Bạn có thể sử dụng file `prometheus-alerts.yml` đi kèm để thiết lập cảnh báo tự động:
- Cảnh báo khi rớt port mạng/quang (`vmware_host_nic_link_status == 0`).
- Cảnh báo khi Datastore đầy trên 90%.
- Cảnh báo khi Host bị ngắt kết nối với vCenter.

---

## 📈 Grafana Dashboards

Các file JSON trong thư mục `dashboards/` đã được tối ưu hóa:
- **vCenter View**: Cái nhìn tổng quan toàn bộ hệ thống.
- **Host Status Table**: (Mới) Bảng tổng hợp trạng thái Host (Hostname, IP, Uptime, CPU, RAM, Model, Status Up/Down).
- **Host View**: Chi tiết về port và tài nguyên host.
- **ESXCLI View**: Chuyên biệt để quản lý phiên bản Driver/Firmware.

---
*Ghi chú: Luôn đặt `vmware.interval=300` để đảm bảo độ ổn định của dữ liệu hiệu năng từ vCenter.*