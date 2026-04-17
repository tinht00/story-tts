# Workflow: Frontend Feature

## 1. Phân tích
- Xác định input của người dùng, output hiển thị và trạng thái lỗi/loading.
- Kiểm tra xem logic nên nằm ở component, store, route hay service API.
- Xác định ảnh hưởng tới contract backend hoặc state dùng chung.

## 2. Triển khai
### Bước 1: UI state
- Xác định state local và state dùng chung.
- Tránh để một component ôm quá nhiều trách nhiệm nếu có thể tách rõ.

### Bước 2: Data flow
- Tách phần gọi API/service ra khỏi phần render nếu structure hiện có của repo hỗ trợ.
- Chuẩn hóa error/loading/empty state.

### Bước 3: Presentation
- Cập nhật component, form, table, modal hoặc route liên quan.
- Thêm comment ngắn ở những đoạn có ràng buộc UI đặc biệt.

## 3. Kiểm tra
- Chạy test/build/lint phù hợp nếu môi trường cho phép.
- Kiểm tra lại các case: loading, success, validation error, empty data, permission nếu có.
- Với flow liên quan file hoặc audio/video, thử thao tác thực tế end-to-end nếu có thể.

## 4. Đồng bộ docs
- Cập nhật docs UI/flow nếu thay đổi hành vi.
- Ghi TODO nếu còn blocker hoặc phần chưa hoàn thiện.

