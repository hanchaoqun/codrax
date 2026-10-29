import unittest

from request_intervals import instant, overlaps, parse_log


def event(second, kind, model="x", level="DEBUG"):
    return ("2026-10-29T00:00:%sZ %s [llm] %s: model=%s\n" %
            (second, level, kind, model)).encode()


def parse(raw, **kwargs):
    return parse_log(raw, "2026-10-29T00:00:00Z", "2026-10-29T00:01:00Z", **kwargs)


class RequestIntervalsTest(unittest.TestCase):
    def test_sticky_ambiguity_until_actual_zero(self):
        raw = (event("00", "request") + event("01", "request") + event("02", "response") +
               event("03", "request") + event("04", "response") + event("05", "response"))
        result = parse(raw)
        self.assertEqual(result["completed"], [])
        self.assertEqual(len(result["ambiguous"]), 1)
        self.assertTrue(result["ambiguous"][0]["closed"])
        self.assertEqual([v["line"] for v in result["ambiguous"][0]["requests"]], [1, 2, 4])
        self.assertEqual(result["unresolved_request_count"], 0)
        result = parse(raw + event("06", "request") + event("07", "response"))
        self.assertEqual([(v["start"]["line"], v["finish"]["line"]) for v in result["completed"]], [(7, 8)])

    def test_last_remaining_response_still_ambiguous(self):
        result = parse(event("00", "request") + event("01", "request") + event("02", "response") + event("03", "response"))
        self.assertEqual(result["completed"], [])
        self.assertTrue(result["ambiguous"][0]["closed"])

    def test_open_group_is_not_cli_state(self):
        result = parse(event("00", "request") + event("01", "request") + event("02", "response") +
                       event("03", "request") + event("04", "response"))
        self.assertEqual(result["completed"], [])
        self.assertEqual(result["unresolved_request_count"], 1)
        self.assertFalse(result["ambiguous"][0]["closed"])
        self.assertIn("not_process_state", result["ambiguous"][0]["reason"])

    def test_cross_model_group_conservative_and_unknown_does_not_drain(self):
        result = parse(event("00", "request", "x") + event("01", "request", "y") +
                       event("02", "response", "z") + event("03", "response", "x"))
        self.assertEqual(result["completed"], [])
        self.assertEqual(result["unresolved_request_count"], 1)
        self.assertEqual(result["ambiguous"][0]["outstanding_by_model"], [{"model": "y", "count": 1}])

    def test_missing_duplicate_and_invalid_model_never_pair(self):
        for metadata in (b"", b"model=x model=x", b"model=\xff", b"model=x\x00"):
            with self.subTest(metadata=metadata):
                raw = event("00", "request") + b"2026-10-29T00:00:01Z DEBUG [llm] response: " + metadata + b"\n"
                result = parse(raw + event("02", "request") + event("03", "response"))
                self.assertEqual(result["completed"], [])
                self.assertEqual(result["unresolved_request_count"], 1)

    def test_unknown_request_model_stays_pending(self):
        result = parse(b"2026-10-29T00:00:00Z DEBUG [llm] request: \n" + event("01", "response") +
                       event("02", "request") + event("03", "response"))
        self.assertEqual(result["completed"], [])
        self.assertEqual(result["unresolved_request_count"], 1)

    def test_invalid_utf8_noise_preserves_line_and_bytes(self):
        raw = event("00", "request") + b"\xff\x00\n\v\fnoise\n" + event("02", "response")
        original = raw[:]
        result = parse(raw)
        self.assertFalse(result["valid_utf8"])
        self.assertEqual(result["completed"][0]["finish"]["line"], 4)
        self.assertEqual(raw, original)

    def test_only_exact_logger_events_and_warn_success(self):
        raw = (b"quoted " + event("00", "request") + event("01", "request") +
               b"2026-10-29T00:00:02Z DEBUG [diag explorer] phase=llm_request\n" +
               event("03", "response", level="WARN"))
        result = parse(raw)
        self.assertEqual(result["counts"], {"requests": 1, "responses": 1, "recognized_events": 2})
        self.assertEqual(result["completed"][0]["duration_seconds"], 2)

    def test_time_order_and_process_bounds_cannot_mint_pair(self):
        cases = [event("03", "request") + event("02", "response"),
                 event("00", "request") + b"2026-10-29T00:02:00Z DEBUG [llm] response: model=x\n",
                 event("00", "request") + b"bad-time DEBUG [llm] response: model=x\n"]
        for raw in cases:
            with self.subTest(raw=raw):
                result = parse(raw)
                self.assertEqual(result["completed"], [])
                self.assertTrue(result["ambiguous"][0]["invalid"])
                self.assertEqual(result["unresolved_request_count"], 1)

    def test_invalid_response_time_cannot_clear_old_request(self):
        raw = event("00", "request") + b"bad-time DEBUG [llm] response: model=x\n"
        result = parse(raw + event("02", "request") + event("03", "response"))
        self.assertEqual(result["completed"], [])
        self.assertEqual(result["unresolved_request_count"], 1)

    def test_no_implicit_host_timezone(self):
        raw = event("00", "request").replace(b"Z DEBUG", b" DEBUG") + event("01", "response").replace(b"Z DEBUG", b" DEBUG")
        self.assertEqual(parse(raw)["completed"], [])
        self.assertEqual(len(parse(raw, utc_offset_seconds=0)["completed"]), 1)
        self.assertEqual(instant("2026-10-28T17:00:00-07:00")[0], instant("2026-10-29T00:00:00Z")[0])

    def test_integer_overlap_and_touching_boundaries(self):
        a = parse(event("01.001", "request") + event("30.000", "response"))["completed"]
        b = parse(event("00.900", "request") + event("22.234", "response"))["completed"]
        result = overlaps(a, b)
        self.assertEqual(result[0]["microseconds"], 21233000)
        self.assertEqual(result[0]["seconds"], 21.233)
        c = parse(event("30.000", "request") + event("31.000", "response"))["completed"]
        self.assertEqual(overlaps(a, c), [])

    def test_orphan_and_multiple_closed_groups(self):
        raw = event("00", "response")
        for n in (1, 6):
            raw += event("%02d" % n, "request") + event("%02d" % (n + 1), "request")
            raw += event("%02d" % (n + 2), "response") + event("%02d" % (n + 3), "response")
        raw += event("11", "request") + event("12", "response")
        result = parse(raw)
        self.assertEqual(len(result["completed"]), 1)
        self.assertEqual(len(result["ambiguous"]), 2)
        self.assertEqual(len(result["diagnostics"]), 1)


if __name__ == "__main__":
    unittest.main()
