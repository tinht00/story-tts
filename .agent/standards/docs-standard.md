# Docs & Knowledge Standard

## 1. Mục tiêu
- Bắt buộc đồng bộ tài liệu khi thay đổi tính năng, flow hoặc contract quan trọng.
- Giảm việc tri thức nằm rải rác trong chat mà không quay về repo.
- Giữ `docs/TODO.md` phản ánh đúng việc còn tồn.

## 2. Phạm vi áp dụng
- Toàn bộ project sau khi bootstrap từ starter kit này.
- Áp dụng cho source code, docs, flow/spec, decision log và ghi chú debug có giá trị tái sử dụng.

## 3. Khi nào bắt buộc cập nhật docs
- Thêm API mới.
- Đổi request, response, status code hoặc error handling quan trọng.
- Đổi flow nghiệp vụ hoặc luồng UI -> API -> DB.
- Chốt một quyết định kiến trúc ảnh hưởng đến nhiều file.
- Fix một lỗi có khả năng lặp lại mà người khác cần tra lại.

## 4. Tài liệu tối thiểu cần rà soát
- `AGENTS.md`
- `.agent/`
- `README.md`
- `docs/ai-digest.md`
- `docs/TODO.md`
- docs API/flow/spec/decision liên quan trực tiếp

## 5. Definition of Done về docs
Chỉ xem là hoàn tất khi:
- Source code đã được cập nhật
- Docs liên quan trực tiếp đã được rà soát và chỉnh nếu cần
- Các mục dang dở đã được ghi hoặc xoá đúng trong `docs/TODO.md`
- Mức verify phù hợp đã được chạy, hoặc đã nêu rõ lý do chưa thể chạy

## 6. Quy tắc ghi TODO
Mỗi mục TODO nên có đủ:
- bối cảnh ngắn
- trạng thái hiện tại
- việc còn thiếu
- cách verify sau khi làm

## 7. Quy tắc đồng bộ NotebookLM
- Chỉ đồng bộ sau khi docs trong repo đã là trạng thái mới nhất.
- NotebookLM là bản tra cứu dùng lặp lại, không thay cho repo.

