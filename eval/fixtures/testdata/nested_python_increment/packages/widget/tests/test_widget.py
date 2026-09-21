import unittest

from widget import increment


class IncrementTest(unittest.TestCase):
    def test_negative_integers(self):
        for value in (-5, -1, -(2**64)):
            with self.subTest(value=value):
                self.assertEqual(increment(value), value + 1)

    def test_zero(self):
        self.assertEqual(increment(0), 1)

    def test_positive_integers(self):
        for value in (1, 7, 2**64):
            with self.subTest(value=value):
                self.assertEqual(increment(value), value + 1)


if __name__ == "__main__":
    unittest.main()
