from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import os
import queue
import re
import subprocess
import tempfile
import threading
import time
import uuid
from concurrent.futures import Future, ThreadPoolExecutor
from contextlib import asynccontextmanager
from dataclasses import dataclass, field
from typing import Any, Optional

import edge_tts
from fastapi import FastAPI, HTTPException, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

logger = logging.getLogger("story_tts_realtime")
logging.basicConfig(level=logging.INFO)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup: nothing special needed
    yield
    # Shutdown: clean up all sessions
    registry.cleanup_all()


app = FastAPI(title="story-tts-realtime", version="0.2.0", lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ─── Models ───────────────────────────────────────────────────────────────────


class RealtimeVoice(BaseModel):
    id: str
    name: str
    locale: str
    gender: str
    friendlyName: str


class RealtimeChapterPayload(BaseModel):
    chapterId: int
    chapterIndex: int
    title: str
    text: str


class CreateSessionRequest(BaseModel):
    storyId: int
    chapterId: int
    chapters: list[RealtimeChapterPayload]
    voice: str = "vi-VN-NamMinhNeural"
    speed: int = 0
    pitch: int = 0
    autoNext: bool = True
    startSegmentIndex: int = 0


class UpdateSessionControlsRequest(BaseModel):
    voice: str | None = None
    speed: int | None = None
    pitch: int | None = None
    autoNext: bool | None = None


class SeekSessionRequest(BaseModel):
    chapterId: int
    segmentIndex: int = 0


class SessionResponse(BaseModel):
    id: str
    storyId: int
    chapterId: int
    currentChapterIndex: int
    status: str
    voice: str
    speed: int
    pitch: int
    autoNext: bool


def now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.localtime())


# ─── Text Processing ──────────────────────────────────────────────────────────


def normalize_text(text: str) -> str:
    cleaned = text.replace("\r\n", "\n").replace("\r", "\n")
    cleaned = re.sub(r"[ \t]+", " ", cleaned)
    cleaned = re.sub(r"\n{3,}", "\n\n", cleaned)
    cleaned = re.sub(r"[-=_*~]{5,}", "\n\n", cleaned)
    cleaned = re.sub(r"\s+([,.;:!?])", r"\1", cleaned)
    cleaned = "\n".join(part.strip() for part in cleaned.split("\n"))
    cleaned = re.sub(r"\n{3,}", "\n\n", cleaned)
    return cleaned.strip()


def split_chapter_into_segments(text: str) -> list[str]:
    """
    Chia chapter thành các đoạn nhỏ theo số từ (word count):
    - Đoạn 1, 2, 3: tối đa 150 từ
    - Đoạn 4+: tối đa 200 từ
    - Tìm dấu ngắt câu gần nhất (.,;:!?…) trong khoảng ±20 từ để không cắt cụt câu
    - Giữ nguyên boundary đoạn văn (\n\n), không cắt giữa đoạn nếu có thể
    """
    normalized = normalize_text(text)
    if not normalized:
        return []

    # Tách thành các đoạn văn lớn (giữ \n\n làm boundary)
    paragraphs = [
        part.strip() for part in re.split(r"\n{2,}", normalized) if part.strip()
    ]
    if not paragraphs:
        return []

    # Gộp tất cả từ thành list để dễ xử lý
    all_words = []
    word_to_paragraph_map = []  # Ánh xạ word index -> paragraph index

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

        # Xác định giới hạn từ theo số thứ tự đoạn
        if segment_number <= 3:
            max_words = 150
        else:
            max_words = 200

        # Tính khoảng từ cần lấy
        target_end = min(word_idx + max_words, len(all_words))

        # Nếu còn ít từ hơn max_words, lấy hết
        if target_end >= len(all_words):
            segment_text = " ".join(all_words[word_idx:])
            segments.append(segment_text)
            break

        # Tìm dấu ngắt câu gần nhất trong khoảng ±20 từ từ target_end
        best_split = target_end

        # Xác định khoảng tìm kiếm: từ (target_end - 20) đến (target_end + 20)
        search_window_start = max(
            word_idx + max_words - 20, word_idx + 50
        )  # Tối thiểu 50 từ
        search_window_end = min(target_end + 20, len(all_words))

        if search_window_start < target_end:
            # Gộp text trong khoảng tìm kiếm
            search_text = " ".join(all_words[word_idx:search_window_end])

            # Tìm dấu ngắt câu ưu tiên: . ! ? > , ; : > …
            breakpoint_chars = [".", "!", "?", ",", ";", ":", "…"]
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
                    words_before = len(search_text[: pos + 1].split())

                    # Kiểm tra xem có trong khoảng chấp nhận được không
                    if min_words_in_segment <= words_before <= max_words_in_segment:
                        # Ưu tiên dấu câu mạnh (. ! ?) hơn (, ; :)
                        if break_char in ".!?":
                            best_break_pos = pos
                            break  # Tìm thấy dấu mạnh, dừng ngay
                        elif best_break_pos == -1 or break_char not in ",;:…":
                            best_break_pos = pos

                    start_pos = pos + 1

            if best_break_pos > 0:
                # Tính lại word index tại vị trí break
                text_before_break = search_text[: best_break_pos + 1]
                words_in_segment = len(text_before_break.split())
                best_split = word_idx + words_in_segment

        # Tạo segment từ word_idx đến best_split
        segment_words = all_words[word_idx:best_split]

        # Ghép lại thành text, giữ paragraph boundary nếu có
        segment_text = _build_segment_with_paragraphs(
            segment_words, word_idx, best_split, word_to_paragraph_map, paragraphs
        )

        segments.append(segment_text)
        word_idx = best_split

    logger.info(
        "Chapter split into %d segments (word-based: 150/200 words)", len(segments)
    )
    return segments


def _build_segment_with_paragraphs(
    words: list[str],
    start_word_idx: int,
    end_word_idx: int,
    word_to_para_map: list[int],
    paragraphs: list[str],
) -> str:
    """
    Gộp các từ thành text segment, giữ paragraph boundary (\n\n) khi có thể.
    """
    if not words:
        return ""

    # Xác định các paragraph tham gia vào segment này
    para_indices = set()
    for word_idx in range(start_word_idx, min(end_word_idx, len(word_to_para_map))):
        para_indices.add(word_to_para_map[word_idx])

    # Nếu chỉ có 1 paragraph, trả về đơn giản
    if len(para_indices) == 1:
        return " ".join(words)

    # Nhiều paragraph: cần reconstruct lại text với \n\n
    result_parts = []
    current_para_idx = -1
    current_para_words = []

    for i, word in enumerate(words):
        global_word_idx = start_word_idx + i
        para_idx = word_to_para_map[global_word_idx]

        if para_idx != current_para_idx:
            # Paragraph mới
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


def split_into_segments(text: str, max_chars: int = 600) -> list[str]:
    """Chia text thành các đoạn nhỏ cho highlighting."""
    normalized = normalize_text(text)
    if not normalized:
        return []

    paragraphs = [
        part.strip() for part in re.split(r"\n{2,}", normalized) if part.strip()
    ]
    if not paragraphs:
        return []

    segments = []
    for paragraph in paragraphs:
        if len(paragraph) <= max_chars:
            segments.append(paragraph)
        else:
            sentences = re.split(r"(?<=[.!?])\s+", paragraph)
            current = ""
            for sentence in sentences:
                if len(current) + len(sentence) <= max_chars:
                    current += (" " if current else "") + sentence
                else:
                    if current:
                        segments.append(current)
                    current = sentence
            if current:
                segments.append(current)

    return segments


def prepare_tts_segment(text: str) -> str:
    return normalize_text(text)


# ─── Edge-TTS Synthesis ───────────────────────────────────────────────────────


def _format_prosody(speed: int, pitch: int) -> tuple[str, str]:
    rate_str = f"+{speed}%" if speed >= 0 else f"{speed}%"
    pitch_str = f"+{pitch}Hz" if pitch >= 0 else f"{pitch}Hz"
    return rate_str, pitch_str


def _preview_text(text: str, limit: int = 96) -> str:
    compact = re.sub(r"\s+", " ", text).strip()
    if len(compact) <= limit:
        return compact
    return f"{compact[:limit].rstrip()}..."


def _format_exception(exc: Exception) -> str:
    message = str(exc).strip()
    return message or exc.__class__.__name__


def _segment_retry_delay(attempt: int) -> float:
    return min(8.0, 0.8 * attempt)


def probe_audio_duration_seconds(file_path: str) -> float:
    """
    Lấy thời lượng thực của file audio bằng ffprobe.
    Fallback trả 0 nếu không probe được, để caller dùng ước lượng cũ.
    """
    try:
        result = subprocess.run(
            [
                "ffprobe",
                "-v",
                "quiet",
                "-print_format",
                "json",
                "-show_format",
                file_path,
            ],
            capture_output=True,
            text=True,
            check=True,
            timeout=8,
        )
        payload = json.loads(result.stdout or "{}")
        duration_raw = payload.get("format", {}).get("duration")
        duration = float(duration_raw)
        if duration > 0:
            return duration
    except Exception as exc:
        logger.debug("ffprobe khong lay duoc duration cho %s: %s", file_path, exc)
    return 0.0


async def _save_audio_with_edge_tts(
    text: str,
    voice: str,
    speed: int,
    pitch: int,
    output_path: str,
    timeout_seconds: float,
) -> None:
    rate_str, pitch_str = _format_prosody(speed, pitch)
    communicate = edge_tts.Communicate(
        text=text,
        voice=voice,
        rate=rate_str,
        pitch=pitch_str,
        connect_timeout=15,
        receive_timeout=max(30, int(timeout_seconds)),
    )
    await asyncio.wait_for(communicate.save(output_path), timeout=timeout_seconds)


def synthesize_segment_edge_tts(
    text: str,
    voice: str,
    speed: int,
    pitch: int,
    output_path: str,
    max_retries: int = 2,
    timeout_seconds: float = 90.0,
) -> tuple[int, float]:
    """
    Dùng edge_tts Python API để tổng hợp text thành file MP3.
    Trả về (file_size_bytes, estimated_duration_seconds).
    """
    if not text.strip():
        return 0, 0.0

    last_error = None
    for attempt in range(1, max_retries + 1):
        try:
            if os.path.exists(output_path):
                os.unlink(output_path)

            asyncio.run(
                _save_audio_with_edge_tts(
                    text=text,
                    voice=voice,
                    speed=speed,
                    pitch=pitch,
                    output_path=output_path,
                    timeout_seconds=timeout_seconds,
                )
            )

            if not os.path.exists(output_path):
                raise RuntimeError("edge_tts không tạo file audio")

            file_size = os.path.getsize(output_path)
            if file_size == 0:
                raise RuntimeError("edge_tts tạo file audio rỗng")

            actual_duration = probe_audio_duration_seconds(output_path)
            if actual_duration > 0:
                estimated_duration = actual_duration
            else:
                # Fallback khi ffprobe không đọc được duration.
                bytes_per_second = 6000
                estimated_duration = file_size / bytes_per_second

            logger.debug(
                "Synthesized segment: %d bytes, %.1fs", file_size, estimated_duration
            )
            return file_size, estimated_duration

        except TimeoutError:
            last_error = RuntimeError(f"edge_tts timeout ({timeout_seconds:.0f}s)")
        except Exception as e:
            last_error = e

        if attempt < max_retries:
            logger.warning(
                "Render retry %d/%d cho voice=%s: %s",
                attempt,
                max_retries,
                voice,
                _format_exception(last_error),
            )
            time.sleep(0.4 * attempt)

    raise RuntimeError(
        f"edge_tts lỗi sau {max_retries} lần thử: {_format_exception(last_error)}"
    )


# ─── Session Management ───────────────────────────────────────────────────────


@dataclass
class RenderedSegment:
    """Lưu kết quả render của một đoạn."""

    index: int
    audio_data: bytes
    duration_estimate: float
    text: str


@dataclass
class RuntimeSession:
    id: str
    story_id: int
    chapters: list[RealtimeChapterPayload]
    current_index: int
    voice: str
    speed: int
    pitch: int
    auto_next: bool
    start_segment_index: int = 0
    created_at: str = field(default_factory=now_iso)
    updated_at: str = field(default_factory=now_iso)
    status: str = "pending"
    last_error: str = ""
    pending_index: int | None = None
    pending_segment_index: int | None = None
    loop: Any = None
    outbox: Any = None
    worker: threading.Thread | None = None
    stop_requested: threading.Event = field(default_factory=threading.Event)
    current_stream: Any = None
    lock: threading.RLock = field(default_factory=threading.RLock)
    closed: threading.Event = field(default_factory=threading.Event)
    stream_epoch: int = 0

    # Pipeline rendering
    render_executor: ThreadPoolExecutor = field(
        default_factory=lambda: ThreadPoolExecutor(max_workers=3)
    )
    rendered_segments: dict[int, RenderedSegment] = field(default_factory=dict)
    render_futures: dict[int, Future] = field(default_factory=dict)
    chapter_rendered_cache: dict[int, dict[int, RenderedSegment]] = field(
        default_factory=dict
    )
    chapter_segment_text_cache: dict[int, list[str]] = field(default_factory=dict)
    chapter_prefetched_until_cache: dict[int, int] = field(default_factory=dict)
    pipeline_initial_window: int = 15
    pipeline_refill_threshold: int = 10
    pipeline_refill_batch: int = 10
    prefetched_until_index: int = -1
    current_segment_index: int = 0
    total_segments: int = 0
    segments_to_render: list[str] = field(
        default_factory=list
    )  # Danh sách text các đoạn
    initial_segment_anchor_used: bool = False

    def close(self):
        """Stop worker thread and shut down executor."""
        self.stop()
        try:
            self.render_executor.shutdown(wait=False)
        except Exception:
            pass

    def to_response(self) -> SessionResponse:
        chapter = self.chapters[self.current_index]
        return SessionResponse(
            id=self.id,
            storyId=self.story_id,
            chapterId=chapter.chapterId,
            currentChapterIndex=chapter.chapterIndex,
            status=self.status,
            voice=self.voice,
            speed=self.speed,
            pitch=self.pitch,
            autoNext=self.auto_next,
        )

    def attach(self, loop, outbox) -> None:
        with self.lock:
            self.loop = loop
            self.outbox = outbox

    def start(self) -> None:
        with self.lock:
            if self.worker and self.worker.is_alive():
                return
            self.worker = threading.Thread(
                target=self._run, daemon=True, name=f"tts-session-{self.id}"
            )
            self.worker.start()
            logger.info("Session %s da khoi dong worker", self.id)

    def stop(self) -> None:
        with self.lock:
            self.stream_epoch += 1
        self.stop_requested.set()
        self._stop_stream()

    def skip(self, delta: int) -> None:
        with self.lock:
            self.stream_epoch += 1
            target_index = max(
                0, min(self.current_index + delta, len(self.chapters) - 1)
            )
            self.pending_index = target_index
            self.pending_segment_index = 0
        self._stop_stream()

    def seek(self, request: SeekSessionRequest) -> SessionResponse:
        with self.lock:
            try:
                target_index = next(
                    index
                    for index, item in enumerate(self.chapters)
                    if item.chapterId == request.chapterId
                )
            except StopIteration as exc:
                raise HTTPException(
                    status_code=400, detail="Không tìm thấy chapterId để seek"
                ) from exc

            self.stream_epoch += 1
            self.pending_index = target_index
            self.pending_segment_index = max(0, request.segmentIndex)

            if self._try_emit_cached_seek(target_index, self.pending_segment_index):
                return self.to_response()

        # Cắt luồng hiện tại ngay để seek phản hồi nhanh, tương tự skip().
        self._stop_stream()
        return self.to_response()

    def _next_position_after(
        self, chapter_index: int, segment_index: int, total_segments: int
    ) -> tuple[int, int] | None:
        if segment_index + 1 < total_segments:
            return (chapter_index, segment_index + 1)
        if self.auto_next and chapter_index + 1 < len(self.chapters):
            return (chapter_index + 1, 0)
        return None

    def _try_emit_cached_seek(self, target_index: int, segment_index: int) -> bool:
        chapter = self.chapters[target_index]
        chapter_cache = self.chapter_rendered_cache.get(chapter.chapterId, {})
        rendered = chapter_cache.get(segment_index)
        if rendered is None:
            return False

        segments = self.chapter_segment_text_cache.get(chapter.chapterId)
        if not segments:
            segments = split_chapter_into_segments(chapter.text)
            self.chapter_segment_text_cache[chapter.chapterId] = segments
        total_segments = len(segments)
        if total_segments == 0:
            return False

        next_position = self._next_position_after(
            target_index,
            segment_index,
            total_segments,
        )
        self.current_index = target_index
        self.start_segment_index = segment_index
        self.current_segment_index = segment_index
        self.initial_segment_anchor_used = True
        self.rendered_segments = dict(chapter_cache)
        self.segments_to_render = segments
        self.total_segments = total_segments
        self.prefetched_until_index = max(
            self.chapter_prefetched_until_cache.get(
                chapter.chapterId, segment_index
            ),
            segment_index,
        )
        if next_position is None:
            self.pending_index = None
            self.pending_segment_index = None
        else:
            self.pending_index, self.pending_segment_index = next_position

        self.emit_event(
            {
                "type": "chapter_segments",
                "sessionId": self.id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "chapterTitle": chapter.title,
                "totalSegments": total_segments,
                "segments": [
                    {
                        "index": index,
                        "text": segment,
                        "wordCount": len(segment.split()),
                    }
                    for index, segment in enumerate(segments)
                ],
                "startSegmentIndex": segment_index,
            }
        )
        self.emit_event(
            {
                "type": "chapter_started",
                "sessionId": self.id,
                "storyId": self.story_id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "chapterTitle": chapter.title,
            }
        )
        self.emit_event(
            {
                "type": "segment_started",
                "sessionId": self.id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "segmentIndex": segment_index,
                "totalSegments": total_segments,
                "segmentText": rendered.text[:200],
                "wordCount": len(rendered.text.split()),
                "durationEstimate": rendered.duration_estimate,
            }
        )
        self.emit_audio(rendered.audio_data)
        self.emit_event(
            {
                "type": "segment_finished",
                "sessionId": self.id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "segmentIndex": segment_index,
            }
        )
        logger.info(
            "Fast seek cache hit: chapter=%d segment=%d, tiếp tục từ %s",
            chapter.chapterIndex,
            segment_index,
            next_position,
        )
        return True

    def update_controls(
        self, controls: UpdateSessionControlsRequest
    ) -> SessionResponse:
        with self.lock:
            if controls.voice is not None and controls.voice.strip():
                self.voice = controls.voice.strip()
            if controls.speed is not None:
                self.speed = max(-100, min(100, controls.speed))
            if controls.pitch is not None:
                self.pitch = max(-120, min(120, controls.pitch))
            if controls.autoNext is not None:
                self.auto_next = controls.autoNext

            payload = {
                "type": "controls_updated",
                "sessionId": self.id,
                "voice": self.voice,
                "speed": self.speed,
                "pitch": self.pitch,
                "autoNext": self.auto_next,
            }

        self.emit_event(payload)
        return self.to_response()

    def _stop_stream(self) -> None:
        stream = self.current_stream
        if stream is not None:
            try:
                stop = getattr(stream, "stop", None)
                if callable(stop):
                    stop()
            except Exception:
                logger.exception("Không dừng được realtime stream")

    def emit_event(self, payload: dict[str, Any], epoch: int | None = None) -> None:
        self.updated_at = now_iso()
        # Don't emit if session is closed or loop/outbox not ready
        if self.closed.is_set():
            return
        if not self.loop or not self.outbox:
            return
        if epoch is None:
            with self.lock:
                epoch = self.stream_epoch
        try:
            asyncio.run_coroutine_threadsafe(
                self.outbox.put(
                    {"kind": "event", "payload": payload, "epoch": epoch}
                ),
                self.loop,
            )
        except Exception as e:
            logger.debug("Failed to emit event %s: %s", payload.get("type"), e)

    def emit_audio(self, payload: bytes, epoch: int | None = None) -> None:
        if not payload or not self.loop or not self.outbox:
            return
        if epoch is None:
            with self.lock:
                epoch = self.stream_epoch
        asyncio.run_coroutine_threadsafe(
            self.outbox.put({"kind": "audio", "payload": payload, "epoch": epoch}),
            self.loop,
        )

    def render_single_segment(
        self,
        chapter_id: int,
        chapter_index: int,
        chapter_title: str,
        segment_index: int,
        text: str,
    ) -> Optional[RenderedSegment]:
        """
        Render một đoạn thành MP3 file dùng edge_tts Python API, trả về RenderedSegment.
        Chạy trong render_executor (thread pool).
        """
        if not text.strip():
            return None

        try:
            # Tạo temp file cho MP3 output
            with tempfile.NamedTemporaryFile(suffix=".mp3", delete=False) as tmp:
                tmp_path = tmp.name

            try:
                file_size, estimated_duration = synthesize_segment_edge_tts(
                    text=text,
                    voice=self.voice,
                    speed=self.speed,
                    pitch=self.pitch,
                    output_path=tmp_path,
                    max_retries=2,
                    timeout_seconds=90.0,
                )

                # Đọc MP3 data
                with open(tmp_path, "rb") as f:
                    audio_data = f.read()

                self.emit_event(
                    {
                        "type": "segment_ready",
                        "sessionId": self.id,
                        "chapterId": chapter_id,
                        "chapterIndex": chapter_index,
                        "chapterTitle": chapter_title,
                        "segmentIndex": segment_index,
                        "totalSegments": self.total_segments,
                        "segmentText": text[:200],
                        "wordCount": len(text.split()),
                        "durationEstimate": estimated_duration,
                    }
                )
                logger.debug(
                    "Rendered segment %d: %d bytes (%.1fs)",
                    segment_index,
                    file_size,
                    estimated_duration,
                )
                return RenderedSegment(
                    index=segment_index,
                    audio_data=audio_data,
                    duration_estimate=estimated_duration,
                    text=text,
                )

            finally:
                # Cleanup temp file
                with contextlib.suppress(FileNotFoundError, PermissionError):
                    os.unlink(tmp_path)
        except Exception as e:
            logger.warning(
                "Segment %d render lỗi: %s | preview=%r",
                segment_index,
                _format_exception(e),
                _preview_text(text),
            )

        return None

    def _submit_segment_render(
        self,
        chapter: RealtimeChapterPayload,
        segment_index: int,
        attempt: int = 1,
    ) -> None:
        if segment_index >= len(self.segments_to_render):
            return
        if (
            segment_index in self.rendered_segments
            or segment_index in self.render_futures
        ):
            return

        segment_text = self.segments_to_render[segment_index]
        self.emit_event(
            {
                "type": "segment_rendering",
                "sessionId": self.id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "segmentIndex": segment_index,
                "totalSegments": self.total_segments,
                "segmentText": segment_text[:200],
                "wordCount": len(segment_text.split()),
                "attempt": attempt,
            }
        )
        future = self.render_executor.submit(
            self.render_single_segment,
            chapter.chapterId,
            chapter.chapterIndex,
            chapter.title,
            segment_index,
            segment_text,
        )
        def _store_render_result(done_future: Future) -> None:
            try:
                result = done_future.result()
            except Exception:
                return
            if result is None:
                return
            with self.lock:
                chapter_cache = self.chapter_rendered_cache.setdefault(
                    chapter.chapterId, {}
                )
                chapter_cache[segment_index] = result
                self.chapter_prefetched_until_cache[chapter.chapterId] = max(
                    self.chapter_prefetched_until_cache.get(chapter.chapterId, -1),
                    segment_index,
                )
                if (
                    self.current_index < len(self.chapters)
                    and self.chapters[self.current_index].chapterId
                    == chapter.chapterId
                ):
                    self.rendered_segments[segment_index] = result

        future.add_done_callback(_store_render_result)
        self.render_futures[segment_index] = future

    def _ensure_pipeline_ahead(
        self,
        chapter: RealtimeChapterPayload,
        start_segment_index: int,
        force_initial: bool = False,
    ):
        """
        Giữ buffer render ahead theo cơ chế:
        - Lần đầu nạp tối đa 15 segment tính từ vị trí bắt đầu.
        - Khi số segment còn lại trong buffer chỉ còn 10, nạp thêm 10 segment.
        """
        if start_segment_index >= self.total_segments:
            return

        reset_window = force_initial or self.prefetched_until_index < (
            start_segment_index - 1
        )
        if reset_window:
            submission_start = start_segment_index
            self.prefetched_until_index = start_segment_index - 1
            batch_size = self.pipeline_initial_window
        else:
            remaining_buffer = self.prefetched_until_index - start_segment_index + 1
            if remaining_buffer > self.pipeline_refill_threshold:
                return
            submission_start = self.prefetched_until_index + 1
            batch_size = self.pipeline_refill_batch

        end_index = min(submission_start + batch_size, self.total_segments)
        if submission_start >= end_index:
            return

        for seg_idx in range(submission_start, end_index):
            self._submit_segment_render(chapter, seg_idx)
        self.prefetched_until_index = max(self.prefetched_until_index, end_index - 1)
        self.chapter_prefetched_until_cache[chapter.chapterId] = (
            self.prefetched_until_index
        )

    def _consume_same_chapter_seek(self, chapter: RealtimeChapterPayload) -> bool:
        with self.lock:
            if (
                self.pending_index != self.current_index
                or self.pending_segment_index is None
            ):
                return False
            target_segment_index = max(
                0, min(self.pending_segment_index, self.total_segments - 1)
            )
            self.current_segment_index = target_segment_index
            self.pending_index = None
            self.pending_segment_index = None

        self.emit_event(
            {
                "type": "chapter_segments",
                "sessionId": self.id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "chapterTitle": chapter.title,
                "totalSegments": self.total_segments,
                "segments": [
                    {
                        "index": index,
                        "text": segment,
                        "wordCount": len(segment.split()),
                    }
                    for index, segment in enumerate(self.segments_to_render)
                ],
                "startSegmentIndex": target_segment_index,
            }
        )
        return True

    def _run(self) -> None:
        try:
            self.status = "streaming"
            logger.info(
                "Worker _run() starting: session_id=%s, current_index=%d, start_segment_index=%d",
                self.id,
                self.current_index,
                self.start_segment_index,
            )
            self.emit_event(
                {
                    "type": "session_started",
                    "sessionId": self.id,
                    "storyId": self.story_id,
                    "chapterId": self.chapters[self.current_index].chapterId,
                    "chapterIndex": self.chapters[self.current_index].chapterIndex,
                    "voice": self.voice,
                    "speed": self.speed,
                    "pitch": self.pitch,
                    "autoNext": self.auto_next,
                }
            )
            self.emit_event({"type": "audio_format", "mime": "audio/mpeg"})
            logger.info("Session %s setup complete, starting chapter loop", self.id)

            chapter_index = self.current_index
            while chapter_index < len(self.chapters):
                if self.stop_requested.is_set():
                    self.status = "stopped"
                    self.emit_event({"type": "stopped", "sessionId": self.id})
                    break

                with self.lock:
                    if self.pending_index is not None:
                        chapter_index = self.pending_index
                        self.pending_index = None
                    self.current_index = chapter_index

                chapter = self.chapters[chapter_index]
                logger.info(
                    "Starting chapter %d: %s", chapter.chapterIndex, chapter.title
                )
                self.emit_event(
                    {
                        "type": "chapter_started",
                        "sessionId": self.id,
                        "storyId": self.story_id,
                        "chapterId": chapter.chapterId,
                        "chapterIndex": chapter.chapterIndex,
                        "chapterTitle": chapter.title,
                    }
                )

                stream_result = self._stream_chapter(chapter)

                if stream_result == "stopped" or self.stop_requested.is_set():
                    self.status = "stopped"
                    self.emit_event({"type": "stopped", "sessionId": self.id})
                    break

                with self.lock:
                    if self.pending_index is not None:
                        target_index = self.pending_index
                        target_segment_index = self.pending_segment_index or 0
                        self.emit_event(
                            {
                                "type": "chapter_transition",
                                "sessionId": self.id,
                                "fromChapterId": chapter.chapterId,
                                "toChapterId": self.chapters[target_index].chapterId,
                                "reason": "seek"
                                if target_segment_index > 0
                                or target_index != chapter_index
                                else "skip",
                            }
                        )
                        chapter_index = target_index
                        self.start_segment_index = target_segment_index
                        self.initial_segment_anchor_used = False
                        self.pending_index = None
                        self.pending_segment_index = None
                        continue

                if stream_result == "skipped":
                    continue

                self.emit_event(
                    {
                        "type": "chapter_finished",
                        "sessionId": self.id,
                        "chapterId": chapter.chapterId,
                        "chapterIndex": chapter.chapterIndex,
                    }
                )
                if not self.auto_next:
                    self.status = "stopped"
                    self.emit_event({"type": "stopped", "sessionId": self.id})
                    break

                if chapter_index >= len(self.chapters) - 1:
                    self.status = "completed"
                    self.emit_event(
                        {
                            "type": "story_finished",
                            "sessionId": self.id,
                            "storyId": self.story_id,
                        }
                    )
                    break

                next_chapter = self.chapters[chapter_index + 1]
                self.emit_event(
                    {
                        "type": "chapter_transition",
                        "sessionId": self.id,
                        "fromChapterId": chapter.chapterId,
                        "toChapterId": next_chapter.chapterId,
                        "reason": "auto_next",
                    }
                )
                chapter_index += 1

        except Exception as exc:
            logger.exception("RealtimeTTS session lỗi")
            self.status = "failed"
            self.last_error = str(exc)
            self.emit_event(
                {"type": "error", "sessionId": self.id, "message": str(exc)}
            )
        finally:
            self.closed.set()
            self.emit_event(
                {"type": "stream_closed", "sessionId": self.id, "status": self.status}
            )

    def _stream_chapter(self, chapter: RealtimeChapterPayload) -> str:
        """
        Stream chapter với pipeline rendering:
        1. Chia chapter thành các đoạn (150 từ cho đoạn 1-3, 200 từ cho đoạn 4+)
        2. Đọc đoạn 1 → render trước các đoạn 2, 3, 4
        3. Sau khi đọc đoạn N → đảm bảo đã render xong đoạn N+1
        4. Continue cho đến hết, đảm bảo không sót đoạn nào
        """
        with self.lock:
            chapter_epoch = self.stream_epoch

        # Chia chapter thành các đoạn theo word count
        segments = split_chapter_into_segments(chapter.text)
        if not segments:
            return "completed"

        # Setup pipeline state
        self.chapter_segment_text_cache[chapter.chapterId] = segments
        self.segments_to_render = segments
        self.total_segments = len(segments)

        # Log debug: check if start_segment_index will be applied
        chapter_matches = (
            chapter.chapterId == self.chapters[self.current_index].chapterId
        )
        logger.info(
            "_stream_chapter: chapterId=%d, current_index=%d, current_chapterId=%d, matches=%s, start_segment_index=%d, initial_anchor_used=%s",
            chapter.chapterId,
            self.current_index,
            self.chapters[self.current_index].chapterId,
            chapter_matches,
            self.start_segment_index,
            self.initial_segment_anchor_used,
        )

        if (
            chapter.chapterId == self.chapters[self.current_index].chapterId
            and not self.initial_segment_anchor_used
        ):
            self.current_segment_index = max(
                0, min(self.start_segment_index, self.total_segments - 1)
            )
            self.initial_segment_anchor_used = True
            logger.info(
                "Applied start_segment_index: %d → current_segment_index: %d",
                self.start_segment_index,
                self.current_segment_index,
            )
        else:
            self.current_segment_index = 0
            logger.info(
                "Reset current_segment_index to 0 (chapter mismatch or anchor already used)"
            )
        cached_rendered = dict(self.chapter_rendered_cache.get(chapter.chapterId, {}))
        self.rendered_segments = cached_rendered
        self.render_futures.clear()
        cached_prefetched_until = self.chapter_prefetched_until_cache.get(
            chapter.chapterId,
            max(cached_rendered.keys(), default=self.current_segment_index - 1),
        )
        self.prefetched_until_index = max(
            self.current_segment_index - 1,
            cached_prefetched_until,
        )
        self.chapter_prefetched_until_cache[chapter.chapterId] = (
            self.prefetched_until_index
        )

        logger.info(
            "Chapter %d split into %d segments for pipeline synthesis",
            chapter.chapterIndex,
            len(segments),
        )
        self.emit_event(
            {
                "type": "chapter_segments",
                "sessionId": self.id,
                "chapterId": chapter.chapterId,
                "chapterIndex": chapter.chapterIndex,
                "chapterTitle": chapter.title,
                "totalSegments": self.total_segments,
                "segments": [
                    {
                        "index": index,
                        "text": segment,
                        "wordCount": len(segment.split()),
                    }
                    for index, segment in enumerate(segments)
                ],
                "startSegmentIndex": self.current_segment_index,
            },
            epoch=chapter_epoch,
        )

        interrupted = False

        # Bắt đầu pipeline: render sẵn tối đa 15 segment tính từ vị trí hiện tại.
        logger.info(
            "Starting pipeline from segment index %d (total: %d)",
            self.current_segment_index,
            self.total_segments,
        )
        self._ensure_pipeline_ahead(
            chapter,
            self.current_segment_index,
            force_initial=True,
        )

        # Đọc và phát tuần tự từng đoạn
        while self.current_segment_index < self.total_segments:
            if self.stop_requested.is_set():
                return "stopped"

            if self._consume_same_chapter_seek(chapter):
                continue

            with self.lock:
                if self.pending_index is not None:
                    return "skipped"
                segment_epoch = self.stream_epoch

            seg_idx = self.current_segment_index

            # Khi buffer ahead chỉ còn 10 segment, nạp thêm 10 segment tiếp theo.
            self._ensure_pipeline_ahead(chapter, seg_idx + 1)

            # Chờ segment hiện tại được render xong
            rendered = self._wait_for_segment(chapter, seg_idx)

            if rendered is None:
                if self.stop_requested.is_set():
                    return "stopped"
                if self._consume_same_chapter_seek(chapter):
                    continue
                with self.lock:
                    if self.pending_index is not None:
                        return "skipped"
                continue

            # Phát audio segment đã render
            with self.lock:
                if segment_epoch != self.stream_epoch:
                    return "skipped"
            self.emit_event(
                {
                    "type": "segment_started",
                    "sessionId": self.id,
                    "chapterId": chapter.chapterId,
                    "chapterIndex": chapter.chapterIndex,
                    "segmentIndex": seg_idx,
                    "totalSegments": self.total_segments,
                    "segmentText": rendered.text[:200],
                    "wordCount": len(rendered.text.split()),
                    "durationEstimate": rendered.duration_estimate,
                },
                epoch=segment_epoch,
            )

            # Gửi audio data
            chunk_size = 4096
            interrupted_by_seek = False
            for offset in range(0, len(rendered.audio_data), chunk_size):
                if self.stop_requested.is_set():
                    interrupted = True
                    break
                with self.lock:
                    if (
                        self.pending_index is not None
                        or segment_epoch != self.stream_epoch
                    ):
                        interrupted_by_seek = True
                        break
                chunk = rendered.audio_data[offset : offset + chunk_size]
                self.emit_audio(chunk, epoch=segment_epoch)
                # Sleep nhỏ để tránh gửi quá nhanh
                time.sleep(0.001)

            if interrupted:
                break

            if interrupted_by_seek:
                if self._consume_same_chapter_seek(chapter):
                    continue
                return "skipped"

            self.emit_event(
                {
                    "type": "segment_finished",
                    "sessionId": self.id,
                    "chapterId": chapter.chapterId,
                    "chapterIndex": chapter.chapterIndex,
                    "segmentIndex": seg_idx,
                },
                epoch=segment_epoch,
            )

            self.render_futures.pop(seg_idx, None)

            self.current_segment_index += 1

        logger.info(
            "Chapter %d finished: %d/%d segments synthesized",
            chapter.chapterIndex,
            self.current_segment_index,
            self.total_segments,
        )
        if interrupted:
            return "stopped"
        return "completed"

    def _wait_for_segment(
        self,
        chapter: RealtimeChapterPayload,
        segment_index: int,
        timeout: float = 90.0,
    ) -> Optional[RenderedSegment]:
        """
        Chờ một segment được render xong.
        Nếu đã có trong cache thì trả về ngay.
        Nếu render lỗi thì submit lại cho tới khi thành công hoặc session bị dừng.

        === FIX: Poll pending_index every 100ms instead of blocking for 90s ===
        This allows seek requests to be processed nhanh hơn khi người dùng click segment.
        """
        attempt = 0
        poll_interval = 0.1  # Check pending_index every 100ms

        while not self.stop_requested.is_set():
            # === FIX: Check pending_index BEFORE waiting ===
            with self.lock:
                if self.pending_index is not None:
                    return None

            if segment_index in self.rendered_segments:
                return self.rendered_segments[segment_index]

            if segment_index >= len(self.segments_to_render):
                return None

            if segment_index not in self.render_futures:
                self._submit_segment_render(chapter, segment_index, attempt + 1)

            future = self.render_futures[segment_index]

            # === FIX: Use short timeout polling instead of blocking 90s ===
            try:
                result = future.result(timeout=poll_interval)
                if result is not None:
                    self.rendered_segments[segment_index] = result
                    return result
            except TimeoutError:
                # Future not ready yet, loop back to check pending_index
                continue
            except Exception as e:
                logger.warning(
                    "Khong lay duoc segment %d: %s", segment_index, _format_exception(e)
                )

            attempt += 1

            self.render_futures.pop(segment_index, None)
            delay = _segment_retry_delay(attempt)
            self.emit_event(
                {
                    "type": "segment_retry",
                    "sessionId": self.id,
                    "chapterId": chapter.chapterId,
                    "chapterIndex": chapter.chapterIndex,
                    "segmentIndex": segment_index,
                    "totalSegments": self.total_segments,
                    "attempt": attempt + 1,
                    "message": f"Render lỗi, thử lại sau {delay:.1f}s",
                }
            )
            if attempt == 1 or attempt % 5 == 0:
                logger.warning(
                    "Segment %d se retry lan %d sau %.1fs",
                    segment_index,
                    attempt,
                    delay,
                )
            waited = 0.0
            while waited < delay and not self.stop_requested.is_set():
                with self.lock:
                    if self.pending_index is not None:
                        return None
                step = min(0.1, delay - waited)
                time.sleep(step)
                waited += step

        return None


class SessionRegistry:
    def __init__(self) -> None:
        self._items: dict[str, RuntimeSession] = {}
        self._lock = threading.RLock()

    def create(self, request: CreateSessionRequest) -> RuntimeSession:
        if not request.chapters:
            raise HTTPException(status_code=400, detail="Danh sách chương trống")

        try:
            current_index = next(
                index
                for index, item in enumerate(request.chapters)
                if item.chapterId == request.chapterId
            )
        except StopIteration as exc:
            raise HTTPException(
                status_code=400,
                detail="Không tìm thấy chapterId trong danh sách chương",
            ) from exc

        logger.info(
            "Creating session: storyId=%d, chapterId=%d, current_index=%d, startSegmentIndex=%d, chapters_count=%d",
            request.storyId,
            request.chapterId,
            current_index,
            request.startSegmentIndex,
            len(request.chapters),
        )

        session = RuntimeSession(
            id=uuid.uuid4().hex,
            story_id=request.storyId,
            chapters=request.chapters,
            current_index=current_index,
            voice=request.voice or "vi-VN-NamMinhNeural",
            speed=request.speed,
            pitch=request.pitch,
            auto_next=request.autoNext,
            start_segment_index=max(0, request.startSegmentIndex),
        )
        with self._lock:
            self._items[session.id] = session
        return session

    def get(self, session_id: str) -> RuntimeSession:
        with self._lock:
            session = self._items.get(session_id)
        if session is None:
            raise HTTPException(
                status_code=404, detail="Không tìm thấy session realtime"
            )
        return session

    def snapshot(self) -> dict[str, Any]:
        with self._lock:
            total = len(self._items)
            active = sum(
                1
                for item in self._items.values()
                if item.status in {"pending", "streaming"}
            )
            items = [item.to_response() for item in self._items.values()]
        return {"total": total, "active": active, "items": items}

    def remove(self, session_id: str) -> None:
        with self._lock:
            session = self._items.pop(session_id, None)
        if session:
            session.close()

    def cleanup_all(self) -> None:
        """Stop and clean up all sessions (for app shutdown)."""
        with self._lock:
            sessions = list(self._items.values())
            self._items.clear()
        for session in sessions:
            session.close()
        logger.info("Cleaned up %d active sessions", len(sessions))


# ─── App State ────────────────────────────────────────────────────────────────


registry = SessionRegistry()

DEFAULT_VOICE = "vi-VN-NamMinhNeural"

VIETNAMESE_VOICES = [
    RealtimeVoice(
        id="vi-VN-HoaiMyNeural",
        name="vi-VN-HoaiMyNeural",
        locale="vi-VN",
        gender="Female",
        friendlyName="Microsoft HoaiMy Online (Natural) - Vietnamese",
    ),
    RealtimeVoice(
        id="vi-VN-NamMinhNeural",
        name="vi-VN-NamMinhNeural",
        locale="vi-VN",
        gender="Male",
        friendlyName="Microsoft NamMinh Online (Natural) - Vietnamese",
    ),
]


# ─── API Routes ───────────────────────────────────────────────────────────────


@app.get("/voices")
def list_voices() -> dict[str, Any]:
    return {"items": VIETNAMESE_VOICES, "defaultVoice": DEFAULT_VOICE}


@app.get("/health")
def health() -> dict[str, Any]:
    return {"status": "ok", "sessions": registry.snapshot()["active"]}


@app.post("/sessions")
def create_session(request: CreateSessionRequest) -> SessionResponse:
    session = registry.create(request)
    return session.to_response()


@app.get("/sessions")
def list_sessions() -> dict[str, Any]:
    return registry.snapshot()


@app.websocket("/sessions/{session_id}/stream")
async def stream_session(session_id: str, websocket: WebSocket):
    await websocket.accept()
    session = registry.get(session_id)

    outbox: asyncio.Queue = asyncio.Queue()
    session.attach(asyncio.get_running_loop(), outbox)
    session.start()

    try:
        while True:
            message = await outbox.get()
            if message.get("epoch") != session.stream_epoch:
                continue
            if message["kind"] == "event":
                await websocket.send_json(message["payload"])
            elif message["kind"] == "audio":
                await websocket.send_bytes(message["payload"])
    except WebSocketDisconnect:
        logger.info("WebSocket disconnected for session %s", session_id)
    except Exception as e:
        logger.exception("WebSocket error for session %s: %s", session_id, e)
    finally:
        session.stop()
        registry.remove(session_id)
        logger.info("Session %s removed from registry", session_id)


@app.post("/sessions/{session_id}/stop")
def stop_session(session_id: str) -> dict[str, str]:
    registry.remove(session_id)
    return {"status": "stopped", "id": session_id}


@app.post("/sessions/{session_id}/skip-next")
def skip_next(session_id: str) -> dict[str, str]:
    session = registry.get(session_id)
    session.skip(1)
    return {"status": "skipped", "id": session_id}


@app.post("/sessions/{session_id}/skip-prev")
def skip_prev(session_id: str) -> dict[str, str]:
    session = registry.get(session_id)
    session.skip(-1)
    return {"status": "skipped", "id": session_id}


@app.post("/sessions/{session_id}/controls")
def update_controls(
    session_id: str, controls: UpdateSessionControlsRequest
) -> SessionResponse:
    session = registry.get(session_id)
    return session.update_controls(controls)


@app.post("/sessions/{session_id}/seek")
def seek_session(session_id: str, request: SeekSessionRequest) -> SessionResponse:
    session = registry.get(session_id)
    return session.seek(request)


# ─── Main ────────────────────────────────────────────────────────────────────


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="127.0.0.1", port=8010, log_level="info")
