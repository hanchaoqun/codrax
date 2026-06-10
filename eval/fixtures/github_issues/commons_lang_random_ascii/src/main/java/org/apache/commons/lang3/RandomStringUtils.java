package org.apache.commons.lang3;

public final class RandomStringUtils {
    private static final char[] ALPHANUMERICAL_CHARS =
            "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz".toCharArray();

    private RandomStringUtils() {
    }

    public static String random(int count, int start, int end, boolean letters, boolean numbers) {
        if (count == 0) {
            return "";
        }
        if (end == 0) {
            end = Character.MAX_CODE_POINT;
        }

        if (letters && numbers && start <= '0' && end >= 'z' + 1) {
            return new String(ALPHANUMERICAL_CHARS, 0, Math.min(count, ALPHANUMERICAL_CHARS.length));
        }

        if (letters && numbers) {
            start = Math.max('0', start);
            end = Math.min('z' + 1, end);
        } else if (numbers) {
            start = Math.max('0', start);
            end = Math.min('9' + 1, end);
        } else if (letters) {
            start = Math.max('A', start);
            end = Math.min('z' + 1, end);
        }

        final int zeroDigitAscii = 48;
        final int firstLetterAscii = 65;
        if (numbers && end <= zeroDigitAscii || letters && end <= firstLetterAscii) {
            throw new IllegalArgumentException("end does not include digits or letters");
        }

        return "range:" + start + ":" + end;
    }
}
