# Decision Log

## 2026-04-04: Chốt kiến trúc v1
- Dự án là `single-user local`.
- Metadata chuẩn dùng `SQLite`.
- Input chuẩn là thư viện TXT theo chapter.
- Reader progress phải lưu riêng để khôi phục chapter, vị trí cuộn và vị trí audio gần nhất.

## 2026-04-04: Chuyển sản phẩm sang reader TXT local
- Bỏ Telegram khỏi luồng chính của UI và contract frontend.
- Folder picker của trình duyệt là điểm vào chuẩn cho thư viện truyện.
- Mỗi thư mục con cấp 1 là một truyện, mỗi file `.txt` trực tiếp bên trong là một chapter.

## 2026-04-07: Chuyển luồng nghe chính sang realtime service Python
- Luồng nghe chính không còn dựa vào `Go + edge-tts CLI + MP3 từng part` trong UI.
- Tách một service Python riêng dùng `FastAPI + WebSocket + edge_tts`.
- Frontend gọi trực tiếp realtime service bằng `HTTP + WebSocket`, còn backend Go tiếp tục phục vụ thư viện, chapter content và progress.
- Audio được stream trực tiếp dưới dạng chunk `audio/mpeg`, không tạo file audio trung gian cho luồng nghe chính.

## 2026-04-10: Chốt nguyên tắc chapter audio thực tế tách khỏi chapter preload
- UI không được tự nhảy chapter chỉ vì backend bắt đầu render ahead chapter kế tiếp.
- Progress phải bám theo chapter audio đang phát thực tế, không bám theo chapter người dùng chỉ đang xem hoặc chapter service vừa mới preload.
- Highlight chỉ được tô trong chapter đang hiển thị nếu chapter đó trùng chapter audio thực tế.

## 2026-04-12: Chốt seek ưu tiên tái dùng session hiện tại
- Click segment đã render phải ưu tiên `fast seek cache hit`, không tạo session mới nếu chưa cần.
- Click segment chưa `ready` vẫn ưu tiên `seek` trên session hiện tại để tận dụng cache/render state.
- Frontend phải có watchdog fallback để tự restart playback nếu seek hiện tại bị kẹt.

## 2026-04-15: Chốt pipeline render ahead `15 / 10 / 10`
- Session realtime khởi tạo trước `15` segment tính từ điểm bắt đầu đọc.
- Khi ahead buffer chỉ còn khoảng `10` segment, service nạp thêm `10` segment kế tiếp.
- Segment hiện tại không được tự bỏ qua khi render lỗi; worker phải retry cho tới khi thành công hoặc phiên bị dừng.

## 2026-04-20: Chốt cách ghi tài liệu runtime theo source code hiện tại
- `docs/architecture-v1.md` là ảnh chụp kiến trúc hiện tại của đường chính.
- `docs/realtime-runtime.md` là nơi lưu luồng runtime và các case lỗi có giá trị tái sử dụng.
- `docs/TODO.md` chỉ giữ các mục chưa xong thật sự; các bước verify đã hoàn tất phải xóa khỏi TODO để tránh báo động giả.

## 2026-04-20: Chốt bản vá player cho case `synth xong nhưng không phát audio`
- Khi kiểm tra thấy WebSocket đã có binary MP3 nhưng UI không phát, coi frontend player là vùng nghi ngờ chính.
- `MediaSource` phải được gắn vào `<audio>` trước khi chờ `sourceopen`.
- Không được làm mất `user gesture` ở lượt phát đầu chỉ vì luôn `await stopRealtimePlayback()` dù không có phiên cũ.
- Logic auto-pause vì buffer thấp không được tự chặn ngay lúc đầu khi `currentTime` còn gần `0`.
