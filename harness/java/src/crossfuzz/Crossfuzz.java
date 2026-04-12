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
import java.util.HashMap;
import java.util.Map;
import java.util.ArrayList;
import java.util.List;

public class Crossfuzz {

    // Shared memory layout — must match pkg/coverage/shmem.go
    static final int OFF_INPUT_LEN   = 8;
    static final int OFF_OUTPUT_LEN  = 12;
    static final int OFF_STATUS      = 16;
    static final int INPUT_OFFSET    = 64;
    static final int INPUT_SIZE      = 1_048_576;
    static final int OUTPUT_OFFSET   = 64 + 1_048_576;
    static final int OUTPUT_SIZE     = 1_048_576;
    static final int COVERAGE_OFFSET = 64 + 1_048_576 + 1_048_576;
    static final int TOTAL_SHM_SIZE  = COVERAGE_OFFSET + 65_536;

    static final int STATUS_OK    = 0;
    static final int STATUS_ERROR = 1;

    static final int MAX_OUTPUT = 1_048_576;
    static final int MAX_MSG    = 1 << 20;

    // Shared memory buffer set by openShm()
    private static volatile MappedByteBuffer shm;

    // ---- Settings ----

    public static class Settings {
        public boolean instrument = true;
        public int warmup = 0;
        public boolean transform = false;
        public boolean hinting = false;
    }

    // ---- Target interfaces ----

    @FunctionalInterface
    public interface FuzzTarget {
        byte[] fuzz(byte[] input) throws Exception;
    }

    @FunctionalInterface
    public interface FilterTarget {
        FilterResult filter(byte[] input) throws Exception;
    }

    public static class FilterResult {
        public final byte[] output;
        public final boolean accepted;

        public FilterResult(byte[] output, boolean accepted) {
            this.output = output;
            this.accepted = accepted;
        }
    }

    @FunctionalInterface
    public interface CompareTarget {
        String compare(byte[] input, String[] targetNames, byte[][] targetOutputs) throws Exception;
    }

    // ---- Standalone functions ----

    public static void openShm() throws Exception {
        String shmPath = System.getenv("CROSSFUZZ_SHM");
        if (shmPath == null) throw new RuntimeException("CROSSFUZZ_SHM not set");
        RandomAccessFile raf = new RandomAccessFile(shmPath, "rw");
        FileChannel ch = raf.getChannel();
        MappedByteBuffer mapped = ch.map(FileChannel.MapMode.READ_WRITE, 0, TOTAL_SHM_SIZE);
        mapped.order(ByteOrder.LITTLE_ENDIAN);
        shm = mapped;
    }

    public static void startInstrumentation() {
        if (shm == null) return;
        ByteBuffer covSlice = shm.duplicate();
        covSlice.order(ByteOrder.LITTLE_ENDIAN);
        covSlice.position(COVERAGE_OFFSET);
        covSlice.limit(COVERAGE_OFFSET + 65_536);
        CoverageRuntime.init(covSlice.slice());
    }

    public static void clearInstrumentation() {
        CoverageRuntime.clear();
    }

    public static void collectInstrumentation() {
        CoverageRuntime.collect();
    }

    public static void setStatus(int status) {
        if (shm != null) shm.putInt(OFF_STATUS, status);
    }

    // ---- Server convenience ----

    public static void initServer() throws Exception {
        String shmPath = System.getenv("CROSSFUZZ_SHM");
        if (shmPath == null) return;
        openShm();
        startInstrumentation();
    }

    // ---- Fuzz ----

    public static void fuzz(FuzzTarget target) throws Exception {
        fuzz(target, new Settings());
    }

    public static void fuzz(FuzzTarget target, Settings settings) throws Exception {
        if (shm == null) {
            openShm();
        }
        if (settings.instrument) {
            startInstrumentation();
        } else {
            CoverageRuntime.disable();
        }

        FileInputStream cmdIn = new FileInputStream("/proc/self/fd/3");
        FileOutputStream respOut = new FileOutputStream("/proc/self/fd/4");

        writeMsg(respOut, "{\"type\":\"ready\"}");

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

                writeMsg(respOut, "{\"type\":\"fuzz_result\",\"ok\":true}");
            }
        }
    }

    // ---- Filter ----

    public static void filter(FilterTarget target) throws Exception {
        filter(target, new Settings());
    }

    public static void filter(FilterTarget target, Settings settings) throws Exception {
        if (shm == null) {
            openShm();
        }
        if (settings.instrument) {
            startInstrumentation();
        }

        FileInputStream cmdIn = new FileInputStream("/proc/self/fd/3");
        FileOutputStream respOut = new FileOutputStream("/proc/self/fd/4");

        writeMsg(respOut, "{\"type\":\"ready\"}");

        while (true) {
            String msg = readMsg(cmdIn);
            if (msg == null) break;
            String type = parseType(msg);

            if ("shutdown".equals(type)) {
                break;
            } else if ("filter".equals(type)) {
                int inputLen = shm.getInt(OFF_INPUT_LEN);
                byte[] input = new byte[inputLen];
                shm.position(INPUT_OFFSET);
                shm.get(input);

                FilterResult result;
                try {
                    result = target.filter(input);
                } catch (Exception e) {
                    result = new FilterResult(null, false);
                }

                if (result.accepted) {
                    if (settings.transform && result.output != null && result.output.length > 0) {
                        int outLen = Math.min(result.output.length, MAX_OUTPUT);
                        shm.putInt(OFF_OUTPUT_LEN, outLen);
                        shm.position(OUTPUT_OFFSET);
                        shm.put(result.output, 0, outLen);
                    } else {
                        // Copy input to output region
                        int outLen = Math.min(inputLen, MAX_OUTPUT);
                        shm.putInt(OFF_OUTPUT_LEN, outLen);
                        shm.position(OUTPUT_OFFSET);
                        shm.put(input, 0, outLen);
                    }
                    writeMsg(respOut, "{\"type\":\"filter_result\",\"ok\":true}");
                } else {
                    shm.putInt(OFF_OUTPUT_LEN, 0);
                    writeMsg(respOut, "{\"type\":\"filter_result\",\"ok\":false}");
                }
            }
        }
    }

    // ---- Compare ----

    public static void compare(CompareTarget target) throws Exception {
        compare(target, new Settings());
    }

    public static void compare(CompareTarget target, Settings settings) throws Exception {
        String targetsJson = System.getenv("CROSSFUZZ_SHM_TARGETS");
        if (targetsJson == null) {
            throw new RuntimeException("CROSSFUZZ_SHM_TARGETS not set");
        }

        // Parse {"name":"path",...} — minimal JSON parser
        Map<String, MappedByteBuffer> targetShms = new HashMap<>();
        targetsJson = targetsJson.trim();
        if (targetsJson.startsWith("{")) targetsJson = targetsJson.substring(1);
        if (targetsJson.endsWith("}")) targetsJson = targetsJson.substring(0, targetsJson.length() - 1);

        String[] pairs = targetsJson.split(",");
        for (String pair : pairs) {
            pair = pair.trim();
            if (pair.isEmpty()) continue;
            String[] kv = pair.split(":", 2);
            if (kv.length != 2) continue;
            String name = kv[0].trim().replace("\"", "");
            String path = kv[1].trim().replace("\"", "");

            RandomAccessFile raf = new RandomAccessFile(path, "r");
            FileChannel ch = raf.getChannel();
            MappedByteBuffer mapped = ch.map(FileChannel.MapMode.READ_ONLY, 0, TOTAL_SHM_SIZE);
            mapped.order(ByteOrder.LITTLE_ENDIAN);
            targetShms.put(name, mapped);
        }

        FileInputStream cmdIn = new FileInputStream("/proc/self/fd/3");
        FileOutputStream respOut = new FileOutputStream("/proc/self/fd/4");

        writeMsg(respOut, "{\"type\":\"ready\"}");

        while (true) {
            String msg = readMsg(cmdIn);
            if (msg == null) break;
            String type = parseType(msg);

            if ("shutdown".equals(type)) {
                break;
            } else if ("compare".equals(type)) {
                String[] reqTargets = parseTargetsArray(msg);

                byte[] input = null;
                List<String> namesList = new ArrayList<>();
                List<byte[]> outputsList = new ArrayList<>();

                for (String name : reqTargets) {
                    MappedByteBuffer tshm = targetShms.get(name);
                    if (tshm == null) continue;

                    if (input == null) {
                        int inLen = tshm.getInt(OFF_INPUT_LEN);
                        if (inLen > INPUT_SIZE) inLen = INPUT_SIZE;
                        input = new byte[inLen];
                        tshm.position(INPUT_OFFSET);
                        tshm.get(input);
                    }

                    int outLen = tshm.getInt(OFF_OUTPUT_LEN);
                    if (outLen > OUTPUT_SIZE) outLen = OUTPUT_SIZE;
                    byte[] output = new byte[outLen];
                    tshm.position(OUTPUT_OFFSET);
                    tshm.get(output);

                    namesList.add(name);
                    outputsList.add(output);
                }

                if (input == null) input = new byte[0];

                String mismatch;
                try {
                    mismatch = target.compare(input,
                        namesList.toArray(new String[0]),
                        outputsList.toArray(new byte[0][]));
                } catch (Exception e) {
                    mismatch = "compare exception: " + e.getMessage();
                }

                if (mismatch != null && !mismatch.isEmpty()) {
                    String escaped = mismatch.replace("\\", "\\\\")
                                             .replace("\"", "\\\"")
                                             .replace("\n", "\\n");
                    writeMsg(respOut, "{\"type\":\"compare_result\",\"error\":\"" + escaped + "\"}");
                } else {
                    writeMsg(respOut, "{\"type\":\"compare_result\"}");
                }
            }
        }
    }

    // ---- Protocol helpers ----

    static String readMsg(InputStream in) throws IOException {
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

    static void writeMsg(OutputStream out, String json) throws IOException {
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

    static String[] parseTargetsArray(String msg) {
        int idx = msg.indexOf("\"targets\"");
        if (idx < 0) return new String[0];
        int bracket = msg.indexOf('[', idx);
        if (bracket < 0) return new String[0];
        int end = msg.indexOf(']', bracket);
        if (end < 0) return new String[0];
        String inner = msg.substring(bracket + 1, end);
        List<String> result = new ArrayList<>();
        for (String part : inner.split(",")) {
            part = part.trim().replace("\"", "");
            if (!part.isEmpty()) result.add(part);
        }
        return result.toArray(new String[0]);
    }
}
