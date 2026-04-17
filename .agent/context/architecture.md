# Architecture Context

## 1. Mục tiêu hệ thống
- Xây dựng một hệ thống quản lý thư viện truyện TXT và pipeline TTS để sinh audio từng chương cùng audio gộp full book, bắt đầu từ định hướng kế thừa `andon-tts-web-api`.

## 2. Thành phần chính
- `Frontend/Web`: UI `Vue 3 + Vite` để quét truyện local, cấu hình Telegram, lưu bot profile, theo dõi job, nghe thử audio chương và tải full book.
- `Backend/API`: `Go + Gin` để điều phối parser TXT, chunking, synthesize, merge audio, metadata và job lifecycle.
- `Database/Metadata store`: `SQLite` với các bảng `stories`, `chapters`, `build_jobs`, `segments`, `artifacts`, `telegram_accounts`, `telegram_bot_profiles`.
- `External services`: `edge-tts` hoặc nguồn tương thích, `ffmpeg`, NotebookLM cho research/governance.

## 3. Luồng dữ liệu mức cao
1. Người dùng chọn hoặc nạp một thư mục truyện chứa nhiều file TXT theo chương.
2. Backend phân tích cấu trúc truyện, tạo metadata chương và chia chunk an toàn cho từng đoạn cần synthesize.
3. Hệ thống gọi provider TTS để sinh audio theo chunk hoặc chương, lưu trạng thái job để có thể retry từng phần.
4. Audio từng chương được ghép và chuẩn hóa thành file chương hoàn chỉnh.
5. Một job merge riêng tạo file full book từ các chương đã hoàn tất.

## 4. Ràng buộc quan trọng
- `Độ bền job`: phải rerun được theo chương hoặc chunk khi provider cộng đồng lỗi.
- `Tương thích repo nền`: ưu tiên học theo pattern provider/service/UI của `andon-tts-web-api`, nhưng không ép lại các rule NAS nếu bài toán mới không cần.
- `Bảo mật`: không commit key TTS, secret và state NotebookLM.
- `File storage`: phải phân biệt thư mục input TXT, audio từng chương, audio full book và file tạm khi merge.
- `Telegram`: v1 dùng `MTProto user session`; phần auth đã có, phần chat/download bot ngoài vẫn là bước tiếp theo.

## 5. Tài liệu liên quan
- `README.md`
- `AGENTS.md`
- `docs/ai-digest.md`
- `docs/TODO.md`
- Repo tham chiếu: `D:\Tinht00_Workspace\Projects\Andon\andon-tts-web-api`
