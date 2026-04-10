package crossfuzz;

import java.lang.instrument.Instrumentation;

public class CoverageAgent {
    public static void premain(String agentArgs, Instrumentation inst) {
        inst.addTransformer(new CoverageTransformer(), true);
    }
}
