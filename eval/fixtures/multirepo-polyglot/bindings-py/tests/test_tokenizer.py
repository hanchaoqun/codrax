import unittest

from fastlex import FastTokenizer


class TokenizerTest(unittest.TestCase):
    def test_merge_order(self):
        tok = FastTokenizer([(104, 105, 256)])
        self.assertEqual(tok.tokenize("hi"), [256])

    def test_consecutive_newline_run_uses_pair_merge_token(self):
        tok = FastTokenizer([(10, 10, 300)])
        self.assertEqual(
            tok.tokenize("#include <set>\n\n\n\n\n"),
            [35, 105, 110, 99, 108, 117, 100, 101, 32, 60, 115, 101, 116, 62, 300],
        )


if __name__ == "__main__":
    unittest.main()
