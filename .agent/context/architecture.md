# Architecture Context

## 1. Mục tiêu hệ thống
- Xây dựng reader local cho truyện TXT theo chapter, có thể đọc trực tiếp trong web app và phát `realtime TTS` liền mạch theo chapter/segment.

## 2. Thành phần chính
- `Frontend/Web`: `Vue 3 + Vite` cho folder picker, reader 3 cột, realtime panel, player `MediaSource`, fallback `Read Aloud` của Edge.
- `Backend/API`: `Go + Gin + SQLite` cho import thư viện, chapter content, progress và config runtime.
- `Realtime TTS service`: `FastAPI + WebSocket + edge_tts` cho session realtime, render ahead, seek và stream `audio/mpeg`.
- `Storage`: `library/` cho chapter TXT chuẩn hóa, `data/` cho SQLite, venv và dữ liệu vận hành.

## 3. Luồng dữ liệu mức cao
1. Người dùng chọn thư mục gốc chứa nhiều thư mục truyện.
2. Frontend đọc manifest, gửi nội dung import sang backend.
3. Backend chuẩn hóa, lưu metadata và chapter content vào `library/` cùng SQLite.
4. Reader tải chapter content và cho phép bấm `Phát`.
5. Frontend tạo realtime session ở `tts_service`.
6. Service chia segment, render ahead bằng `edge_tts`, stream MP3 bytes qua WebSocket.
7. Frontend append bytes vào `MediaSource`, đồng bộ trạng thái chapter/segment và lưu progress theo chapter audio thực tế.

## 4. Ràng buộc quan trọng
- Source code là nguồn sự thật cho runtime; docs chỉ là lớp điều hướng.
- Đường chính hiện tại là `frontend -> realtime TTS service -> WebSocket -> MediaSource`.
- Reader phải tách rõ `chapter đang xem`, `chapter đang preload`, `chapter audio đang phát thực tế`.
- Seek phải ưu tiên tái dùng session/cache hiện có; chỉ restart playback khi thật sự cần.
- Telegram và direct TTS legacy không còn là luồng chính của UI.

## 5. Tài liệu liên quan
- `README.md`
- `AGENTS.md`
- `docs/architecture-v1.md`
- `docs/realtime-runtime.md`
- `docs/decision-log.md`
- `docs/TODO.md`
