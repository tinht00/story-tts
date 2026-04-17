# Project Protocol

Tài liệu này là quy chuẩn toàn cục của dự án, kết hợp rule kỹ thuật chung và protocol làm việc với agent.

## 1. Triết lý cốt lõi
**Strict & Explicit**

- Mọi thay đổi phải rõ mục đích, phạm vi và ảnh hưởng.
- Ưu tiên code dễ đọc, dễ kiểm tra và dễ bảo trì.
- Không được nuốt lỗi một cách im lặng; lỗi phải được xử lý hoặc bề mặt hóa rõ ràng.

## 2. Nguồn sự thật và phạm vi tra cứu
- Source code là nguồn sự thật cho implementation và runtime.
- `AGENTS.md`, `.agent/`, `docs/` và NotebookLM là lớp tri thức hỗ trợ điều hướng và tra cứu.
- Không đọc hàng loạt docs nếu chưa cần; chỉ mở đúng phần phục vụ task hiện tại.
- Khi dùng NotebookLM hoặc `nlm`, ưu tiên tái dùng notebook hiện có của dự án trước khi tạo notebook mới.

## 3. Quy tắc kỹ thuật chung
- Viết comment ngắn gọn ở những đoạn khó hiểu hoặc có chủ ý đặc biệt.
- Không thêm abstraction sớm khi chưa có lý do rõ ràng.
- Tôn trọng structure có sẵn của dự án; chỉ refactor mạnh khi có nhu cầu thật và phạm vi đã rõ.
- Khi thêm logic mới, cân nhắc nơi đặt code theo ranh giới hiện có của dự án.

## 4. Quy tắc theo nhóm stack
### Golang
- Ưu tiên luồng rõ ràng theo handler -> service -> repository nếu dự án theo kiến trúc lớp.
- Kiểm tra `err` ngay sau lệnh có thể lỗi.
- Khi thao tác I/O, path, network hoặc concurrency, cần xử lý lỗi tường minh.

### Vue 3
- Nếu dự án dùng Vue 3 hiện đại, ưu tiên Composition API và `script setup`.
- State dùng theo convention hiện có của repo, ví dụ Pinia nếu repo đã dùng.
- Tránh đưa logic nặng vào template.

### React
- Ưu tiên functional components.
- Tôn trọng style hiện có của repo về state và data fetching.
- Chỉ thêm tối ưu như memoization khi có lý do hoặc repo đã dùng pattern đó.

### Python
- Ưu tiên type hints.
- Tách rõ script tạm với code production.
- Với API, ưu tiên schema rõ ràng thay vì raw dict nếu framework hỗ trợ.

## 5. Workflow bắt buộc
- Trước khi code, xác định rõ input, output, vị trí logic và rủi ro chính.
- Khi triển khai, làm theo các bước nhỏ và kiểm chứng từng phần.
- Sau khi xong, tự rà lại ảnh hưởng và cập nhật docs liên quan.

## 6. Quy tắc docs và tri thức
- Thay đổi flow, API, chức năng, policy, decision quan trọng phải kéo theo cập nhật docs liên quan.
- Nếu task chưa xong hoặc bị chặn, ghi vào `docs/TODO.md`.
- Nếu debug ra một case có khả năng lặp lại, ghi lại thành tri thức dùng lại trong docs hoặc comment phù hợp.
- Nếu dự án dùng NotebookLM, cần đồng bộ lại sau khi docs trong repo đã cập nhật.
- Nếu bắt đầu dùng `nlm` mà chưa đăng nhập hoặc session hết hạn, phải mở lại luồng đăng nhập trước khi hỏi notebook.
- Sau khi đăng nhập, phải tìm notebook liên quan của dự án bằng canonical name, alias, URL hoặc metadata có sẵn; chỉ tạo notebook mới khi đã kiểm tra mà không thấy notebook phù hợp.
- Nếu tạo notebook mới, phải upload bộ tài liệu governance tối thiểu như `AGENTS.md`, `.agent/`, `docs/`, `README.md` và cập nhật lại `Notebook ID` cùng `URL` trong `AGENTS.md`.

## 7. Rule đặc thù của dự án
- Mọi ràng buộc riêng của dự án phải ghi trong `.agent/rules/project_rules.md`.
- Nếu project rule mâu thuẫn với rule chung, project rule được ưu tiên.
