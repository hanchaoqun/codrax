pub const MILLIS_PER_SEC: i64 = 1_000;
pub const NANOS_PER_MILLI: i32 = 1_000_000;
pub const NANOS_PER_SEC: i32 = 1_000_000_000;

#[derive(Clone, Copy, Debug, Eq, PartialEq, Ord, PartialOrd)]
pub struct Duration {
    secs: i64,
    nanos: i32,
}

pub(crate) const MIN: Duration = Duration {
    secs: -i64::MAX / MILLIS_PER_SEC - 1,
    nanos: NANOS_PER_SEC + (-i64::MAX % MILLIS_PER_SEC) as i32 * NANOS_PER_MILLI,
};

pub(crate) const MAX: Duration = Duration {
    secs: i64::MAX / MILLIS_PER_SEC,
    nanos: (i64::MAX % MILLIS_PER_SEC) as i32 * NANOS_PER_MILLI,
};

impl Duration {
    pub const fn milliseconds(milliseconds: i64) -> Duration {
        let secs = milliseconds / MILLIS_PER_SEC;
        let millis = milliseconds % MILLIS_PER_SEC;
        let nanos = millis as i32 * NANOS_PER_MILLI;
        Duration { secs, nanos }
    }

    pub fn checked_sub(&self, rhs: &Duration) -> Option<Duration> {
        let secs = self.secs - rhs.secs;
        let nanos = self.nanos - rhs.nanos;
        let candidate = Duration { secs, nanos };
        if candidate < MIN || candidate > MAX {
            return None;
        }
        Some(candidate)
    }
}
