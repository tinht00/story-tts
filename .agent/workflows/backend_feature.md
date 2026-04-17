# Workflow: Backend Feature

## 1. Phân tích
- Xác định input, output, contract và các case lỗi chính.
- Kiểm tra logic này thuộc handler, service, repository hay module nền nào.
- Xác định có ảnh hưởng DB, queue, file storage, API contract hay không.

## 2. Triển khai
### Bước 1: Model và contract
- Định nghĩa hoặc cập nhật request/response, entity, DTO nếu cần.

### Bước 2: Data access
- Viết hoặc cập nhật repository/query/adapter tương ứng.
- Kiểm tra các điều kiện lỗi và tính tương thích của câu query nếu dự án có constraint DB.

### Bước 3: Business logic
- Viết service hoặc use case xử lý nghiệp vụ.
- Giữ logic rõ ràng, tránh nhồi toàn bộ vào controller.

### Bước 4: Transport
- Cập nhật controller/handler/router.
- Trả lỗi rõ ràng theo contract của dự án.

## 3. Kiểm tra
- Chạy test/build tối thiểu nếu môi trường cho phép.
- Gọi thử API thật hoặc test tích hợp ở mức phù hợp.
- Kiểm tra log, error path và dữ liệu ghi xuống DB/file/external service.

## 4. Đồng bộ docs
- Cập nhật docs API/flow nếu có thay đổi.
- Nếu chưa hoàn tất toàn bộ, ghi phần còn lại vào `docs/TODO.md`.

