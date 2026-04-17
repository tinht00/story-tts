# Workflow: Bootstrap Governance

## Mục tiêu
Thiết lập nhanh governance tối thiểu cho một project mới ngay sau khi copy starter kit.

## Các bước
1. Điền `AGENTS.md` với tên project, notebook canonical và mô tả ngắn.
2. Điền `.agent/project_settings.md`.
3. Điền `.agent/rules/project_rules.md` bằng constraint thật của dự án.
4. Điền `.agent/context/architecture.md` ở mức high-level.
5. Khi cần dùng NotebookLM hoặc `nlm`, kiểm tra đăng nhập trước; nếu session không hợp lệ thì mở lại luồng đăng nhập và xác nhận truy cập thành công.
6. Tìm notebook hiện có của dự án bằng canonical name, alias hoặc metadata. Nếu tìm thấy, cập nhật `Notebook ID` và `URL` thật vào `AGENTS.md`.
7. Nếu chưa có notebook phù hợp, tạo notebook canonical `[ProjectName] - Governance & Docs`, upload bộ tài liệu tối thiểu như `AGENTS.md`, `.agent/`, `docs/`, `README.md` và ghi lại `Notebook ID` cùng `URL`.
8. Cập nhật `docs/ai-digest.md` với tri thức ổn định ban đầu.
9. Tạo `docs/TODO.md` nếu đã có hạng mục dang dở cần theo dõi hoặc còn tài liệu cần upload thêm lên notebook.

## Kết quả mong đợi
- Project có điểm điều hướng rõ ràng cho agent
- Rule chung và rule riêng được tách bạch
- Có nơi lưu tri thức ổn định và việc còn tồn
