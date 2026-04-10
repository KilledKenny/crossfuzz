package crossfuzz;

import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.RandomAccessFile;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.MappedByteBuffer;
import java.nio.channels.FileChannel;
import java.nio.charset.StandardCharsets;

public class Harness {

    // Shared memory layout — must match pkg/coverage/shmem.go
    static final int OFF_INPUT_LEN   = 8;
    static final int OFF_OUTPUT_LEN  = 12;
    static final int OFF_STATUS      = 16;
    static final int INPUT_OFFSET    = 64;
    static final int OUTPUT_OFFSET   = 64 + 1_048_576;
    static final int COVERAGE_OFFSET = 64 + 1_048_576 + 1_048_576;
    static final int TOTAL_SHM_SIZE  = COVERAGE_OFFSET + 65_536;

    static final int STATUS_OK    = 0;
    static final int STATUS_ERROR = 1;

    static final int MAX_OUTPUT = 1_048_576;
    static final int MAX_MSG    = 1 << 20;

    public static void run(Target target) throws Exception {
        String shmPath = System.getenv("CROSSFUZZ_SHM");
        if (shmPath == null) {
            throw new RuntimeException("CROSSFUZZ_SHM not set");
        }

        // Map shared memory
        RandomAccessFile raf = new RandomAccessFile(shmPath, "rw");
        FileChannel shmChannel = raf.getChannel();
        MappedByteBuffer shm = shmChannel.map(
            FileChannel.MapMode.READ_WRITE, 0, TOTAL_SHM_SIZE);
        shm.order(ByteOrder.LITTLE_ENDIAN);

        // Slice out the coverage bitmap region and hand it to CoverageRuntime
        ByteBuffer covSlice = shm.duplicate();
        covSlice.order(ByteOrder.LITTLE_ENDIAN);
        covSlice.position(COVERAGE_OFFSET);
        covSlice.limit(COVERAGE_OFFSET + 65_536);
        CoverageRuntime.init(covSlice.slice());

        // Open inherited pipe FDs
        FileInputStream  cmdIn  = new FileInputStream("/proc/self/fd/3");
        FileOutputStream respOut = new FileOutputStream("/proc/self/fd/4");

        // Handshake
        writeMsg(respOut, "{\"type\":\"ready\"}");

        // Main loop
        while (true) {
            String msg = readMsg(cmdIn);
            if (msg == null) break;

            String type = parseType(msg);

            if ("shutdown".equals(type)) {
                break;
            } else if ("fuzz".equals(type)) {
                int inputLen = shm.getInt(OFF_INPUT_LEN);
                byte[] input = new byte[inputLen];
                shm.position(INPUT_OFFSET);
                shm.get(input);

                byte[] output;
                int status = STATUS_OK;
                try {
                    output = target.fuzz(input);
                    if (output == null) output = new byte[0];
                } catch (Exception e) {
                    output = new byte[0];
                    status = STATUS_ERROR;
                }

                int outLen = Math.min(output.length, MAX_OUTPUT);
                shm.putInt(OFF_OUTPUT_LEN, outLen);
                shm.putInt(OFF_STATUS, status);
                shm.position(OUTPUT_OFFSET);
                shm.put(output, 0, outLen);

                writeMsg(respOut, "{\"type\":\"fuzz_result\",\"ok\":true,\"exec_ns\":0}");
            }
            // ignore unknown message types
        }

        shmChannel.close();
        raf.close();
    }

    private static String readMsg(InputStream in) throws IOException {
        byte[] lenBytes = new byte[4];
        int total = 0;
        while (total < 4) {
            int n = in.read(lenBytes, total, 4 - total);
            if (n < 0) return null;
            total += n;
        }
        int len = ((lenBytes[0] & 0xFF) << 24)
                | ((lenBytes[1] & 0xFF) << 16)
                | ((lenBytes[2] & 0xFF) << 8)
                |  (lenBytes[3] & 0xFF);
        if (len <= 0 || len > MAX_MSG) return null;

        byte[] payload = new byte[len];
        total = 0;
        while (total < len) {
            int n = in.read(payload, total, len - total);
            if (n < 0) return null;
            total += n;
        }
        return new String(payload, StandardCharsets.UTF_8);
    }

    private static void writeMsg(OutputStream out, String json) throws IOException {
        byte[] payload = json.getBytes(StandardCharsets.UTF_8);
        int plen = payload.length;
        byte[] header = {
            (byte)(plen >> 24), (byte)(plen >> 16),
            (byte)(plen >> 8),  (byte)(plen)
        };
        out.write(header);
        out.write(payload);
        out.flush();
    }

    // Minimal JSON type extraction — no external dependency needed.
    // Handles {"type":"fuzz",...} and {"type":"shutdown"}.
    static String parseType(String msg) {
        int start = msg.indexOf("\"type\"");
        if (start < 0) return "";
        int colon = msg.indexOf(':', start + 6);
        if (colon < 0) return "";
        int q1 = msg.indexOf('"', colon + 1);
        if (q1 < 0) return "";
        int q2 = msg.indexOf('"', q1 + 1);
        if (q2 < 0) return "";
        return msg.substring(q1 + 1, q2);
    }
}
