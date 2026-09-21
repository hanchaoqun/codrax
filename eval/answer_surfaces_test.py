import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from answer_surfaces import load


class AnswerSurfacesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="answer-surfaces-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "directory with spaces"
        self.root.mkdir()
        self.log = self.root / "run.log"
        self.prefix = str(self.root / "run-1")
        self.body = b"model first\nsystem front/middle\nmodel last\ncitation-only"
        self.obj = {"schema_version": 1, "status": "available",
                    "markdown_sha256": hashlib.sha256(self.body).hexdigest(),
                    "answer_sha256": hashlib.sha256(self.body).hexdigest(),
                    "primary": "model first\nmodel last", "principal": "model first\nmodel last\ncitation-only"}
        self.path = self.root / "current.answer-surfaces.json"
        self.md = self.root / "current.md"
        self.md.write_bytes(self.body)
        self.publish()

    def publish(self, append=False):
        self.path.write_text(json.dumps(self.obj), encoding="utf-8")
        markdown = Path(str(self.path)[:-len(".answer-surfaces.json")] + ".md")
        markdown.write_bytes(self.body)
        line = ("2026-09-20T00:00:00.000 INFO [output_dump] wrote " + str(markdown)
                + " (" + str(len(self.body)) + " bytes)\n"
                + "2026-09-20T00:00:00.001 INFO [output_dump] answer surfaces path=" + str(self.path)
                + " sha256=" + hashlib.sha256(self.path.read_bytes()).hexdigest() + "\n")
        self.log.write_text((self.log.read_text() if append else "") + line)

    def append_log(self, text):
        self.log.write_text(self.log.read_text() + text)

    def markdown_event(self, path, body):
        path.write_bytes(body)
        return ("2026-09-20T00:00:01.000 INFO [output_dump] wrote " + str(path)
                + " (" + str(len(body)) + " bytes)\n")

    def test_exact_surfaces_and_archived_binding(self):
        load(self.log, self.prefix)
        self.assertEqual(Path(self.prefix + ".primary.md").read_text(), self.obj["primary"])
        self.assertNotIn("system", Path(self.prefix + ".principal.md").read_text())
        self.assertEqual(Path(self.prefix + ".answer-transcript.md").read_bytes(), self.body)

    def test_receipt_changed(self):
        self.path.write_text("{}")
        with self.assertRaisesRegex(ValueError, "receipt_mismatch"):
            load(self.log, self.prefix)

    def test_transcript_changed(self):
        self.md.write_bytes(b"different run")
        with self.assertRaisesRegex(ValueError, "transcript_mismatch"):
            load(self.log, self.prefix)

    def test_latest_unavailable_does_not_borrow_old_scope(self):
        self.obj["status"] = "unavailable"
        self.path = self.root / "later.answer-surfaces.json"
        self.publish(append=True)
        with self.assertRaisesRegex(ValueError, "ownership_unavailable"):
            load(self.log, self.prefix)

    def test_latest_markdown_receipt_failure_does_not_borrow_old_scope(self):
        self.append_log(self.markdown_event(self.root / "later.md", b"new answer")
                        + "2026-09-20T00:00:01.001 WARN [output_dump] answer surfaces write failed: denied\n")
        with self.assertRaisesRegex(ValueError, "receipt_missing"):
            load(self.log, self.prefix)
        self.assertFalse(Path(self.prefix + ".primary.md").exists())

    def test_same_path_rewrite_requires_new_receipt_event(self):
        # A millisecond filename collision may overwrite the same physical path.
        # Even identical bytes do not let the earlier render certify the new one.
        self.append_log(self.markdown_event(self.md, self.body))
        with self.assertRaisesRegex(ValueError, "receipt_missing"):
            load(self.log, self.prefix)

    def test_old_receipt_logged_after_new_markdown_is_not_its_receipt(self):
        old_receipt_event = self.log.read_text().splitlines(keepends=True)[-1]
        self.append_log(self.markdown_event(self.root / "later.md", b"new answer") + old_receipt_event)
        with self.assertRaisesRegex(ValueError, "receipt_binding_invalid"):
            load(self.log, self.prefix)

    def test_receipt_without_markdown_write_event_cannot_mint_scope(self):
        self.log.write_text(self.log.read_text().splitlines(keepends=True)[-1])
        with self.assertRaisesRegex(ValueError, "transcript_write_missing"):
            load(self.log, self.prefix)

    def test_latest_matching_markdown_and_receipt_are_selected(self):
        self.path = self.root / "later.answer-surfaces.json"
        self.body = b"latest model\nsystem appendix"
        self.obj.update(markdown_sha256=hashlib.sha256(self.body).hexdigest(),
                        answer_sha256=hashlib.sha256(self.body).hexdigest(),
                        primary="latest model", principal="latest model")
        self.publish(append=True)
        load(self.log, self.prefix)
        self.assertEqual(Path(self.prefix + ".primary.md").read_text(), "latest model")
        self.assertEqual(Path(self.prefix + ".answer-transcript.md").read_bytes(), self.body)

    def test_auxiliary_and_explicit_copy_writes_do_not_replace_default_binding(self):
        self.append_log("2026-09-20T00:00:01.000 INFO [output_dump] wrote " + str(self.root / "current.html") + " (90 bytes)\n"
                        + "2026-09-20T00:00:01.001 INFO [output_dump] wrote " + str(self.root / "later.root-causes.json") + " (90 bytes)\n"
                        + "2026-09-20T00:00:01.002 INFO [output_dump] wrote explicit report " + str(self.root / "copy.md") + " (90 bytes)\n")
        load(self.log, self.prefix)
        self.assertEqual(Path(self.prefix + ".primary.md").read_text(), self.obj["primary"])

    def test_markdown_write_byte_count_binds_transcript(self):
        self.log.write_text(self.log.read_text().replace("(" + str(len(self.body)) + " bytes)", "(1 bytes)"))
        with self.assertRaisesRegex(ValueError, "transcript_mismatch"):
            load(self.log, self.prefix)

    def test_latest_default_markdown_or_directory_failure_cannot_borrow_old_scope(self):
        original = self.log.read_text()
        for failure in ("mkdir " + str(self.root / "later") + " failed: denied",
                        "write " + str(self.root / "later.md") + " failed: denied"):
            with self.subTest(failure=failure):
                self.log.write_text(original + "2026-09-20T00:00:01.000 WARN [output_dump] " + failure + "\n")
                with self.assertRaisesRegex(ValueError, "transcript_write_failed"):
                    load(self.log, self.prefix)
        # A later successful default write and its own receipt may recover.
        self.publish(append=True)
        load(self.log, self.prefix)

    def test_auxiliary_and_explicit_failures_do_not_invalidate_default_receipt(self):
        for failure in ("write " + str(self.root / "current.html") + " failed: denied",
                        "render html for " + str(self.md) + " failed: invalid",
                        "write explicit report " + str(self.root / "copy.md") + " failed: denied",
                        "mkdir " + str(self.root / "copy") + " for explicit report " + str(self.root / "copy.md") + " failed: denied"):
            self.append_log("2026-09-20T00:00:01.000 WARN [output_dump] " + failure + "\n")
        load(self.log, self.prefix)
        self.assertEqual(Path(self.prefix + ".primary.md").read_text(), self.obj["primary"])

    def test_empty_model_body_is_valid_not_full_answer_fallback(self):
        self.obj["primary"] = self.obj["principal"] = ""
        self.publish()
        load(self.log, self.prefix)
        self.assertEqual(Path(self.prefix + ".primary.md").read_text(), "")

    def test_model_or_missing_log_line_cannot_mint_scope(self):
        for text in ("", "model says [output_dump] answer surfaces path=" + str(self.path)):
            self.log.write_text(text)
            with self.assertRaisesRegex(ValueError, "receipt_missing"):
                load(self.log, self.prefix)

    def test_invalid_schema_and_surface_fail_closed(self):
        for field, value, reason in (("schema_version", True, "schema_invalid"), ("primary", None, "surface_invalid"), ("answer_sha256", "", "answer_binding_invalid")):
            with self.subTest(field=field):
                old = self.obj[field]
                self.obj[field] = value
                self.publish()
                with self.assertRaisesRegex(ValueError, reason):
                    load(self.log, self.prefix)
                self.obj[field] = old


if __name__ == "__main__":
    unittest.main()
