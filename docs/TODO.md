# TODO

Danh sách này chỉ giữ các hạng mục chưa xong, đang bị chặn hoặc còn thiếu bước verify.

## Mẫu ghi
### [Tiêu đề ngắn]
- Bối cảnh: `[mô tả ngắn]`
- Trạng thái hiện tại: `[đã làm tới đâu]`
- Việc còn thiếu: `[phần còn lại]`
- Cách verify: `[cách kiểm tra sau khi hoàn tất]`

## Hạng mục hiện tại
### Đồng bộ notebook canonical sau đợt cập nhật docs
- Bối cảnh: bộ docs trong repo đã được cập nhật lại theo runtime hiện tại, gồm architecture, runtime cases, digest và decision log.
- Trạng thái hiện tại: notebook canonical đã tồn tại, nhưng trong phiên này chưa có bước upload/sync lại các file markdown mới cập nhật.
- Việc còn thiếu: đồng bộ tối thiểu `README.md`, `docs/architecture-v1.md`, `docs/realtime-runtime.md`, `docs/decision-log.md`, `docs/ai-digest.md` vào notebook `story-tts - Governance & Docs`.
- Cách verify: trong NotebookLM thấy đúng phiên bản tài liệu mới và có thể tra cứu được các case runtime vừa ghi lại.

### Làm sạch code TTS legacy sau khi đường chính đã ổn định
- Bối cảnh: backend Go vẫn còn route, model và runtime của luồng `DirectTTSSession` cũ để tránh refactor phá vỡ quá nhanh.
- Trạng thái hiện tại: frontend đã chạy ổn theo đường `realtime TTS service`; docs chính đã được cập nhật theo luồng hiện tại.
- Việc còn thiếu: xác định chính xác các route/model/service legacy không còn được frontend gọi tới, rồi dọn dần mà không làm gãy import thư viện, chapter content và progress.
- Cách verify: frontend không còn phụ thuộc vào route cũ, backend build pass, smoke test import/content/progress không bị ảnh hưởng.
