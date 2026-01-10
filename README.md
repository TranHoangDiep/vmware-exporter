# VMware Exporter (Bản Go)

Đây là trình thu thập dữ liệu (exporter) cho Prometheus, giúp lấy dữ liệu từ hệ thống VMware vSphere/vCenter. Exporter này được tối ưu bằng ngôn ngữ Go, có hiệu năng cao và hỗ trợ nhiều vCenter cùng lúc.

## 🚀 Cách sử dụng nhanh (Docker Compose)

Để giám sát nhiều vCenter với các thông tin đăng nhập khác nhau, chúng ta sử dụng `docker-compose.yml` để chạy nhiều instance của exporter.

### 1. Cấu hình vCenter
Mở file `docker-compose.yml` và chỉnh sửa thông tin cho từng service:

```yaml
services:
  vcenter-01:
    image: lam86/vmware-exporter:latest
    ports:
      - "9169:9169" # Cổng cho vCenter 01
    command:
      - "-vmware.vcenter=10.10.x.x"
      - "-vmware.username=administrator@vsphere.local"
      - "-vmware.password=MAT_KHAU_CUA_BAN"
      # ... cấu hình collector ...

  vcenter-02:
    image: lam86/vmware-exporter:latest
    ports:
      - "9170:9169" # Cổng cho vCenter 02
    command:
      - "-vmware.vcenter=10.10.y.y"
      - "-vmware.username=administrator@vsphere.local"
      - "-vmware.password=MAT_KHAU_KHAC"
      # ... cấu hình collector ...
```

### 2. Khởi động
```bash
docker-compose up -d
```

### 3. Kiểm tra dữ liệu
Sau khi khởi động, bạn có thể truy cập các địa chỉ sau để xem metrics:
*   **vCenter 01:** `http://<IP_Server>:9169/metrics`
*   **vCenter 02:** `http://<IP_Server>:9170/metrics`

## 📊 Các Collector hỗ trợ

| Tham số | Chức năng (Bật: true, Tắt: false) |
| :--- | :--- |
| `-collector.vm` | Thu thập metrics Máy ảo (CPU, RAM, Disk, Uptime, Tools, Snapshot...) |
| `-collector.host` | Thu thập metrics Máy chủ vật lý (Hardware info, CPU model, RAM...) |
| `-collector.datastore` | Thu thập metrics Ổ cứng lưu trữ (Capacity, Free space...) |
| `-collector.cluster` | Thu thập metrics Cluster (Cụm máy chủ) |
| `-collector.datacenter` | Thu thập metrics Datacenter |
| `-collector.esxcli.host.nic` | Thu thập thông tin Firmware NIC của ESXi (via ESXCLI) |
| `-collector.esxcli.storage` | Thu thập thông tin Firmware Storage của ESXi (via ESXCLI) |

## 🛠️ Các tham số quan trọng khác

*   `-vmware.insecureTLS=true`: Bỏ qua xác thực chứng chỉ SSL (thường dùng cho các vCenter dùng cert tự ký).
*   `-vmware.interval=20`: Chu kỳ thu thập dữ liệu (mặc định 20 giây).
*   `-log.level=info`: Mức độ ghi log (debug, info, warn, error).

## ⚠️ Lưu ý khi gặp lỗi
Nếu exporter báo lỗi `lookup ... no such host`, hãy kiểm tra:
1.  Địa chỉ IP của vCenter trong file cấu hình có chứa ký tự lạ không (ví dụ: dư chữ `v` ở đầu).
2.  Khả năng kết nối mạng từ máy chạy Docker tới IP vCenter.
3.  Đảm bảo mật khẩu không chứa các ký tự đặc biệt lồng nhau nếu không được đặt trong dấu ngoặc kép.