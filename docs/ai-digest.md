# AI Digest

## 1. Nhận diện dự án
- Tên: `story-tts`
- Mô tả ngắn: `App local để import thư mục truyện TXT, đọc trực tiếp trong web app và phát realtime TTS theo chapter/segment.`
- Stack chính: `Go + Gin, Vue 3 + Vite, FastAPI, SQLite, edge_tts, ffmpeg`

## 2. Thành phần chính
- `Governance kit`: `AGENTS.md`, `.agent/`, `docs/`
- `NotebookLM canonical`: `story-tts - Governance & Docs`
- `backend/`: API thư viện, chapter content, progress reader, config runtime
- `frontend/`: reader 3 cột, folder picker, panel realtime, player `MediaSource`, fallback `Read Aloud` của Edge
- `tts_service/`: realtime session service dùng `FastAPI + WebSocket + edge_tts`

## 3. Điểm vào và lệnh quan trọng
- Local launcher: `.\run.ps1`
- Backend dev: `go run ./cmd/api`
- TTS service dev: `data/run/tts-venv/Scripts/python.exe -m uvicorn tts_service.app:app --host 127.0.0.1 --port 8010`
- Frontend dev: `pnpm dev` hoặc `npm run dev`
- Frontend build: `pnpm build`
- Backend build: `go build ./...`
- Docker local: `docker compose up -d --build`

## 4. Luồng chính đang chạy
- `folder picker -> import TXT -> reader -> POST /sessions -> WebSocket audio/mpeg -> progress`
- Reader và realtime panel không được nhầm giữa:
  - chapter đang hiển thị
  - chapter đang buffer ahead
  - chapter audio đang phát thực tế
- Seek ưu tiên tái dùng session hiện tại; chỉ restart playback khi seek hiện tại bị kẹt hoặc không còn reuse được cache.

## 5. Ràng buộc ổn định cần nhớ
- Source code là nguồn sự thật cho runtime.
- Luồng nghe chính đã chốt là `frontend -> tts_service -> WebSocket -> MediaSource`.
- Provider thực tế của đường chính hiện tại là `edge_tts` Python API, không phải pipeline MP3 legacy của backend Go.
- Pipeline ahead hiện tại là `15 / 10 / 10`.
- Segment hiện tại render lỗi thì phải retry; không tự bỏ qua để giữ tuần tự nghe.

## 6. Tài liệu chuẩn nên đọc trước
- `AGENTS.md`
- `README.md`
- `docs/architecture-v1.md`
- `docs/realtime-runtime.md`
- `docs/decision-log.md`
- `docs/TODO.md`
- `.agent/context/architecture.md`
- `.agent/rules/project_rules.md`

## 7. Ghi chú vận hành
- Notebook canonical đã tồn tại với ID `04b036e8-540b-43f7-999a-3d3e1dd8a747`.
- `run.ps1` và `.codex/environments/setup.ps1` đã fallback sang binary chuẩn của Go và Node trên Windows để giảm lỗi do `PATH` cũ.
- `frontend/vite.config.ts` có middleware fallback cho `/@vite/client` và `/@vite/env`.
- `frontend/src/App.vue` đã vá case `synth xong nhưng không phát audio` bằng cách:
  - gắn `MediaSource` vào `<audio>` trước khi chờ `sourceopen`
  - prime playback sớm hơn
  - tránh mất `user gesture` ở lượt phát đầu
- `tts_service/app.py` có cache render theo chapter/segment và hỗ trợ `fast seek cache hit`.
- Khi cần debug realtime, ưu tiên xác nhận WebSocket có binary MP3 trước khi đổ lỗi cho service.

## 8. Những phần còn legacy
- Direct TTS cũ trong backend Go vẫn còn trong codebase để tránh refactor quá gấp.
- Telegram tables và một số thành phần legacy vẫn còn trong schema/code nhưng không còn thuộc đường chính của UI.
