# Project-Specific Rules

## 1. Tổng quan
- Tên dự án: `story-tts`
- Domain nghiệp vụ: `thư viện TTS đọc truyện dài từ file TXT theo chương`
- Stack chính: `Go + Gin, Vue 3 + Vite, ffmpeg, edge-tts compatible providers`

## 2. Cấu trúc thư mục quan trọng
- `AGENTS.md`: router gốc và mapping NotebookLM canonical của dự án
- `.agent/`: governance, rules, workflow phát triển
- `docs/`: digest, TODO, các tài liệu API/flow/decision của dự án
- `backend/`: API, job orchestration, parser TXT, provider edge, Telegram session và audio pipeline
- `frontend/`: UI quản lý thư viện truyện, job synthesize, Telegram auth và preview nghe thử

## 3. Quy ước kỹ thuật bắt buộc
- Không mô tả nhầm định hướng thành implementation thật khi repo chưa có code.
- V1 ưu tiên `edge-only`, nhưng phải giữ abstraction provider để sau này thêm nguồn TTS khác mà không phá contract nội bộ.
- Input chuẩn của thư viện là `1 thư mục truyện` chứa nhiều file TXT theo chương; tên file và metadata phải đủ ổn định để rerun theo chương.
- Output chuẩn phải có `audio từng chương` và `audio gộp full book`; pipeline merge audio phải tách biệt khỏi bước synthesize từng chunk/chương.
- Bài toán chính là đọc truyện dài, nên thiết kế phải có chunking an toàn, retry từng phần và trạng thái job rõ ràng.

## 4. Quy ước môi trường
- Cổng local: backend `18080`, frontend `5174`
- Env quan trọng: dự kiến gồm biến cho provider TTS, thư mục input TXT, thư mục output audio và binary `ffmpeg`
- Service phụ thuộc: `NotebookLM`, provider TTS, `ffmpeg`; database/queue sẽ chốt khi có implementation thật

## 5. Quy ước deploy và vận hành
- Chưa chốt deploy thật ở thời điểm bootstrap.
- Khi triển khai thực tế, phải ghi rõ app chạy bằng Docker hay bare-metal, thư mục mount nào chứa TXT/audio và credential provider nằm ở đâu.
- Nếu dùng provider trả phí hoặc key nhạy cảm, secret chỉ đi qua env hoặc mount ngoài repo.
- Khi có CI/CD hoặc compose file, phải cập nhật lại file này để làm nguồn chuẩn.

## 6. Điều cấm hoặc cần lưu ý
- Không commit credential hoặc cookie NotebookLM/TTS vào repo.
- Không dùng NotebookLM để suy diễn runtime nếu source code hoặc config thực tế nói khác.
- Không để TODO triển khai lớn chỉ nằm trong chat; phải ghi lại trong `docs/TODO.md`.
