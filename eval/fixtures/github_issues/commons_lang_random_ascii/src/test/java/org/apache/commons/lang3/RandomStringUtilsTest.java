package org.apache.commons.lang3;

import static org.junit.jupiter.api.Assertions.assertEquals;

class RandomStringUtilsTest {
    void testAsciiLettersStillWork() {
        assertEquals("range:65:123", RandomStringUtils.random(4, 'A', 'z' + 1, true, false));
    }
}
