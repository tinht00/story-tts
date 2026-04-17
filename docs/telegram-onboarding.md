# Telegram onboarding v1

## Mục tiêu
Cho phép `story-tts` dùng tài khoản Telegram thật của người dùng để nói chuyện với bot bên thứ ba và tải TXT về local library.

## Cách đăng nhập hiện tại
1. Tạo `app_id` và `app_hash` tại `https://my.telegram.org`.
2. Điền các biến `STORY_TTS_TELEGRAM_APP_ID` và `STORY_TTS_TELEGRAM_APP_HASH`.
3. Chọn một trong hai luồng:
   - Luồng OTP: gọi `POST /api/telegram/send-code`, sau đó `POST /api/telegram/sign-in`, và nếu cần thì gọi thêm `POST /api/telegram/password`.
   - Luồng QR: gọi `POST /api/telegram/qr/start`, lấy `qrCodeDataUrl` hoặc `loginUrl` để hiển thị, rồi poll `GET /api/state` hoặc `GET /api/telegram/qr` cho tới khi `status = authenticated`.
4. Nếu muốn hủy phiên QR đang chờ, gọi `POST /api/telegram/qr/cancel`.
5. Session được lưu ở `data/telegram/session.json`.

## Trạng thái QR login
- `starting`: backend vừa mở phiên mới.
- `pending`: đã có mã QR và đang chờ Telegram app quét.
- `authenticated`: đăng nhập thành công, session đã lưu.
- `cancelled`: người dùng hủy phiên đang chờ.
- `error`: backend hoặc Telegram trả lỗi; xem `lastError`.

## Bot profile
`telegram_bot_profiles` giữ cấu hình bot ngoài để tránh hard-code flow vào lõi:
- `bot_username`
- `search_template`
- `chapter_template`
- `document_rule`
- `story_title_rule`
- `chapter_title_rule`

## Trạng thái hiện tại
- Đã có lớp đăng nhập MTProto và lưu bot profile.
- Chưa có bước chat/download TXT tự động từ bot ngoài.
- Khi làm bước import bot thật, phải giữ nguyên contract: file tải xong luôn được đưa vào `library/<story_slug>/source/chapters/`.
