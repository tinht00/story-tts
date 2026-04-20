# Kiến trúc v1

## Mục tiêu
- Đọc truyện dài từ nhiều file TXT theo chương.
- Mở nội dung chương trực tiếp trong web app.
- Phát `realtime TTS` theo chapter/segment mà không cần lưu file audio trung gian cho luồng nghe chính.
- Tự chuyển sang chapter kế tiếp sau khi đọc xong chapter hiện tại.
- Chạy local cho một người dùng với `Go backend + Vue frontend + Python realtime TTS service`.

## Thành phần chính
- `frontend/`
  - UI `Vue 3 + Vite`
  - folder picker, danh sách truyện/chapter, reader 3 cột, realtime panel, player `MediaSource`, fallback `Read Aloud` của Edge
- `backend/`
  - `Go + Gin + SQLite`
  - import thư viện, lấy chapter content, lưu progress, trả config runtime cho frontend
- `tts_service/`
  - `FastAPI + WebSocket + edge_tts`
  - quản lý realtime session, segment pipeline, seek, retry, emit audio bytes
- `data/`
  - SQLite runtime, virtualenv Python, artifact tạm và dữ liệu vận hành khác
- `library/`
  - bản chuẩn hóa của thư viện TXT sau khi import

## Luồng chính
1. Người dùng chọn thư mục gốc bằng folder picker.
2. Frontend quét mỗi thư mục con cấp 1 như một truyện, mỗi file `.txt` trực tiếp bên trong là một chapter.
3. Backend chuẩn hóa text, copy chapter vào `library/<story_slug>/source/chapters/`, lưu metadata vào SQLite và giữ `sourcePath` để `Làm mới thư viện` bám đúng thư mục gốc ban đầu.
4. Reader tải chapter content từ backend và format lại để dễ đọc.
5. Khi người dùng bấm `Phát`, frontend gửi session payload sang `tts_service`.
6. `tts_service` chia chapter thành các segment theo word count, render ahead bằng `edge_tts`, rồi stream `audio/mpeg` qua WebSocket.
7. Frontend append audio bytes vào `MediaSource`, đồng bộ trạng thái chapter/segment và phát trực tiếp trong player ẩn.
8. Khi chapter hiện tại kết thúc, service tự chuyển chapter kế tiếp nếu `autoNext=true`.
9. Frontend lưu progress theo chapter audio đang phát thực tế.

## Pipeline realtime hiện tại

### Segment hóa
- Chapter được chia bằng word-based split.
- Chiến lược log hiện tại là `150/200 words` để segment đầu vào tiếng nhanh hơn, các segment sau dài hơn một chút để giảm overhead.
- Text segment vẫn ưu tiên giữ paragraph boundary khi có thể.

### Render ahead
- Pipeline hiện tại dùng cơ chế `15 / 10 / 10`.
- Khởi tạo trước tối đa `15` segment tính từ điểm bắt đầu đọc.
- Khi ahead buffer còn khoảng `10` segment thì submit thêm `10` segment tiếp theo.
- Mỗi segment sau khi render xong sẽ được cache theo `chapterId -> segmentIndex`.

### Stream audio
- Mỗi segment được render thành MP3 bằng `edge_tts.Communicate.save(...)`.
- Service đọc file MP3 vào memory và phát qua WebSocket bằng chunk `4096` bytes.
- Frontend dùng `MediaSource` với `audio/mpeg` để append các chunk liên tiếp và tự resume playback khi buffer đủ.

### Retry và chịu lỗi
- Nếu render segment lỗi, service emit `segment_retry` và retry với backoff tăng dần.
- Segment hiện tại không bị tự bỏ qua chỉ vì render lỗi; worker sẽ tiếp tục retry cho tới khi thành công hoặc phiên bị dừng/seek sang chỗ khác.
- `durationEstimate` ưu tiên lấy từ `ffprobe`; nếu không probe được thì fallback theo kích thước file.

## Logic chapter và progress
- Frontend tách riêng:
  - chapter đang hiển thị trong reader
  - chapter đang buffer ahead
  - chapter audio đang phát thực tế
- Reader không tự nhảy chapter chỉ vì backend bắt đầu preload chapter sau.
- Progress, trạng thái đang đọc và highlight phải bám theo chapter audio thực tế.

## Logic seek
- Click segment đã render:
  - ưu tiên `fast seek cache hit` trên session hiện tại
  - không tạo session mới nếu cache đang dùng được
- Click segment chưa render:
  - vẫn ưu tiên `seek` trên session hiện tại để giữ cache/pipeline
  - frontend có watchdog timeout để fallback sang restart playback nếu seek bị kẹt
- Trong lúc seek:
  - khóa auto-sync chapter
  - drop audio cũ cho tới khi mục tiêu mới thực sự bắt đầu

## Reader và hiển thị
- Reader format lại text theo block `heading/body/divider/spacer`.
- Highlight theo mức `từng chữ` dựa trên segment đang phát và thời lượng ước lượng.
- Chỉ highlight khi chapter đang hiển thị trùng chapter audio thực tế, tránh tô nhầm chapter đang preload.
- Panel realtime nhóm theo chapter để người dùng thấy rõ:
  - chapter nào đang render
  - chapter nào đang phát
  - chapter nào đã hoàn tất

## Metadata cốt lõi
- `stories`: thông tin truyện, slug, source path, library path, lần mở gần nhất
- `chapters`: thứ tự chapter, file nguồn, normalized text, checksum
- `reader_progress`: chapter đang đọc, scroll percent, audio position
- Runtime cache trong `tts_service`:
  - `chapter_rendered_cache`
  - `chapter_segment_text_cache`
  - `chapter_prefetched_until_cache`

## Ranh giới v1
- Chỉ hỗ trợ cấu trúc `thu_muc_goc/truyen/*.txt`, không quét cây thư mục sâu nhiều tầng.
- Luồng chính là `folder picker -> import TXT -> reader -> realtime TTS -> progress`.
- Telegram và direct TTS legacy còn tồn tại trong codebase nhưng không còn là đường chính của UI.
- Hệ thống đang tối ưu cho local single-user trên Windows với Chrome/Edge là trình duyệt ưu tiên.
