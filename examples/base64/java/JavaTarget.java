import io.killedkenny.crossfuzz.Crossfuzz;

public class JavaTarget implements Crossfuzz.FuzzTarget {

    private static final byte[] ALPHABET =
        "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".getBytes();

    @Override
    public byte[] fuzz(byte[] input) {
        int outLen = 4 * ((input.length + 2) / 3);
        byte[] out = new byte[outLen];

        int i = 0, j = 0;
        while (i < input.length) {
            int a = input[i++] & 0xFF;
            int b = i < input.length ? input[i++] & 0xFF : 0;
            int c = i < input.length ? input[i++] & 0xFF : 0;
            int triple = (a << 16) | (b << 8) | c;

            out[j++] = ALPHABET[(triple >>> 18) & 0x3F];
            out[j++] = ALPHABET[(triple >>> 12) & 0x3F];
            out[j++] = ALPHABET[(triple >>> 6)  & 0x3F];
            out[j++] = ALPHABET[ triple         & 0x3F];
        }

        int mod = input.length % 3;
        if (mod > 0) {
            out[outLen - 1] = '=';
            if (mod == 1) out[outLen - 2] = '=';
        }

        return out;
    }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new JavaTarget());
    }
}
