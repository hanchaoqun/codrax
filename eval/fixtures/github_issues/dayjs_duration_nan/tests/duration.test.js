// Mirrors the upstream regression added by iamkun/dayjs#1611.
const assert = require("assert");
const { parseIso, formatDuration } = require("../src/duration.js");

const partial = parseIso("PT1H");
assert.deepStrictEqual(partial, {
  years: 0,
  months: 0,
  days: 0,
  hours: 1,
  minutes: 0,
  seconds: 0
});
const rendered = formatDuration(partial);
assert.ok(!rendered.includes("NaN"), "missing components must not render NaN, got " + rendered);
assert.strictEqual(rendered, "0000-00-00T01:00:00");

const full = parseIso("P1Y2M3DT4H5M6S");
assert.strictEqual(formatDuration(full), "0001-02-03T04:05:06");

console.log("duration ISO parse regression ok");
