# Preset prosody v1

## Mục tiêu
Tạo các lựa chọn giọng đọc ổn định cho truyện dài mà vẫn giữ được nhịp kể khác nhau, trong giới hạn của `edge-tts`.

## Preset đang có
### `stable`
- Mục tiêu: trung tính, rõ chữ, dùng mặc định cho truyện dài.
- Mapping hiện tại: `rate -1%`, `pitch +0Hz`, `volume +0%`

### `gentle`
- Mục tiêu: kể chuyện nhẹ hơn, nhịp chậm hơn một chút.
- Mapping hiện tại: `rate -8%`, `pitch -2Hz`, `volume +0%`

### `tense`
- Mục tiêu: nhịp nhanh hơn nhẹ, phù hợp đoạn căng thẳng.
- Mapping hiện tại: `rate +4%`, `pitch +4Hz`, `volume +3%`

### `climax`
- Mục tiêu: đoạn cao trào, rõ và đẩy năng lượng hơn.
- Mapping hiện tại: `rate +6%`, `pitch +8Hz`, `volume +6%`

## Ghi chú
- Mapping hiện tại là preset vận hành đầu tiên, chưa phải tuning cuối.
- Nếu `edge-tts` provider thay đổi hành vi, file này phải cập nhật cùng code.
- V1 chưa cho phép chỉnh tay từng câu; override chỉ ở cấp truyện hoặc chương.
