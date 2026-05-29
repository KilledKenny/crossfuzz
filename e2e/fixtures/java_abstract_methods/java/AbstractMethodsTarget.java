import io.killedkenny.crossfuzz.Crossfuzz;

/**
 * Fuzz target whose class hierarchy includes abstract methods.
 *
 * When the -javaagent instruments this class it loads both inner classes
 * through the application class loader:
 *   AbstractMethodsTarget$Processor      (abstract — has abstract methods)
 *   AbstractMethodsTarget$ReverseProcessor (concrete — must be instrumented)
 *
 * Before the CoverageTransformer fix, loading Processor would produce a
 * ClassFormatError because the transformer inserted a Code attribute into the
 * abstract methods, which the JVM forbids (JVMS §4.7.3). After the fix the
 * abstract methods are skipped and the concrete ones are still instrumented.
 */
public class AbstractMethodsTarget implements Crossfuzz.FuzzTarget {

    abstract static class Processor {
        abstract byte[] process(byte[] input);
        abstract String describe();
    }

    static class ReverseProcessor extends Processor {
        @Override
        public byte[] process(byte[] input) {
            byte[] out = new byte[input.length];
            for (int i = 0; i < input.length; i++) {
                out[i] = input[input.length - 1 - i];
            }
            return out;
        }

        @Override
        public String describe() { return "reverser"; }
    }

    private final Processor proc = new ReverseProcessor();

    @Override
    public byte[] fuzz(byte[] input) {
        return proc.process(input);
    }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new AbstractMethodsTarget());
    }
}
