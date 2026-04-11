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

    /** Zeroes the coverage bitmap so that only edges from the current
     *  iteration are attributed. Call this before the code path to observe. */
    public static void clear() {
        ByteBuffer b = bitmap;
        if (b == null) return;
        for (int i = 0, n = b.capacity(); i < n; i++) b.put(i, (byte) 0);
    }

    /** Ensures coverage accumulated since the last clear() is visible in
     *  shared memory. For Java, hit() writes directly to the mapped region,
     *  so this is a no-op; it exists for API symmetry with the Go harness. */
    public static void collect() {
        // hit() writes directly to the MappedByteBuffer; no snapshot step needed.
    }
}
