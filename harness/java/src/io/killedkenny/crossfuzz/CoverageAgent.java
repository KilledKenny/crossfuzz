package io.killedkenny.crossfuzz;

import java.io.RandomAccessFile;
import java.lang.instrument.Instrumentation;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.MappedByteBuffer;
import java.nio.channels.FileChannel;

public class CoverageAgent {
    public static void premain(String agentArgs, Instrumentation inst) {
        System.err.println("[crossfuzz] agent: premain loaded");
        // Initialize the coverage bitmap before any application class is loaded.
        // CROSSFUZZ_SHM is already set by the coordinator before spawning the JVM.
        String shmPath = System.getenv("CROSSFUZZ_SHM");
        if (shmPath != null) {
            try {
                RandomAccessFile raf = new RandomAccessFile(shmPath, "rw");
                FileChannel ch = raf.getChannel();
                MappedByteBuffer shm = ch.map(
                    FileChannel.MapMode.READ_WRITE, 0, Crossfuzz.TOTAL_SHM_SIZE);
                ByteBuffer dup = shm.duplicate();
                dup.order(ByteOrder.LITTLE_ENDIAN);
                dup.position(Crossfuzz.COVERAGE_OFFSET);
                dup.limit(Crossfuzz.COVERAGE_OFFSET + 65_536);
                CoverageRuntime.init(dup.slice());
            } catch (Exception e) {
                System.err.println("[crossfuzz] agent: failed to map SHM: " + e);
            }
        } else {
            System.err.println("[crossfuzz] agent: CROSSFUZZ_SHM not set — no coverage bitmap");
        }
        inst.addTransformer(new CoverageTransformer(), true);
    }
}
