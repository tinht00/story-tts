"""
Test script cho logic chunking mới:
- Đoạn 1, 2, 3: tối đa 150 từ
- Đoạn 4+: tối đa 200 từ
- Tìm dấu ngắt gần nhất trong khoảng ±20 từ
"""
import sys
import os
import re

# Import trực tiếp các hàm thay vì import cả app.py
# Copy lại logic để test độc lập

def normalize_text(text: str) -> str:
    cleaned = text.replace("\r\n", "\n").replace("\r", "\n")
    cleaned = re.sub(r"[ \t]+", " ", cleaned)
    cleaned = re.sub(r"\n{3,}", "\n\n", cleaned)
    cleaned = re.sub(r"[-=_*~]{5,}", "\n\n", cleaned)
    cleaned = re.sub(r"\s+([,.;:!?])", r"\1", cleaned)
    cleaned = "\n".join(part.strip() for part in cleaned.split("\n"))
    cleaned = re.sub(r"\n{3,}", "\n\n", cleaned)
    return cleaned.strip()


def _build_segment_with_paragraphs(
    words: list[str],
    start_word_idx: int,
    end_word_idx: int,
    word_to_para_map: list[int],
    paragraphs: list[str],
) -> str:
    if not words:
        return ""

    para_indices = set()
    for word_idx in range(start_word_idx, min(end_word_idx, len(word_to_para_map))):
        para_indices.add(word_to_para_map[word_idx])

    if len(para_indices) == 1:
        return " ".join(words)

    result_parts = []
    current_para_idx = -1
    current_para_words = []

    for i, word in enumerate(words):
        global_word_idx = start_word_idx + i
        para_idx = word_to_para_map[global_word_idx]

        if para_idx != current_para_idx:
            if current_para_words:
                result_parts.append(" ".join(current_para_words))
            if current_para_idx != -1:
                result_parts.append("\n\n")
            current_para_idx = para_idx
            current_para_words = [word]
        else:
            current_para_words.append(word)

    if current_para_words:
        result_parts.append(" ".join(current_para_words))

    return "".join(result_parts)


def split_chapter_into_segments(text: str) -> list[str]:
    normalized = normalize_text(text)
    if not normalized:
        return []

    paragraphs = [part.strip() for part in re.split(r"\n{2,}", normalized) if part.strip()]
    if not paragraphs:
        return []

    all_words = []
    word_to_paragraph_map = []

    for para_idx, para in enumerate(paragraphs):
        words = para.split()
        all_words.extend(words)
        word_to_paragraph_map.extend([para_idx] * len(words))

    if not all_words:
        return []

    segments = []
    word_idx = 0
    segment_number = 0

    while word_idx < len(all_words):
        segment_number += 1

        if segment_number <= 3:
            max_words = 150
        else:
            max_words = 200

        target_end = min(word_idx + max_words, len(all_words))

        if target_end >= len(all_words):
            segment_text = " ".join(all_words[word_idx:])
            segments.append(segment_text)
            break

        # Tìm dấu ngắt câu gần nhất trong khoảng ±20 từ từ target_end
        best_split = target_end
        
        # Xác định khoảng tìm kiếm: từ (target_end - 20) đến (target_end + 20)
        search_window_start = max(word_idx + max_words - 20, word_idx + 50)  # Tối thiểu 50 từ
        search_window_end = min(target_end + 20, len(all_words))
        
        if search_window_start < target_end:
            # Gộp text trong khoảng tìm kiếm
            search_text = " ".join(all_words[word_idx:search_window_end])
            
            # Tìm dấu ngắt câu ưu tiên: . ! ? > , ; : > …
            breakpoint_chars = ['.', '!', '?', ',', ';', ':', '…']
            best_break_pos = -1
            
            # Tìm trong khoảng từ (max_words - 20) đến (max_words + 20) từ
            min_words_in_segment = max_words - 20
            max_words_in_segment = max_words + 20
            
            for break_char in breakpoint_chars:
                # Tìm tất cả vị trí của break_char
                start_pos = 0
                while True:
                    pos = search_text.find(break_char, start_pos)
                    if pos == -1:
                        break
                    
                    # Đếm số từ trước vị trí này
                    words_before = len(search_text[:pos+1].split())
                    
                    # Kiểm tra xem có trong khoảng chấp nhận được không
                    if min_words_in_segment <= words_before <= max_words_in_segment:
                        # Ưu tiên dấu câu mạnh (. ! ?) hơn (, ; :)
                        if break_char in '.!?':
                            best_break_pos = pos
                            break  # Tìm thấy dấu mạnh, dừng ngay
                        elif best_break_pos == -1 or break_char not in ',;:…':
                            best_break_pos = pos
                    
                    start_pos = pos + 1
            
            if best_break_pos > 0:
                # Tính lại word index tại vị trí break
                text_before_break = search_text[:best_break_pos+1]
                words_in_segment = len(text_before_break.split())
                best_split = word_idx + words_in_segment

        segment_words = all_words[word_idx:best_split]

        segment_text = _build_segment_with_paragraphs(
            segment_words, word_idx, best_split, word_to_paragraph_map, paragraphs
        )

        segments.append(segment_text)
        word_idx = best_split

    return segments


def test_segmentation(text: str, label: str):
    print(f"\n{'='*80}")
    print(f"TEST: {label}")
    print(f"{'='*80}")
    
    segments = split_chapter_into_segments(text)
    
    print(f"Tổng số đoạn: {len(segments)}\n")
    
    for i, seg in enumerate(segments, 1):
        word_count = len(seg.split())
        char_count = len(seg)
        preview = seg[:100].replace('\n', ' ')
        
        # Highlight nếu vượt quá giới hạn
        limit = 150 if i <= 3 else 200
        status = "✓" if word_count <= limit else f"⚠ VƯỢT GIỚI HẠN ({limit})"
        
        print(f"Đoạn {i:2d}: {word_count:4d} từ, {char_count:5d} ký tự {status}")
        print(f"         Preview: {preview}...")
        print()


# Test 1: Text ngắn
test_text_1 = """
Đây là một đoạn văn bản ngắn để test cơ bản. Nó có khoảng 50 từ.
Không có gì đặc biệt ở đây cả. Chỉ là một vài câu đơn giản.
""".strip()

# Test 2: Text trung bình (~500 từ)
test_text_2 = """
Chương 1: Bắt Đầu

Trong một ngôi làng nhỏ nằm giữa những ngọn núi xanh tươi, có một cậu bé tên là Minh.
Cậu sống cùng với bà nội trong một căn nhà gỗ cũ kỹ bên bờ suối.
Mỗi sáng, Minh thức dậy sớm, đi bộ qua cánh rừng để lấy nước và hái củi.

Cuộc sống tuy nghèo nàn nhưng đầy ắp tiếng cười và tình yêu thương của bà.
Bà thường kể cho Minh nghe những câu chuyện cổ tích về các vị anh hùng và phép thuật.
Cậu bé luôn lắng nghe với đôi mắt sáng ngời, tưởng tượng mình là nhân vật trong những câu chuyện đó.

Một ngày nọ, khi đang đi sâu vào rừng, Minh phát hiện một hang động bí ẩn.
Hang động được che phủ bởi những dây leo chằng chịt và tỏa ra ánh sáng mờ ảo.
Tò mò, cậu bước vào bên trong và thấy một cuốn sách cũ kỹ nằm trên tảng đá.

Cuốn sách có bìa da màu nâu sẫm, với những ký tự kỳ lạ không ai có thể đọc được.
Nhưng khi Minh chạm tay vào, những trang sách tự động mở ra và phát ra ánh sáng rực rỡ.
Từ trong cuốn sách, một giọng nói vang lên: "Con đã được chọn, hỡi người thừa kế cuối cùng."

Minh giật mình lùi lại, nhưng giọng nói tiếp tục: "Đừng sợ hãi. Cuốn sách này chứa đựng kiến thức của hàng ngàn năm.
Và giờ đây, nó sẽ truyền lại cho con sức mạnh mà con không thể tưởng tượng được."

Ánh sáng từ cuốn sách bao trùm lấy cậu bé, và trong khoảnh khắc đó, Minh cảm nhận được
một luồng năng lượng kỳ lạ chạy khắp cơ thể. Cậu biết rằng cuộc đời mình sẽ thay đổi mãi mãi.

Khi tỉnh lại, Minh thấy mình nằm trước cửa hang động, cuốn sách đã biến mất.
Nhưng trong đầu cậu giờ đây tràn ngập những kiến thức và ký ức không phải của mình.
Cậu có thể hiểu được ngôn ngữ của gió, của cây cối, và của cả những sinh vật trong rừng.

Minh quay trở về nhà với tâm trạng bồi hồi. Bà nội nhìn cậu và mỉm cười:
"Cuối cùng thì ngày đó cũng đến. Bà đã đợi con từ rất lâu rồi."

Và từ đó, cuộc phiêu lưu thực sự của Minh bắt đầu, với những thử thách và phép thuật
mà cậu chưa từng mơ tới. Nhưng cậu không còn sợ hãi nữa, bởi vì cậu đã có
sức mạnh của cuốn sách cổ và tình yêu thương của bà luôn đồng hành.
""".strip()

# Test 3: Text dài với nhiều đoạn văn (~1000 từ)
test_text_3 = """
Chương 10: Cuộc Chiến Cuối Cùng

Bầu trời đen ngịt bao trùm khắp vương quốc. Những đám mây cuồn cuộn sấm sét
như báo hiệu một thảm họa kinh hoàng sắp ập đến. Gió rít gào qua những khe núi,
mang theo mùi tanh tưởi của máu và chết chóc.

Quân đoàn bóng tối đã tập hợp ở cánh đồng phía bắc. Hàng vạn tên orc, troll,
và những sinh vật kinh tởm khác đứng san sát nhau, tạo thành một biển người đen ngòm.
Chỉ huy chúng là chúa tể hắc ám Vrakthar, một gã khổng lồ với đôi mắt đỏ ngầu
và thanh kiếm tỏa ra hơi lạnh tử thần.

Phía bên kia chiến tuyến, liên quân các tộc người cũng đã sẵn sàng.
Con người, elf, dwarf, và cả rồng đứng cùng nhau trong một hàng ngũ chỉnh tề.
Chỉ huy là công chúa Elara, người thừa kế ngai vàng cuối cùng,
cùng với vị pháp sư già Gandor và đội quân tinh nhuệ nhất vương quốc.

"Hôm nay chúng ta chiến đấu không chỉ cho bản thân," Elara vang giọng,
"mà cho tương lai của tất cả mọi người. Cho những đứa trẻ sẽ được sinh ra
trong hòa bình. Cho những người đã ngã xuống sẽ không phải vô ích!"

Tiếng hò reo vang dội bầu trời. Tinh thần binh sĩ dâng cao như sóng trào.

Trận chiến bắt đầu khi Vrakthar ra lệnh tấn công. Những tên orc lao lên trước,
gầm rú man dại. Cung thủ hai bên đồng loạt bắn, tạo thành những cơn mưa tên
đen ngòm bay qua bầu trời.

Hàng đầu tiên của liên quân sụp đổ ngay trong đợt xung phong đầu tiên.
Nhưng họ không lùi bước. Những người phía sau tiến lên, lấp đầy khoảng trống,
và đáp trả bằng những đòn chí mạng.

Gandor giương cao cây trượng phép, đọc thần chú. Một cơn bão lửa bùng lên
giữa đội hình địch, thiêu cháy hàng trăm tên orc. Nhưng chúng quá đông,
và cứ lớp này ngã xuống thì lớp khác lại lao lên.

Trên bầu trời, những con rồng của liên quân giao chiến với dơi khổng lồ của địch.
Lửa rồng phụt ra xé nát bầu trời, trong khi tiếng dơi rít lên inh ỏi.
Một con rồng bị trúng tên độc, rơi xuống như một ngôi sao băng đỏ rực.

Elara lao vào trận chiến với thanh kiếm ánh sáng. Mỗi nhát chém của cô
là một tên orc ngã xuống. Nhưng chúa tể Vrakthar đã xuất hiện,
và cuộc đối đầu giữa hai người bắt đầu.

Kiếm của Vrakthar nặng如山, mỗi đòn đánh khiến Elara phải lùi lại.
Nhưng cô nhanh nhẹn hơn, né tránh những đòn chí mạng và tìm kẽ hở.
Cuối cùng, cô đâm thẳng thanh kiếm ánh sáng vào ngực Vrakthar.

Chúa tể hắc ám gầm lên đau đớn, nhưng không ngã xuống.
Hắn cười lớn: "Ngươi nghĩ điều này có thể giết ta sao? Ta là bất tử!"

Nhưng Elara không sợ. Cô rút kiếm ra và đâm lần nữa, lần này vào trán hắn.
Ánh sáng từ thanh kiếm bùng lên, và Vrakthar tan thành tro bụi.

Không còn chỉ huy, quân đoàn bóng tối hoảng loạn và bắt đầu rút lui.
Tiếng hò reo chiến thắng vang dội khắp chiến trường. Liên quân đã thắng.

Nhưng cái giá phải thật đắt. Hàng ngàn người đã ngã xuống.
Những cánh đồng nhuộm đỏ máu. Và Elara biết rằng,
dù đã chiến thắng, hành trình tái thiết vương quốc mới chỉ bắt đầu.
""".strip()

# Test 4: Text với câu rất dài không có dấu ngắt
test_text_4 = """
Đây là một câu rất rất dài không có dấu chấm câu nào cả và nó tiếp tục tiếp tục và tiếp tục thêm nhiều từ nữa để xem hệ thống sẽ xử lý như thế nào khi gặp phải trường hợp đặc biệt này vì thông thường chúng ta sẽ tìm dấu ngắt câu gần nhất nhưng nếu không có dấu ngắt nào thì sao hệ thống phải cắt cứng tại giới hạn từ và điều đó có thể làm cho câu bị cụt lủn không có nghĩa nhưng đó là cách duy nhất để xử lý những trường hợp đặc biệt như thế này và chúng ta cần phải test kỹ để đảm bảo hệ thống hoạt động tốt trong mọi tình huống có thể xảy ra trong quá trình sử dụng thực tế.
""".strip()

# Chạy tests
print("KIỂM TRA LOGIC CHUNKING MỚI")
print("="*80)

test_segmentation(test_text_1, "Text ngắn (~50 từ)")
test_segmentation(test_text_2, "Text trung bình (~500 từ)")
test_segmentation(test_text_3, "Text dài (~1000 từ)")
test_segmentation(test_text_4, "Câu rất dài không dấu ngắt")

# Test với file thật
print(f"\n{'='*80}")
print("TEST VỚI FILE THỰC TẾ")
print(f"{'='*80}")

test_file = "data/run/chapter14.txt"
if os.path.exists(test_file):
    with open(test_file, 'r', encoding='utf-8') as f:
        content = f.read()
    test_segmentation(content[:3000], f"File thực tế (3000 ký tự đầu)")
else:
    print(f"Không tìm thấy file: {test_file}")

print("\n" + "="*80)
print("HOÀN TẤT KIỂM TRA")
print("="*80)
