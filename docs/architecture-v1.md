# Kiến trúc v1

## Mục tiêu
- Đọc truyện dài từ nhiều file TXT theo chương.
- Mở nội dung chương trực tiếp trong web app.
- Stream `audio realtime` theo chương mà không cần tạo file audio trung gian.
- Tự chuyển sang chương kế tiếp sau khi đọc xong chương hiện tại.
- Chạy local cho một người dùng với `Go backend + Python realtime TTS service`.

## Luồng chính
1. Người dùng chọn một thư mục gốc bằng folder picker ở frontend.
   - Trên `Chrome/Edge`, frontend lưu `directory handle` để có thể quét lại cùng thư mục khi bấm `Làm mới thư viện`.
2. Frontend nhóm file `.txt` theo cấu trúc `goc/truyen/chapter.txt`, rồi gửi manifest + content sang backend.
3. Backend chuẩn hóa text, copy chapter vào `library/<story_slug>/source/chapters/`, đồng thời lưu `sourcePath` theo dạng `goc/truyen` để refresh luôn bám theo đúng thư mục cha đã chọn.
4. Frontend tải danh sách truyện/chương từ backend để mở reader.
5. Reader format lại text để hiển thị theo nhịp khoảng `3-4 câu/đoạn`, tách heading và bỏ các dòng phân cách dài.
6. Khi người dùng bấm nghe, frontend lấy content của các chapter trong truyện và gửi sang `tts_service` bằng `POST /sessions`.
7. Service Python dùng `RealtimeTTS + EdgeEngine` để phát sinh audio chunk realtime, rồi stream chunk `audio/mpeg` qua `WebSocket`.
8. Frontend nhận audio chunk, append trực tiếp vào `MediaSource`, cập nhật trạng thái `đang kết nối / đang đọc / đang chuyển chương / đã dừng / lỗi`.
9. Khi service phát xong một chapter, session sẽ tự chuyển sang chapter kế tiếp và frontend đồng bộ chapter đang hiển thị theo event realtime.
10. Frontend lưu tiến độ đọc gần nhất vào `SQLite` qua backend Go.

## Thành phần backend
- `internal/api`: HTTP API Go cho UI và automation local.
- `internal/service`: điều phối import thư viện, chapter content và reader progress.
- `internal/storage`: schema `SQLite` và repository.
- `internal/library`: parser TXT và slug/path chuẩn.
- `tts_service/app.py`: service Python realtime TTS qua `FastAPI + WebSocket`.
- `internal/telegram`: giữ lại tạm thời ở codebase, không còn nằm trong luồng chính của UI.

## Metadata cốt lõi
- `stories`: thông tin truyện, library path, source path, preset mặc định, lần mở gần nhất.
- `chapters`: thứ tự chương, file TXT nguồn, normalized text, checksum, preset.
- `reader_progress`: chapter đang đọc, vị trí cuộn và vị trí audio gần nhất.
- `recent_stories`: truyện đã mở gần đây để ưu tiên restore UI.
- `telegram_accounts` và `telegram_bot_profiles`: bảng cũ còn được giữ trong schema để tránh refactor phá vỡ, nhưng không còn thuộc luồng chính của sản phẩm.

## Ranh giới v1
- Chỉ hỗ trợ cấu trúc `thư mục gốc/truyện/*.txt`, không quét thư mục lồng nhiều tầng.
- Folder picker tối ưu cho `Chrome/Edge`, chưa tối ưu cho Firefox.
- UI reader không còn dùng Telegram, bot profile hay jobs.
- Cảm xúc hiện tại chủ yếu điều khiển qua `voice + speed + pitch`; `preset prosody` chỉ còn nằm ở phía reader/legacy.
- Luồng `Go + edge-tts + file MP3` vẫn còn trong codebase để giữ tương thích tạm thời, nhưng không còn là đường chính của UI.
