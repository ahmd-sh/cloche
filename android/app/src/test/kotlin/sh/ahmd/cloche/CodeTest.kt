package sh.ahmd.cloche

import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import org.junit.Assert.assertEquals
import org.junit.Test
import sh.ahmd.cloche.ui.typed

class CodeTest {
    private fun v(text: String, at: Int) = TextFieldValue(text, TextRange(at))
    private fun v(text: String, from: Int, to: Int) = TextFieldValue(text, TextRange(from, to))

    @Test
    fun typingAtTheEndAppends() {
        assertEquals(v("ABC", 3), typed(v("AB", 2), v("ABc", 3)))
    }

    @Test
    fun typingInTheMiddleOfAShortCodeInserts() {
        assertEquals(v("AXB", 2), typed(v("AB", 1), v("AXB", 2)))
    }

    @Test
    fun typingIntoAFullCodeTypesOver() {
        val full = "ABCDEFGHJKMN"
        assertEquals(v("ABCDXFGHJKMN", 5), typed(v(full, 4), v("ABCDXEFGHJKMN", 5)))
    }

    @Test
    fun aSelectedCharacterIsReplaced() {
        val full = "ABCDEFGHJKMN"
        assertEquals(v("ABCDXFGHJKMN", 5), typed(v(full, 4, 5), v("ABCDXFGHJKMN", 5)))
    }

    @Test
    fun typingPastAFullCodeChangesNothing() {
        val full = "ABCDEFGHJKMN"
        assertEquals(v(full, 12), typed(v(full, 12), v(full + "P", 13)))
    }

    @Test
    fun backspaceInTheMiddleDeletesTheOneBefore() {
        assertEquals(v("ABDEFGHJKMN", 2), typed(v("ABCDEFGHJKMN", 3), v("ABDEFGHJKMN", 2)))
    }

    @Test
    fun aPasteWithDashesGoesToTheEnd() {
        assertEquals(v("ABCDEFGHJKMN", 12), typed(v("", 0), v("abcd-efgh-jkmn", 14)))
    }
}
