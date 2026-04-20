# Project-Specific Rules

## 1. Tổng quan
- Tên dự án: `story-tts`
- Domain nghiệp vụ: `thư viện local để đọc truyện TXT theo chapter và phát realtime TTS`
- Stack chính: `Go + Gin, Vue 3 + Vite, FastAPI, SQLite, edge_tts, ffmpeg`

## 2. Cấu trúc thư mục quan trọng
- `AGENTS.md`: router gốc và mapping NotebookLM canonical
- `.agent/`: governance, rules, workflow phát triển
- `docs/`: digest, architecture, decision log, runtime cases, TODO
- `backend/`: API import thư viện, content, progress và phần legacy còn tạm giữ
- `frontend/`: UI reader, realtime panel, MediaSource player, Edge Read Aloud fallback
- `tts_service/`: realtime session service dùng `edge_tts`

## 3. Quy ước kỹ thuật bắt buộc
- Source code hiện tại là nguồn sự thật cho runtime.
- Luồng chính phải được mô tả là `folder picker -> import TXT -> reader -> realtime TTS -> progress`.
- Không mô tả nhầm đường chính thành pipeline MP3 legacy của backend Go.
- Provider thực tế của đường chính hiện tại là `edge_tts` Python API qua `tts_service`.
- Frontend phải phân biệt rõ `chapter đang xem`, `chapter đang preload`, `chapter audio đang phát thực tế`.
- Seek phải ưu tiên tái dùng session/cache hiện có; chỉ restart playback khi seek hiện tại không còn đi đúng mục tiêu.
- Với lỗi render segment, worker không được tự bỏ qua segment hiện tại nếu chưa có lệnh dừng/seek mới.

## 4. Quy ước môi trường
- Cổng local mặc định:
  - backend `18080`
  - realtime TTS service `8010`
  - frontend dev `5174`
- Env quan trọng:
  - `STORY_TTS_REALTIME_TTS_BASE_URL`
  - binary `ffmpeg`
  - Python venv tại `data/run/tts-venv`

## 5. Quy ước deploy và vận hành
- `run.ps1` là điểm vào local mặc định.
- Khi runtime hoặc luồng chính thay đổi, phải cập nhật tối thiểu:
  - `README.md`
  - `docs/architecture-v1.md`
  - `docs/realtime-runtime.md`
  - `docs/decision-log.md`
- Nếu một case lỗi có giá trị tái sử dụng đã được chốt nguyên nhân gốc và cách sửa, phải ghi lại vào `docs/realtime-runtime.md`.

## 6. Điều cấm hoặc cần lưu ý
- Không commit credential, cookie NotebookLM hoặc secret khác vào repo.
- Không dùng NotebookLM để suy diễn runtime nếu source code hoặc log thực tế nói khác.
- Không để TODO lớn chỉ nằm trong chat; các mục chưa xong thật sự phải có trong `docs/TODO.md`.
