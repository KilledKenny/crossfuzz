import crossfuzz.Crossfuzz;

/**
 * JSON parse target for cross-language differential fuzzing.
 *
 * Parses the input as JSON using a hand-rolled recursive-descent parser and
 * returns the root value type as ASCII bytes: object / array / string /
 * number / true / false / null / error.
 *
 * Intentionally hand-rolled (no external JSON library) so that edge-case
 * handling can diverge from the C and Go implementations.
 */
public class JavaTarget implements Crossfuzz.FuzzTarget {

    // -----------------------------------------------------------------------
    // Parser state
    // -----------------------------------------------------------------------

    private byte[] src;
    private int    pos;

    // -----------------------------------------------------------------------
    // Entry point
    // -----------------------------------------------------------------------

    @Override
    public byte[] fuzz(byte[] input) {
        src = input;
        pos = 0;
        try {
            String type = parseValue();
            skipWs();
            if (pos != src.length) return b("error");
            return b(type);
        } catch (Exception e) {
            return b("error");
        }
    }

    // -----------------------------------------------------------------------
    // Recursive descent
    // -----------------------------------------------------------------------

    private String parseValue() throws Exception {
        skipWs();
        if (pos >= src.length) throw new Exception("eof");
        byte c = src[pos];
        if (c == '"')             { parseString(); return "string"; }
        if (c == '{')             { parseObject(); return "object"; }
        if (c == '[')             { parseArray();  return "array";  }
        if (c == 't')             { matchLiteral("true");  return "true";  }
        if (c == 'f')             { matchLiteral("false"); return "false"; }
        if (c == 'n')             { matchLiteral("null");  return "null";  }
        if (c == '-' || isDigit(c)) { parseNumber(); return "number"; }
        throw new Exception("unexpected char: " + (char)c);
    }

    private void parseString() throws Exception {
        expect('"');
        while (pos < src.length) {
            byte c = src[pos++];
            if (c == '"') return;
            if (c == '\\') {
                if (pos >= src.length) throw new Exception("eof in escape");
                byte esc = src[pos++];
                if (esc == 'u') {
                    for (int i = 0; i < 4; i++) {
                        if (pos >= src.length) throw new Exception("eof in \\u");
                        byte h = src[pos++];
                        if (!isHex(h)) throw new Exception("bad hex digit");
                    }
                }else {
                    switch (esc) {
                        case '"':
                        case '\\':
                        case '/':
                        case 'b':
                        case 'f':
                        case 'n':
                        case 'r':
                        case 't':
                            break;
                        default:
                            throw new Exception("invalid char in escape code");
                    }

                }
                // other escapes accepted as-is
            } else if ((c & 0xFF) < 0x20) {
                throw new Exception("control char in string");
            }
        }
        throw new Exception("unterminated string");
    }

    private void parseNumber() throws Exception {
        if (pos < src.length && src[pos] == '-') pos++;
        if (pos >= src.length) throw new Exception("eof");
        if (src[pos] == '0') {
            pos++;
        } else {
            if (!isDigit19(src[pos])) throw new Exception("bad number");
            while (pos < src.length && isDigit(src[pos])) pos++;
        }
        if (pos < src.length && src[pos] == '.') {
            pos++;
            if (pos >= src.length || !isDigit(src[pos])) throw new Exception("bad fraction");
            while (pos < src.length && isDigit(src[pos])) pos++;
        }
        if (pos < src.length && (src[pos] == 'e' || src[pos] == 'E')) {
            pos++;
            if (pos < src.length && (src[pos] == '+' || src[pos] == '-')) pos++;
            if (pos >= src.length || !isDigit(src[pos])) throw new Exception("bad exponent");
            while (pos < src.length && isDigit(src[pos])) pos++;
        }
    }

    private void parseArray() throws Exception {
        expect('[');
        skipWs();
        if (pos < src.length && src[pos] == ']') { pos++; return; }
        while (true) {
            parseValue();
            skipWs();
            if (pos >= src.length) throw new Exception("eof in array");
            if (src[pos] == ']') { pos++; return; }
            if (src[pos] != ',') throw new Exception("expected , or ]");
            pos++;
            skipWs();
        }
    }

    private void parseObject() throws Exception {
        expect('{');
        skipWs();
        if (pos < src.length && src[pos] == '}') { pos++; return; }
        while (true) {
            skipWs();
            parseString();
            skipWs();
            expect(':');
            skipWs();
            parseValue();
            skipWs();
            if (pos >= src.length) throw new Exception("eof in object");
            if (src[pos] == '}') { pos++; return; }
            if (src[pos] != ',') throw new Exception("expected , or }");
            pos++;
        }
    }

    private void matchLiteral(String lit) throws Exception {
        byte[] bytes = lit.getBytes();
        if (pos + bytes.length > src.length) throw new Exception("eof");
        for (byte b : bytes) {
            if (src[pos++] != b) throw new Exception("literal mismatch");
        }
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    private void skipWs() {
        while (pos < src.length) {
            byte c = src[pos];
            if (c == ' ' || c == '\t' || c == '\r' || c == '\n') pos++;
            else break;
        }
    }

    private void expect(char ch) throws Exception {
        if (pos >= src.length || src[pos] != (byte)ch)
            throw new Exception("expected '" + ch + "'");
        pos++;
    }

    private static boolean isDigit(byte c)   { return c >= '0' && c <= '9'; }
    private static boolean isDigit19(byte c) { return c >= '1' && c <= '9'; }
    private static boolean isHex(byte c) {
        return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
    }

    private static byte[] b(String s) { return s.getBytes(); }

    // -----------------------------------------------------------------------
    // Main
    // -----------------------------------------------------------------------

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new JavaTarget());
    }
}
