# Java Harness — Complete Reference

## Settings

```java
Crossfuzz.Settings settings = new Crossfuzz.Settings();
settings.instrument = false;  // disable coverage (thin HTTP client)
settings.transform  = true;   // filter mode: returned bytes replace input
Crossfuzz.fuzz(target, settings);
```

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | `true` | Enable JaCoCo-based coverage collection |
| `warmup` | `0` | Reserved |
| `transform` | `false` | Filter mode: when true, returned bytes replace the original input |
| `hinting` | `false` | Reserved |

## Filter target

```java
import crossfuzz.Crossfuzz;

public class MyFilter {
    public static void main(String[] args) throws Exception {
        Crossfuzz.filter(input -> {
            // Return FilterResult(output, accepted).
            // output only used when settings.transform = true.
            if (input.length < 4) {
                return new Crossfuzz.FilterResult(null, false);  // reject
            }
            return new Crossfuzz.FilterResult(input, true);       // accept
        });
    }
}
```

### FilterResult

```java
public static class FilterResult {
    public final byte[] output;
    public final boolean accepted;

    public FilterResult(byte[] output, boolean accepted) { ... }
}
```

Configure in `crossfuzz.toml` as `[input_filter]` (not as a `[[target]]`).

## Compare target

```java
import crossfuzz.Crossfuzz;

public class MyComparator {
    public static void main(String[] args) throws Exception {
        Crossfuzz.compare((input, targetNames, targetOutputs) -> {
            // Return null or "" for match, non-empty string for mismatch.
            if (targetOutputs.length < 2) return null;
            if (!java.util.Arrays.equals(targetOutputs[0], targetOutputs[1])) {
                return targetNames[0] + " and " + targetNames[1] + " differ";
            }
            return null;
        });
    }
}
```

### CompareTarget interface

```java
@FunctionalInterface
public interface CompareTarget {
    String compare(byte[] input, String[] targetNames, byte[][] targetOutputs) throws Exception;
}
```

Configure in `crossfuzz.toml` as `[comparator] type = "harness"`.

## Server mode

```java
import crossfuzz.Crossfuzz;

public class MyServer {
    public static void main(String[] args) throws Exception {
        // Call once during initialization:
        Crossfuzz.initServer();  // opens SHM + starts instrumentation (no-op if CROSSFUZZ_SHM not set)

        // Before handling each request:
        Crossfuzz.clearInstrumentation();

        // ... handle request ...

        // After response is complete:
        Crossfuzz.collectInstrumentation();
    }
}
```

Configure the server in `crossfuzz.toml` as `type = "server"`:

```toml
[[target]]
name = "java_api"
type = "server"
language = "java"
binary = "java"
args = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
        "-cp", "../../harness/java/build/libs/crossfuzz.jar:.", "MyServer"]
```

## Gradle project layout

For larger targets, set up a Gradle project alongside the harness:

```
my_target/
├── build.gradle
└── src/main/java/MyTarget.java
```

`build.gradle`:
```groovy
plugins {
    id 'java'
    id 'application'
}

application {
    mainClass = 'MyTarget'
}

dependencies {
    implementation files('../../harness/java/build/libs/crossfuzz.jar')
}
```

Build and run config:
```toml
[[target]]
name = "java_impl"
language = "java"
binary = "java"
args = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
        "-cp", "../../harness/java/build/libs/crossfuzz.jar:build/libs/my_target.jar", "MyTarget"]
build_cmd = "cd ../../harness/java && gradle jar && cd - && gradle jar"
```

## Full example: JSON parser (Java)

From `examples/json_parse/JavaTarget.java`:

```java
import crossfuzz.Crossfuzz;

public class JavaTarget implements Crossfuzz.FuzzTarget {
    private byte[] src;
    private int    pos;

    @Override
    public byte[] fuzz(byte[] input) {
        src = input;
        pos = 0;
        try {
            String type = parseValue();
            skipWs();
            if (pos != src.length) return b("error");
            return b(type);
        } catch (Exception e) {
            return b("error");
        }
    }

    // ... parser implementation ...

    private static byte[] b(String s) { return s.getBytes(java.nio.charset.StandardCharsets.UTF_8); }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new JavaTarget());
    }
}
```

Build command from `examples/json_parse/crossfuzz.toml`:

```bash
cd ../../harness/java && gradle jar && cd - && javac -cp ../../harness/java/build/libs/crossfuzz.jar JavaTarget.java
```

## Common pitfalls

- **Missing `-javaagent`**: binary runs but produces no coverage — fuzzer never discovers new inputs.
- **JVM startup time**: not a concern — the harness uses persistent mode, so the JVM starts once and runs thousands of iterations.
- **`/proc/self/fd/3` and `/proc/self/fd/4`**: the Java harness uses these paths to open the pipes. Requires Linux (works on all standard Linux distros).
- **Classpath order**: `crossfuzz.jar` must appear before your target classes in `-cp` for the agent to instrument them.
- **Java version**: requires JDK 11+ (uses `FileChannel.map` and functional interfaces).
