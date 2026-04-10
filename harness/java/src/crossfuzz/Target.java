package crossfuzz;

public interface Target {
    byte[] fuzz(byte[] input) throws Exception;
}
