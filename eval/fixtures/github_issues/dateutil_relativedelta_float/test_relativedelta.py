"""Mirrors the upstream regression from dateutil gh-411 / gh-553."""
import datetime
import unittest

from relativedelta import relativedelta


class RelativeDeltaFloatTest(unittest.TestCase):
    def test_whole_float_months_apply(self):
        result = datetime.date(2020, 1, 1) + relativedelta(months=12.0)
        self.assertEqual(result, datetime.date(2021, 1, 1))

    def test_whole_float_years_apply(self):
        result = datetime.date(2020, 3, 15) + relativedelta(years=2.0)
        self.assertEqual(result, datetime.date(2022, 3, 15))

    def test_int_months_still_work(self):
        result = datetime.date(2020, 1, 15) + relativedelta(months=1)
        self.assertEqual(result, datetime.date(2020, 2, 15))

    def test_non_integer_float_rejected(self):
        with self.assertRaises(ValueError):
            relativedelta(months=1.5)


if __name__ == "__main__":
    unittest.main()
