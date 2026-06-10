"""Reduced from dateutil/dateutil relativedelta (pre gh-411/gh-553).

The constructor accepted whole-number floats for months/years but kept
them as floats, so applying the delta to a date exploded later in
calendar arithmetic. Upstream normalizes integer-valued floats to int
and rejects non-integer floats with ValueError.
"""
import datetime


class relativedelta(object):
    def __init__(self, years=0, months=0):
        self.years = years
        self.months = months

    def __radd__(self, other):
        if not isinstance(other, datetime.date):
            raise TypeError("relativedelta can only be added to dates")
        year = other.year + self.years
        month = other.month + self.months
        extra_years, month_index = divmod(month - 1, 12)
        return other.replace(year=year + extra_years, month=month_index + 1)
