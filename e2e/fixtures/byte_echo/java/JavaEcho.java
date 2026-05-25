import io.killedkenny.crossfuzz.Crossfuzz;

public class JavaEcho implements Crossfuzz.FuzzTarget {

    @Override
    public byte[] fuzz(byte[] input) {
        byte[] out = new byte[input.length];
        for (int i = 0; i < input.length; i++) {
            int b = input[i] & 0xFF;
            if (b < 0x20)      out[i] = (byte) b;
            else if (b < 0x40) out[i] = (byte) b;
            else if (b < 0x60) out[i] = (byte) b;
            else if (b < 0x80) out[i] = (byte) b;
            else               out[i] = (byte) b;
        }
        return out;
    }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new JavaEcho());
    }
}
