# story-tts - Agent Map

## Mục đích
File này là điểm điều hướng tối thiểu cho agent và lập trình viên khi làm việc trong workspace `story-tts`.

## NotebookLM Canonical
- Tên notebook chuẩn: `story-tts - Governance & Docs`
- Alias CLI: `story-tts`
- Notebook ID: `04b036e8-540b-43f7-999a-3d3e1dd8a747`
- URL: `https://notebooklm.google.com/notebook/04b036e8-540b-43f7-999a-3d3e1dd8a747`

## Quy ước vận hành NotebookLM và `nlm`
- Khi bắt đầu dùng `nlm` hoặc NotebookLM cho dự án này, phải kiểm tra tình trạng đăng nhập trước. Nếu chưa đăng nhập hoặc session hết hạn, phải mở lại luồng đăng nhập và xác nhận truy cập thành công rồi mới tiếp tục.
- Sau khi xác thực xong, phải tìm notebook liên quan của dự án theo canonical name, alias, URL cũ hoặc metadata tương ứng trước khi tạo mới.
- Nếu tìm thấy notebook phù hợp mà `Notebook ID` trong file này còn trống hoặc placeholder, phải cập nhật lại ID mặc định và URL của notebook đó vào `AGENTS.md`.
- Nếu chưa có notebook phù hợp, phải tự tạo notebook canonical theo mẫu `story-tts - Governance & Docs`, sau đó cập nhật ngay `Notebook ID` và `URL` vào file này.
- Sau khi tạo notebook mới, phải ưu tiên đưa lên các tài liệu điều hướng và governance cốt lõi như `AGENTS.md`, `.agent/`, `docs/`, `README.md` và các flow/spec/decision log quan trọng nếu có.
- Nếu một tài liệu chuẩn chưa sẵn sàng để upload ngay, cần ghi lại ngắn gọn trong `docs/TODO.md` hoặc tài liệu theo dõi tương đương để đồng bộ bổ sung.

## Phạm vi sử dụng NotebookLM
- Dùng để tra cứu governance, kiến trúc, workflow, decision log, policy, phân tích chức năng và tài liệu nội bộ cần dùng lặp lại.
- Ưu tiên notebook khi cần đọc lại nhiều tài liệu Markdown hoặc đối chiếu nhiều văn bản nội bộ.
- Không dùng notebook để thay thế source code hiện tại. Với hành vi runtime và implementation đang chạy, repo vẫn là nguồn sự thật.

## Tài nguyên chuẩn nên đưa lên NotebookLM
- `README.md`
- `AGENTS.md`
- toàn bộ `.agent/` cần thiết cho governance và workflow
- `.agent/project_settings.md`
- `.agent/rules.md`
- `.agent/rules/project_rules.md`
- `.agent/context/architecture.md`
- `docs/ai-digest.md`
- các tài liệu quan trọng trong `docs/`
- các flow/spec/decision log quan trọng của dự án

## Quy ước cập nhật
- Khi có thêm, sửa, xoá chức năng hoặc thay đổi flow, logic nghiệp vụ, kiến trúc, API contract hay policy quan trọng, bắt buộc cập nhật tài liệu liên quan trong repo trước khi kết thúc công việc.
- Tối thiểu phải rà soát các điểm trực tiếp liên quan: `AGENTS.md`, `.agent/`, `README.md`, `docs/`, flow/spec, decision log, comment nội bộ liên quan.
- Sau khi cập nhật docs trong repo, cần đồng bộ lại notebook canonical nếu dự án đang dùng NotebookLM.
- Nếu dự án chưa có notebook thật, vẫn phải giữ canonical name theo mẫu `story-tts - Governance & Docs` trong file này.
- Nếu đã xác minh được notebook thật của dự án, không để `Notebook ID` ở placeholder hoặc bỏ trống quá lâu.
- Nếu chốt được một case lỗi có giá trị tái sử dụng, phải ghi lại ngắn gọn: triệu chứng, nguyên nhân gốc, cách xác minh và cách sửa.
- Nếu có hạng mục chưa làm xong, bị chặn, hoặc chưa verify hết, phải ghi vào `docs/TODO.md`.
- Khi một mục TODO đã hoàn tất và đã được kiểm tra xong, phải xoá mục đó khỏi `docs/TODO.md`.

## Nguồn sự thật
- Source code hiện tại là nguồn sự thật cho hành vi runtime.
- Docs và NotebookLM là lớp tri thức hỗ trợ tra cứu, không được dùng để đoán ngược implementation nếu source code đang nói khác.

## Ràng buộc đặc thù của dự án
- Điền các constraint thật của dự án vào `.agent/rules/project_rules.md`.
- Nếu dự án có yêu cầu môi trường đặc biệt như DB version cũ, NAS mount, message broker, rule deploy riêng, ghi lại rõ ở đó và tóm tắt lại tại đây nếu cần.
- Trạng thái hiện tại của repo đã có source code nền cho:
  - backend `Go + Gin + SQLite` cho import thư viện, chapter content và progress
  - service `Python + FastAPI + edge_tts` cho luồng nghe realtime
  - frontend `Vue 3 + Vite` theo mô hình reader 3 cột
  - docs v1 cho kiến trúc reader TXT local, realtime TTS và decision log
- Luồng chính hiện tại là `folder picker -> import TXT -> đọc chương -> realtime TTS -> lưu tiến độ`.
- Telegram không còn nằm trong luồng chính của sản phẩm; phần code cũ chỉ được giữ tạm ở backend để tránh refactor phá vỡ các thành phần phụ.
- Dự án này được định hướng kế thừa cấu trúc từ `andon-tts-web-api`, nhưng đã chốt trọng tâm ở bài toán thư viện đọc truyện TXT theo chương và stream audio realtime.
