import unittest

from totals import total


class TotalTest(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(total([]), 0)

    def test_signed_integers(self):
        self.assertEqual(total([4, -7, 3, 12]), 12)

    def test_large_integers(self):
        self.assertEqual(total([2**70, 5, -(2**70)]), 5)

    def test_single_pass_iterable(self):
        values = (value for value in (2, -3, 8))
        self.assertEqual(total(values), 7)


if __name__ == "__main__":
    unittest.main()
