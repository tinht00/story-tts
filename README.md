# story-tts

Project local để quản lý thư mục truyện TXT, mở chương để đọc trực tiếp trong web app và phát `realtime TTS` theo từng chương mà không cần tạo file audio trung gian.

## Trạng thái hiện tại
- Đã scaffold backend `Go + Gin + SQLite`.
- Đã scaffold frontend `Vue 3 + Vite`.
- Đã có notebook canonical: [story-tts - Governance & Docs](https://notebooklm.google.com/notebook/04b036e8-540b-43f7-999a-3d3e1dd8a747).
- Đây là dự án độc lập, không thuộc domain `Andon`; `andon-tts-web-api` chỉ là repo tham chiếu kỹ thuật.
- Luồng chính hiện tại là `folder picker -> import TXT -> đọc chương -> realtime TTS -> lưu tiến độ`.

## Định hướng v1
- Dạng project: `API + web app`
- Repo tham chiếu kỹ thuật: `D:\Tinht00_Workspace\Projects\Andon\andon-tts-web-api`
- Provider ưu tiên hiện tại: `RealtimeTTS + EdgeEngine`
- Input chuẩn: `1 thư mục gốc`, mỗi thư mục con là một truyện, các file `.txt` bên trong là các chương
- Output chuẩn: `đọc text trong app + stream audio realtime + tự chuyển chương`

## Stack hiện tại
- `Go + Gin` cho backend import thư viện, chapter content và reader progress
- `Vue 3 + Vite` cho frontend reader 3 cột, voice control và realtime player
- `FastAPI + edge_tts` cho service Python stream audio qua WebSocket
- `NotebookLM` cho governance, research và tri thức dự án

## Cấu trúc hiện tại
- `backend/`: API reader/import, storage SQLite, parser TXT, progress và phần TTS cũ ở trạng thái legacy
- `frontend/`: giao diện local reader với folder picker, danh sách truyện/chương, voice control và realtime stream player
- `tts_service/`: service Python realtime TTS, hiện runtime dùng `edge_tts.Communicate` để render MP3 rồi stream qua WebSocket
- `docs/architecture-v1.md`: kiến trúc v1
- `docs/prosody-presets.md`: preset prosody
- `docs/decision-log.md`: decision log của dự án

## Chạy local
Đã kiểm chứng trên máy hiện tại ngày `2026-04-08` với:
- `Go 1.23.4`
- `Node.js v22.12.0`, `npm 11.0.0`
- `Python 3.13.1`
- `Docker 27.4.0`, `Docker Compose v2.31.0`

### Môi trường Codex
- File môi trường local: `.codex/environments/environment.toml`
- Script setup: `.codex/environments/setup.ps1`
- Script này sẽ:
  - tạo `backend/.env` từ `.env.example` nếu còn thiếu
  - tạo venv tại `data/run/tts-venv`
  - cài dependency cho `tts_service`
  - cài dependency frontend nếu `frontend/node_modules` chưa có
  - cập nhật `STORY_TTS_EDGE_BINARY` trong `backend/.env` cho backend legacy hoặc các bài test cần gọi `edge-tts` binary

### Backend
```powershell
cd D:\Tinht00_Workspace\VibeCode\story-tts\backend
# Chỉ cần chạy dòng dưới nếu file .env chưa tồn tại.
Copy-Item .env.example .env
go run ./cmd/api
```

### Chạy local bằng một lệnh
```powershell
cd D:\Tinht00_Workspace\VibeCode\story-tts
.\run.ps1
```

Script này sẽ:
- in ra URL và cổng hiện tại của frontend, backend và realtime TTS
- tự copy `backend/.env` từ `.env.example` nếu file chưa có
- mở 3 cửa sổ PowerShell riêng để chạy `tts_service`, `backend` và `frontend`

Nếu chỉ muốn kiểm tra cấu hình cổng mà chưa mở terminal mới:
```powershell
.\run.ps1 -DryRun
```

### Realtime TTS Service
```powershell
cd D:\Tinht00_Workspace\VibeCode\story-tts
D:\Tinht00_Workspace\VibeCode\story-tts\data\run\tts-venv\Scripts\python.exe -m uvicorn tts_service.app:app --host 127.0.0.1 --port 8010
```

### Frontend
```powershell
cd D:\Tinht00_Workspace\VibeCode\story-tts\frontend
npm install
npm run dev
```

Frontend dev bằng Vite sẽ chạy ở `http://127.0.0.1:5174`.

### Thứ tự nên chạy
1. Mở terminal 1 chạy `tts_service`.
2. Mở terminal 2 chạy `backend`.
3. Mở terminal 3 chạy `frontend`.
4. Mở `http://127.0.0.1:5174`.

## Chạy bằng Docker local
```powershell
cd D:\Tinht00_Workspace\VibeCode\story-tts
docker compose up -d --build
```

Mặc định Docker sẽ mở:
- Frontend static: `http://127.0.0.1:4173`
- Backend health: `http://127.0.0.1:18080/health`
- Realtime TTS service: `http://127.0.0.1:8010/health`

Các volume local đang được mount:
- `./data -> /app/data`
- `./library -> /app/library`

Lệnh hữu ích:
```powershell
docker compose logs -f
docker compose ps
docker compose down
```

## Lưu ý vận hành
- Backend Go mặc định chạy ở `:18080`, realtime TTS service chạy ở `http://127.0.0.1:8010`, frontend dev mặc định ở `:5174`.
- `go run ./cmd/api` có thể mất vài giây đầu để build rồi mới bind cổng `18080`; nếu vừa chạy xong mà probe chưa thấy cổng mở ngay thì chờ thêm một chút.
- `tts_service` hiện render audio trực tiếp bằng thư viện `edge_tts` của Python và có endpoint `GET /health` để `run.ps1` kiểm tra service đã sẵn sàng.
- Playback realtime luôn đi đúng tuần tự segment; nếu segment hiện tại render lỗi thì service sẽ retry với backoff cho tới khi thành công hoặc người dùng dừng session, không tự bỏ qua segment lỗi.
- Khi chạy Docker, frontend dùng Nginx static container ở `:4173` và reverse proxy `/api`, `/health`, `/library` sang backend container.
- Tách Docker frontend khỏi `:5174` để tránh mở nhầm bundle cũ khi đang phát triển bằng `npm run dev`.
- Trong Docker local, backend trả `realtimeTtsBaseUrl=http://localhost:8010` để browser trên máy host kết nối trực tiếp sang realtime TTS service.
- Dữ liệu runtime mặc định nằm tại `data/` và thư viện truyện nằm tại `library/`.
- Trên `Chrome/Edge`, app ưu tiên `showDirectoryPicker` để nhớ quyền truy cập thư mục; khi đó nút `Làm mới thư viện` sẽ quét lại đúng thư mục cũ và nhận chương mới vừa thêm.
- Khi import từ folder picker, backend lưu `sourcePath` theo dạng `thu_muc_cha/truyen`, để `Làm mới thư viện` luôn bám theo đúng thư mục cha chứa các truyện con.
- Nếu trình duyệt chỉ hỗ trợ fallback `webkitdirectory`, cần chọn lại thư mục khi muốn nạp thay đổi từ ổ đĩa.
- Mỗi truyện hiển thị dưới dạng card gọn, có tổng số chương, tiến độ chương đang đọc và nút `Đọc tiếp`.
- Reader sẽ tự format nội dung theo nhịp khoảng `3-4 câu/đoạn` để dễ đọc hơn.
- Reader dùng chữ không chân, cho phép tăng giảm cỡ chữ ngay trong màn hình đọc và giữ lại lựa chọn cỡ chữ ở local.
- Frontend gọi trực tiếp realtime service bằng `HTTP + WebSocket`, nhận chunk `audio/mpeg` và phát liền mạch qua `MediaSource`.
- UI không tự nhảy chapter chỉ vì backend render ahead. Reader chỉ tự chuyển chapter khi `audio đang phát thực tế` đã sang chapter mới.
- Nút `Trước/Sau` khi realtime đang chạy chỉ đổi chapter hiển thị trong reader, không restart session realtime và không làm render lại pipeline hiện tại.
- Khi đang xem một chapter khác với chapter audio hiện tại, nút `Phát` trong pane `Đọc chữ` sẽ chuyển playback sang chapter đang mở thay vì resume chapter cũ.
- Panel realtime của chapter đang mở hiển thị toàn bộ các segment đã được nạp/render tính từ đoạn bắt đầu đọc, để theo dõi rõ audio đã tạo tới đâu và đang phát tới đâu.
- Panel realtime được nhóm theo từng chapter để nhìn rõ đã nạp/render tới chapter nào, chapter nào đang phát và chapter nào đã hoàn tất.
- Mỗi segment có progress bar riêng cho `tạo audio` và `đang đọc`, giúp nhìn rõ voice đang đọc tới đâu và audio đã render tới đâu trong chapter hiện tại.
- Có thể click trực tiếp vào bất kỳ segment nào trong panel để nhảy tới đúng chapter/segment; nếu audio của segment đó đã render xong thì frontend/service sẽ ưu tiên tái dùng cache hiện có thay vì render lại từ đầu.
- Khi click một segment chưa `ready`, frontend vẫn giữ nguyên session realtime hiện tại và gọi `seek` trên session đó, để service tận dụng cache/render state đang có thay vì dừng session rồi tạo session mới từ đầu.
- Khi người dùng click sang segment/chapter khác, reader sẽ giữ chapter vừa chọn trong lúc seek đang chờ; watcher auto-sync không được kéo UI quay về chapter cũ trước khi audio thật sự bắt đầu ở mục tiêu mới.
- Frontend tách riêng `chapter đang buffer ahead` khỏi `chapter audio đang phát thực tế`; progress được lưu theo chapter audio thực tế, không còn bám nhầm vào chapter đang hiển thị hoặc chapter backend mới bắt đầu nạp trước.
- Service realtime dùng cơ chế `15 / 10 / 10`: khởi tạo trước `15` segment tính từ vị trí bắt đầu đọc; khi phần ahead còn khoảng `10` segment thì nạp tiếp `10` segment kế tiếp.
- Có nút `Đọc từ bôi chọn` trong reader để lấy vùng người dùng đang chọn, map về segment tương ứng và tiếp tục đọc từ đó.
- Highlight trong reader chuyển sang mức `từng chữ` dựa trên segment đang phát và tiến độ audio ước lượng, thay vì tô cả đoạn/block như trước.
- Khi đọc xong một chương, session realtime sẽ tự chuyển sang chương kế tiếp cho tới hết truyện.
- Người dùng có thể chọn giọng, chỉnh tốc độ và cao độ trước khi bắt đầu phiên đọc; mặc định hiện tại là `vi-VN-NamMinhNeural`.
- Khi bật `Read Aloud` của Edge, app sẽ quét toàn bộ khối chữ của chương hiện tại trước khi gửi phím tắt và có nút `Quét khối chữ` để chọn lại vùng đọc nếu cần.
- `STORY_TTS_REALTIME_TTS_BASE_URL` phải trỏ đúng về service Python nếu đổi port hoặc host.
- Với Windows vừa mới cài hoặc nâng cấp runtime, `.\run.ps1` sẽ ưu tiên gọi trực tiếp binary chuẩn như `C:\Program Files\Go\bin\go.exe` và `C:\Program Files\nodejs\npm.cmd` để tránh lỗi terminal/action còn giữ `PATH` cũ.
- Nếu Vite dev server trên máy Windows trả lỗi với `/@vite/client`, frontend đã có fallback middleware trong `vite.config.ts` để phục vụ trực tiếp `@vite/client` và `@vite/env`, tránh lỗi 404 làm hỏng trang dev.

## Tài liệu điều hướng
- [AGENTS.md](D:/Tinht00_Workspace/Projects/story-tts/AGENTS.md)
- [.agent/HOW_TO_USE.md](D:/Tinht00_Workspace/Projects/story-tts/.agent/HOW_TO_USE.md)
- [.agent/project_settings.md](D:/Tinht00_Workspace/Projects/story-tts/.agent/project_settings.md)
- [.agent/rules/project_rules.md](D:/Tinht00_Workspace/Projects/story-tts/.agent/rules/project_rules.md)
- [docs/ai-digest.md](D:/Tinht00_Workspace/Projects/story-tts/docs/ai-digest.md)
- [docs/TODO.md](D:/Tinht00_Workspace/Projects/story-tts/docs/TODO.md)
