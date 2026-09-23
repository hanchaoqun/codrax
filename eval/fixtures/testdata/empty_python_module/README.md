# Totals module

`totals.py` is an intentionally empty tracked module. Implement `total(values)`
to return the exact sum of an iterable of integers, with zero for an empty input.

The existing regression tests use only Python's standard library:

```sh
python3 -m unittest discover -s tests -v
```

Do not change the tests or introduce dependencies. A generator is a valid input;
no random access or second traversal is required by this API.
