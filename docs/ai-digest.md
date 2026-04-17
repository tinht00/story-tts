# AI Digest

## 1. Nhận diện dự án
- Tên: `story-tts`
- Mô tả ngắn: `App local để import thư mục truyện TXT, đọc trực tiếp trong web app và phát TTS theo từng chương.`
- Stack chính: `Go + Gin, Vue 3 + Vite, FastAPI, ffmpeg, edge-tts`

## 2. Thành phần chính
- `Governance kit`: `AGENTS.md`, `.agent/`, `docs/` cho điều hướng và tri thức dự án
- `NotebookLM canonical`: notebook `story-tts - Governance & Docs` dùng cho research/governance lặp lại
- `backend/`: API `Gin`, `SQLite`, parser/chunker TXT, direct TTS theo chương và lưu progress reader
- `frontend/`: UI reader 3 cột, folder picker, danh sách truyện/chương, audio player và khôi phục tiến độ

## 3. Lệnh và điểm vào quan trọng
- Local launcher: `.\run.ps1`
- Backend dev: `go run ./cmd/api`
- TTS service dev: `data/run/tts-venv/Scripts/python.exe -m uvicorn tts_service.app:app --host 127.0.0.1 --port 8010`
- Backend build: `go build ./...`
- Frontend dev: `npm run dev`
- Frontend build: `npm run build`
- Docker local: `docker compose up -d --build`
- Backend entrypoint: `backend/cmd/api/main.go`
- Frontend entrypoint: `frontend/src/main.ts`
- Environment setup: `.codex/environments/setup.ps1`

## 4. Ràng buộc ổn định cần nhớ
- Không được coi notebook hay chat là nguồn sự thật cho runtime; khi có code, source code là chuẩn.
- V1 đã khóa hướng sản phẩm: `API + web app`, `edge-only`, `1 thư mục gốc nhiều thư mục truyện`, `đọc text + audio cache từng chương`.

## 5. Tài liệu và nguồn tra cứu chuẩn
- `AGENTS.md`
- `.agent/project_settings.md`
- `.agent/rules.md`
- `.agent/rules/project_rules.md`
- `.agent/context/architecture.md`
- `docs/TODO.md`
- Notebook: `story-tts - Governance & Docs`
- Repo tham chiếu: `D:\Tinht00_Workspace\Projects\Andon\andon-tts-web-api`

## 6. Ghi chú vận hành
- Notebook canonical đã tồn tại thật với ID `04b036e8-540b-43f7-999a-3d3e1dd8a747`.
- `ffmpeg` cần có trên PATH nếu chapter dài bị chia nhiều segment và cần merge lại thành một file MP3.
- `tts_service` ưu tiên binary `edge-tts` trong `data/run/tts-venv`, nên worktree mới nên chạy `.codex/environments/setup.ps1` trước khi verify end-to-end audio.
- Đây là dự án độc lập; các tham chiếu tới `andon-tts-web-api` chỉ nhằm tái sử dụng pattern kỹ thuật.
