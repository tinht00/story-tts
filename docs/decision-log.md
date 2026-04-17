# Decision Log

## 2026-04-04: Chốt kiến trúc v1
- Dự án là `single-user local`.
- Metadata chuẩn dùng `SQLite`.
- Worker và direct speak cùng chạy trong backend process, không tách service riêng.
- Output chuẩn của UI là `đọc text + MP3 từng chương`.

## 2026-04-04: Chốt hướng provider
- V1 là `edge-only`.
- Vẫn giữ abstraction provider ngay từ đầu để tránh khóa chặt vào một nguồn duy nhất.

## 2026-04-04: Chuyển sản phẩm sang reader TXT local
- Bỏ Telegram khỏi luồng chính của UI và contract frontend.
- Folder picker của trình duyệt là điểm vào chuẩn cho thư viện truyện.
- Mỗi thư mục con cấp 1 là một truyện, mỗi file `.txt` trực tiếp bên trong là một chương.
- Reader progress được lưu riêng để khôi phục chapter, vị trí cuộn và vị trí audio gần nhất.

## 2026-04-04: Chốt cảm xúc v1
- Không giả lập multi-style SSML như Azure.
- Dùng `preset prosody` để đạt khác biệt nhịp kể ở mức đủ dùng cho truyện dài.

## 2026-04-07: Chuyển luồng nghe chính sang realtime service Python
- Luồng nghe chính không còn dựa vào `Go + edge-tts CLI + MP3 từng part` trong UI.
- Tách một service Python riêng dùng `FastAPI + RealtimeTTS + EdgeEngine`.
- Frontend gọi trực tiếp realtime service bằng `HTTP + WebSocket`, còn backend Go tiếp tục phục vụ thư viện, chapter content và progress.
- Audio được stream trực tiếp dưới dạng chunk `audio/mpeg`, không tạo file audio trung gian cho luồng nghe chính.
- Khi đọc xong một chapter, session realtime tự chuyển sang chapter tiếp theo cho tới hết truyện.
