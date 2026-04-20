# Realtime Runtime Cases

## Mục tiêu tài liệu
- Ghi lại luồng runtime thực tế của reader realtime TTS.
- Lưu các case lỗi đã gặp có giá trị tái sử dụng.
- Làm nguồn tra cứu nhanh khi cần debug `frontend -> websocket -> audio player -> tts_service`.

## Luồng runtime hiện tại
1. Người dùng mở một chapter trong reader và bấm `Phát`.
2. Frontend gọi `POST /sessions` tới `tts_service` với `storyId`, `chapterId`, danh sách chapter, `voice`, `speed`, `pitch`, `autoNext` và `startSegmentIndex` nếu có.
3. `tts_service` tạo `RuntimeSession`, mở worker nền và emit các event khởi tạo như `session_started`, `audio_format`, `chapter_started`, `chapter_segments`.
4. Service chia chapter theo word-based segment:
   - segment đầu ưu tiên ngắn hơn để vào tiếng nhanh
   - log hiện tại ghi rõ chiến lược `150/200 words`
5. Service render ahead theo cơ chế `15 / 10 / 10`:
   - khởi tạo trước tối đa `15` segment tính từ vị trí bắt đầu đọc
   - khi ahead buffer còn khoảng `10` segment thì nạp tiếp `10` segment kế tiếp
6. Mỗi segment được render bằng `edge_tts.Communicate.save(...)` ra MP3 tạm, sau đó đọc bytes vào memory, đo `durationEstimate` bằng `ffprobe` nếu có.
7. Khi một segment sẵn sàng, service emit `segment_ready`; khi bắt đầu phát thật, service emit `segment_started` rồi gửi bytes MP3 theo chunk `4096` bytes qua WebSocket.
8. Frontend nhận binary frame, append vào `MediaSource` với `audio/mpeg`, cập nhật timeline phát và trạng thái segment trong panel realtime.
9. Khi hết segment hiện tại, frontend dựa trên event `segment_finished` và `chapter_finished` để cập nhật UI, còn service sẽ tự chuyển chapter nếu `autoNext=true`.
10. Progress được lưu theo `chapter audio đang phát thực tế`, không bám theo chapter đang buffer ahead hoặc chapter người dùng chỉ đang xem.

## Event quan trọng cần nhớ
- Khởi tạo phiên: `session_started`, `audio_format`
- Đồng bộ chapter: `chapter_started`, `chapter_segments`, `chapter_finished`, `chapter_transition`, `story_finished`
- Đồng bộ segment: `segment_rendering`, `segment_ready`, `segment_retry`, `segment_started`, `segment_finished`
- Đóng phiên và lỗi: `stopped`, `stream_closed`, `error`

## Logic seek và phát lại
- Click vào segment đã `ready` hoặc đã `played`:
  - frontend ưu tiên giữ nguyên session hiện tại
  - service thử `fast seek cache hit`
  - nếu cache có sẵn audio segment đích, service emit lại `chapter_segments`, `chapter_started`, `segment_started`, đẩy audio cached và tiếp tục từ vị trí kế tiếp
- Click vào segment chưa `ready`:
  - frontend vẫn ưu tiên `seek` trên session hiện tại để không mất pipeline/cache đang có
  - nếu session không vào đúng segment đích sau timeout ngắn, frontend tự restart playback từ chapter/segment đó
- Khi đang seek:
  - frontend khóa tạm auto-sync chapter
  - audio cũ bị drop cho đến khi event `segment_started` của mục tiêu mới xuất hiện

## Logic hiển thị trong reader
- Reader và panel realtime tách riêng:
  - `chapter đang hiển thị`
  - `chapter đang buffer ahead`
  - `chapter audio đang phát thực tế`
- Highlight chỉ được phép bám theo segment đang phát của đúng chapter đang hiển thị, tránh tô nhầm sang chapter backend mới render ahead.
- Panel segment ưu tiên chapter audio hiện tại, sau đó mới tới chapter người dùng đang mở và chapter đang buffer ahead; các chapter đã nằm phía sau audio hiện tại sẽ bị ẩn khỏi panel chính để tránh hiển thị lẫn lộn.
- Panel realtime nhóm segment theo chapter để nhìn rõ:
  - chapter nào đang render ahead
  - chapter nào đang phát
  - chapter nào đã hoàn tất

## Case lỗi đã ghi nhận

### 1. Log báo synth xong toàn bộ segment nhưng không phát audio
- Triệu chứng:
  - backend/service log `Chapter ... finished: x/x segments synthesized`
  - WebSocket đã `connection open`
  - UI hiển thị segment `ready` hoặc `completed`
  - player không phát tiếng dù session đã chạy
- Nguyên nhân gốc:
  - lỗi nằm ở frontend player, không phải ở `tts_service`
  - `MediaSource` từng được chờ `sourceopen` trước khi gắn vào `<audio>`, làm thứ tự khởi tạo sai
  - lượt phát đầu có thể mất `user gesture` nếu luôn `await stopRealtimePlayback()` trước khi chuẩn bị stream mới
  - logic pause vì buffer thấp có thể tự chặn ngay ở thời điểm đầu khi `currentTime` gần `0`
- Cách xác minh:
  - mở session trực tiếp bằng WebSocket và xác nhận nhận được cả JSON event lẫn binary frame MP3
  - nếu có binary frame nhưng UI vẫn im lặng, lỗi nằm ở frontend player
- Cách sửa đã chốt:
  - gắn `MediaSource` vào `<audio>` trước khi chờ `sourceopen`
  - prime playback sớm hơn bằng `resumeRealtimeAudioPlayback()`
  - chỉ stop phiên cũ khi thật sự có session/socket/media stream cũ
  - không auto-pause ở thời điểm đầu khi `currentTime` còn sát `0`
- File liên quan:
  - `frontend/src/App.vue`
  - `tts_service/app.py`

### 2. Click seek sang segment/chapter khác nhưng UI bị kéo ngược về chapter cũ
- Triệu chứng:
  - người dùng click segment mới
  - reader nhảy đúng chapter mục tiêu một lúc rồi bị kéo về chapter cũ
  - audio thực tế vẫn đang chờ seek hoặc đang render chapter đích
- Nguyên nhân gốc:
  - watcher auto-sync chapter từng bám theo event realtime cũ trong lúc seek chưa hoàn tất
- Cách sửa đã chốt:
  - khóa tạm auto-sync chapter trong lúc chờ `segment_started` của mục tiêu mới
  - chỉ mở khóa khi pending seek hoàn tất hoặc fallback restart đã kích hoạt

### 3. Seek sang chapter khác nhưng service phát trúng segment cache rồi không đi tiếp đúng luồng
- Triệu chứng:
  - click một segment đã cache ở chapter khác
  - audio mục tiêu có thể phát được ngay một đoạn
  - sau đó worker không tiếp tục chapter đúng hoặc rơi vào trạng thái skip sai
- Nguyên nhân gốc:
  - worker cũ nuốt nhầm `same chapter seek`
  - `_stream_chapter()` chưa trả về `skipped` sạch sẽ khi context chapter thực tế không còn khớp chapter worker đang xử lý
- Cách sửa đã chốt:
  - chỉ xử lý `same chapter seek` khi worker thực sự còn ở đúng chapter
  - nếu chapter context đã lệch, để worker thoát về outer loop và chuyển chapter sạch sẽ
- File liên quan:
  - `tts_service/app.py`

### 4. Progress bị lưu nhầm sang chapter backend đang preload
- Triệu chứng:
  - UI đang nghe chapter A nhưng progress hoặc highlight nhảy theo chapter B vừa được render ahead
- Nguyên nhân gốc:
  - frontend từng dùng nhầm chapter từ event buffer/render thay vì chapter audio đang phát thật
- Cách sửa đã chốt:
  - tách rõ `buffered chapter` và `audible chapter`
  - progress chỉ lưu theo chapter audio thực tế

### 5. Dev server Vite trên Windows trả lỗi `/@vite/client`
- Triệu chứng:
  - frontend dev load trắng trang hoặc lỗi HMR script 404
- Nguyên nhân gốc:
  - một số phiên chạy trên Windows không trả chuẩn module HMR nội bộ
- Cách sửa đã chốt:
  - `frontend/vite.config.ts` có middleware fallback để phục vụ trực tiếp `/@vite/client` và `/@vite/env`

### 6. Máy vừa cài Go hoặc Node nhưng `run.ps1` vẫn báo không tìm thấy binary
- Triệu chứng:
  - terminal cũ chưa nhận `PATH` mới
  - script mở service thất bại dù đã cài runtime
- Cách sửa đã chốt:
  - `run.ps1` và `.codex/environments/setup.ps1` ưu tiên gọi binary chuẩn như `C:\Program Files\Go\bin\go.exe` và `C:\Program Files\nodejs\npm.cmd`

## Checklist debug nhanh khi realtime lỗi
1. Xác nhận `backend :18080`, `tts_service :8010`, `frontend :5174` đều đang sống.
2. Gọi `GET /health` của realtime service.
3. Tạo session bằng HTTP và kiểm tra `POST /sessions` trả `id`.
4. Mở WebSocket `/sessions/{id}/stream` và xác nhận:
   - có event `audio_format`
   - có event `segment_started`
   - có binary frame MP3
5. Nếu binary frame có nhưng UI im lặng:
   - kiểm tra frontend player, `MediaSource`, autoplay/user gesture và buffer pause logic
6. Nếu không có binary frame:
   - kiểm tra render retry, timeout `edge_tts`, voice đang dùng, log `segment_retry` và `error`
