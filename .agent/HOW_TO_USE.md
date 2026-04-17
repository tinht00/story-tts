# Hướng Dẫn Vận Hành Agent

## Mục tiêu
Tài liệu này mô tả cách dùng bộ `.agent` tối thiểu của dự án sau khi bootstrap từ starter kit.

## Trình tự đọc tối thiểu
Trước khi làm việc dài hoặc nhiều bước:
1. Đọc `AGENTS.md` của dự án.
2. Đọc `.agent/rules.md`.
3. Đọc `.agent/rules/project_rules.md`.
4. Chỉ mở thêm workflow, standards hoặc docs khi task thực sự cần.

## Nguyên tắc chung
- Trả lời người dùng bằng tiếng Việt có dấu, trừ khi dự án hoặc người dùng yêu cầu ngôn ngữ khác.
- Có thể tự suy luận nội bộ bằng tiếng Anh nếu cần, nhưng phản hồi bên ngoài vẫn theo ngôn ngữ yêu cầu.
- Không quét toàn bộ docs theo mặc định; chỉ đọc phần tối thiểu đủ để giải quyết task.
- Ưu tiên source code khi xác minh hành vi thực tế.
- NotebookLM dùng cho governance và tài liệu lặp lại, không thay thế source code.
- Nếu dùng `nlm` lần đầu cho dự án hoặc session NotebookLM đã hết hạn, phải xử lý đăng nhập lại trước rồi mới tiếp tục tra cứu.
- Sau khi có quyền truy cập NotebookLM, phải tìm notebook hiện có của dự án trước; nếu không có thì tạo notebook canonical và nạp bộ tài liệu tối thiểu của dự án.
- Khi xác định được notebook thật, phải cập nhật `Notebook ID` và `URL` trong `AGENTS.md` thay cho placeholder.

## Khi nào cần cập nhật docs
- Có thay đổi chức năng
- Có thay đổi API contract
- Có thay đổi flow hoặc decision quan trọng
- Có bug fix đáng lưu lại để tái sử dụng khi debug
- Có task bị chặn hoặc chưa hoàn tất cần ghi lại trong `docs/TODO.md`

## Cách dùng workflow
- Nếu cần biết cách chạy local hoặc lệnh cơ bản, mở `.agent/workflows/dev.md`
- Nếu task là thêm tính năng backend, mở `.agent/workflows/backend_feature.md`
- Nếu task là thêm tính năng frontend, mở `.agent/workflows/frontend_feature.md`
- Nếu mới bootstrap dự án hoặc cần rà soát governance, mở `.agent/workflows/bootstrap.md`
