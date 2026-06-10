// Reduced from iamkun/dayjs src/plugin/duration/index.js (pre-#1611).
//
// parseIso extracts Y/M/D/H/m/s from an ISO 8601 duration. The upstream
// bug: missing components ran straight through Number(undefined) and
// became NaN, so formatting a partial duration like "PT1H" rendered
// "NaN-NaN-..." instead of zeroed components.
const durationRegex = /^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$/;

function parseIso(iso) {
  const match = String(iso).match(durationRegex);
  if (!match) {
    return null;
  }
  const properties = match.slice(1);
  const numberD = properties.map(value => Number(value));
  return {
    years: numberD[0],
    months: numberD[1],
    days: numberD[2],
    hours: numberD[3],
    minutes: numberD[4],
    seconds: numberD[5]
  };
}

function pad(value, width) {
  return String(value).padStart(width, "0");
}

function formatDuration(d) {
  return (
    pad(d.years, 4) + "-" + pad(d.months, 2) + "-" + pad(d.days, 2) +
    "T" + pad(d.hours, 2) + ":" + pad(d.minutes, 2) + ":" + pad(d.seconds, 2)
  );
}

module.exports = { parseIso, formatDuration };
