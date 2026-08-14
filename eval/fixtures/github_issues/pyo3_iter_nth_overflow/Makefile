check: Cargo.toml src/lib.rs src/types/list.rs src/types/tuple.rs tests/iterators.rs tests/check_iterators.py
	python3 tests/check_iterators.py --self-test
	python3 tests/check_iterators.py
	cargo test --quiet
