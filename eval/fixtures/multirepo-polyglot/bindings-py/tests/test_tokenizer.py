import unittest

from fastlex import FastTokenizer


class TokenizerTest(unittest.TestCase):
    def test_merge_order(self):
        tok = FastTokenizer([(104, 105, 256)])
        self.assertEqual(tok.tokenize("hi"), [256])


if __name__ == "__main__":
    unittest.main()
