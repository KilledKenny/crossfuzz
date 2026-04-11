package crossfuzz;

import java.nio.ByteBuffer;

public class CoverageRuntime {
    private static volatile ByteBuffer bitmap;

    static void init(ByteBuffer b) {
        bitmap = b;
    }

    /** Disables coverage collection by discarding the bitmap reference. */
    static void disable() {
        bitmap = null;
    }

    public static void hit(int index) {
        ByteBuffer b = bitmap;
        if (b == null) return;
        int idx = index & 0xFFFF;
        int cur = b.get(idx) & 0xFF;
        if (cur < 255) b.put(idx, (byte)(cur + 1));
    }
}
