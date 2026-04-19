import unittest

from app import RenderedSegment, RealtimeChapterPayload, RuntimeSession


class RuntimeSessionSeekTests(unittest.TestCase):
    def make_session(self) -> RuntimeSession:
        chapters = [
            RealtimeChapterPayload(
                chapterId=101,
                chapterIndex=1,
                title="chuong_1",
                text="Noi dung chuong 1",
            ),
            RealtimeChapterPayload(
                chapterId=202,
                chapterIndex=2,
                title="chuong_2",
                text="Noi dung chuong 2",
            ),
        ]
        session = RuntimeSession(
            id="test",
            story_id=1,
            chapters=chapters,
            current_index=0,
            voice="vi-VN-NamMinhNeural",
            speed=0,
            pitch=0,
            auto_next=True,
        )
        session.emit_event = lambda *args, **kwargs: None
        session.emit_audio = lambda *args, **kwargs: None
        return session

    def test_same_chapter_seek_is_consumed_inline(self):
        session = self.make_session()
        session.current_index = 1
        session.total_segments = 5
        session.segments_to_render = ["s0", "s1", "s2", "s3", "s4"]
        session.pending_index = 1
        session.pending_segment_index = 3

        consumed = session._consume_same_chapter_seek(session.chapters[1])

        self.assertTrue(consumed)
        self.assertEqual(session.current_segment_index, 3)
        self.assertIsNone(session.pending_index)
        self.assertIsNone(session.pending_segment_index)

    def test_cross_chapter_cached_seek_is_not_consumed_by_old_worker(self):
        session = self.make_session()
        session.total_segments = 4
        session.segments_to_render = ["old-0", "old-1", "old-2", "old-3"]
        session.chapter_segment_text_cache[202] = ["new-0", "new-1", "new-2", "new-3"]
        session.chapter_rendered_cache[202] = {
            2: RenderedSegment(
                index=2,
                audio_data=b"cached",
                duration_estimate=1.25,
                text="new-2",
            )
        }

        fast_hit = session._try_emit_cached_seek(target_index=1, segment_index=2)
        consumed = session._consume_same_chapter_seek(session.chapters[0])

        self.assertTrue(fast_hit)
        self.assertFalse(consumed)
        self.assertEqual(session.current_index, 1)
        self.assertEqual(session.pending_index, 1)
        self.assertEqual(session.pending_segment_index, 3)
        self.assertEqual(session.current_segment_index, 2)


if __name__ == "__main__":
    unittest.main()
