import crossfuzz.Crossfuzz;
import java.net.URL;
import java.nio.charset.StandardCharsets;

/**
 * URL parse target for cross-language differential fuzzing.
 *
 * Parses the input as a URL using java.net.URL and returns the normalised
 * components so they can be compared with Go's net/url output.
 */
public class JavaTarget implements Crossfuzz.FuzzTarget {

    @Override
    public byte[] fuzz(byte[] input) {
        try {
            String s = new String(input, StandardCharsets.UTF_8);
            URL u = new URL("http://example.com/"+s);

            String scheme   = lower(u.getProtocol());
            String host     = lower(u.getHost());
            String port     = u.getPort() == -1 ? "" : String.valueOf(u.getPort());
            String path     = nvl(u.getPath());
            String query    = nvl(u.getQuery());
            String fragment = nvl(u.getRef());

            if (host.length() == 0 || path.length()==0  || query.length() == 0){
                	return "error".getBytes(StandardCharsets.UTF_8);
            }

            return ("scheme=" + scheme
                  + "|host="     + host
                  + "|port="     + port
                  + "|path="    // + path
                  + "|query="    + query
                  + "|fragment=" + fragment).getBytes(StandardCharsets.UTF_8);
/*
            return ("scheme=" + scheme
                  + "|host="     + host
                  + "|port="     + port
                  + "|path="     + path
                  + "|query="    + query
                  + "|fragment=" + fragment).getBytes(StandardCharsets.UTF_8);
                  */
        } catch (Exception e) {
            return "error".getBytes(StandardCharsets.UTF_8);
        }
    }

    private static String lower(String s) { return s == null ? "" : s.toLowerCase(); }
    private static String nvl(String s)   { return s == null ? "" : s; }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new JavaTarget());
    }
}
