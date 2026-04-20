# Project Settings

## Tóm tắt dự án
- Tên dự án: `story-tts`
- Mô tả ngắn: `Thư viện local để import truyện TXT theo chapter, đọc trực tiếp trong web app và phát realtime TTS theo chapter/segment.`
- Stack chính: `Go + Gin, Vue 3 + Vite, FastAPI, SQLite, edge_tts, ffmpeg`
- Repo hoặc module chính: `D:\Tinht00_Workspace\VibeCode\story-tts`

## Quy ước làm việc
- Ưu tiên dùng source code để xác minh implementation hiện tại.
- Nếu task nhỏ và chỉ chạm một vài file, bỏ qua NotebookLM và chỉ mở file cần thiết.
- Nếu task liên quan tới governance, workflow, decision log hoặc nhiều tài liệu nội bộ, ưu tiên dùng NotebookLM canonical của dự án.
- Khi có thay đổi về flow hoặc chức năng, cập nhật docs liên quan ngay trong cùng phiên làm việc.
- Khi dự án chưa có code, không giả định structure runtime; phải ghi rõ đó là định hướng hoặc TODO thiết kế.
- `andon-tts-web-api` chỉ là repo tham chiếu kỹ thuật, không phải cùng domain nghiệp vụ.

## Quy ước giao tiếp
- Phản hồi bằng tiếng Việt có dấu.
- Nêu giả định quan trọng một cách ngắn gọn, rõ ràng.
- Nếu có blocker từ môi trường, nêu rõ blocker, tác động và cách verify sau khi gỡ.

## Quy ước kiểm tra
- Luôn chạy mức verify phù hợp với phạm vi thay đổi nếu môi trường cho phép.
- Nếu không chạy được test/build, phải nói rõ lý do.
- Không kết luận hoàn tất khi còn mục dang dở mà chưa ghi lại vào `docs/TODO.md`.

## Checklist sau khi copy starter
- Thay placeholder trong file này
- Điền `AGENTS.md`
- Điền `.agent/rules/project_rules.md`
- Tạo notebook canonical hoặc ghi sẵn canonical name
- Điền `docs/ai-digest.md` với tri thức ban đầu của dự án
- Scaffold source code tối thiểu cho `backend/` và `frontend/` khi bắt đầu triển khai thực tế
