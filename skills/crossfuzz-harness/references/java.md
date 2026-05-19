# Java Harness

## JAR

The harness is distributed as a single JAR built with Gradle:

```bash
cd harness/java && gradle jar
# Produces: harness/java/build/libs/crossfuzz.jar
```

The JAR serves as both the agent (`-javaagent:crossfuzz.jar`) and classpath dependency (`-cp crossfuzz.jar`).

## Fuzz target

```java
import crossfuzz.Crossfuzz;

public class MyTarget implements Crossfuzz.FuzzTarget {
    @Override
    public byte[] fuzz(byte[] input) throws Exception {
        // Throw to mark execution as error-status.
        return input;
    }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new MyTarget());
    }
}
```

Or with a lambda:

```java
Crossfuzz.fuzz(input -> { return input; });
```

### Interface

```java
@FunctionalInterface
public interface FuzzTarget {
    byte[] fuzz(byte[] input) throws Exception;
}
```

## Build and run

```bash
cd ../../harness/java && gradle jar
cd -
javac -cp ../../harness/java/build/libs/crossfuzz.jar MyTarget.java
```

The `-javaagent` flag activates coverage instrumentation. Without it the binary runs but produces no coverage signal.

## TOML config entry

```toml
[[target]]
name = "java_impl"
language = "java"
binary = "java"
args = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
        "-cp", "../../harness/java/build/libs/crossfuzz.jar:.", "MyTarget"]
build_cmd = "cd ../../harness/java && gradle jar && cd - && javac -cp ../../harness/java/build/libs/crossfuzz.jar MyTarget.java"
```

## Settings

```java
Crossfuzz.Settings settings = new Crossfuzz.Settings();
settings.instrument = false;  // disable coverage (thin HTTP client)
settings.transform  = true;   // filter mode: returned bytes replace input
Crossfuzz.fuzz(target, settings);
```

| Field | Default | Description |
|-------|---------|-------------|
| `instrument` | `true` | Enable coverage collection via the javaagent |
| `transform` | `false` | Filter mode: when true, returned bytes replace the original input |

## Filter target

```java
import crossfuzz.Crossfuzz;

public class MyFilter {
    public static void main(String[] args) throws Exception {
        Crossfuzz.filter(input -> {
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

Configure as `[input_filter]`.

## Compare target

```java
import crossfuzz.Crossfuzz;

public class MyComparator {
    public static void main(String[] args) throws Exception {
        Crossfuzz.compare((input, targetNames, targetOutputs) -> {
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

Configure as `[comparator] type = "harness"`.

## Server mode

```java
import crossfuzz.Crossfuzz;

public class MyServer {
    public static void main(String[] args) throws Exception {
        Crossfuzz.initServer();  // opens SHM + starts instrumentation (no-op if CROSSFUZZ_SHM not set)

        // Before handling each request:
        Crossfuzz.clearInstrumentation();

        // ... handle request ...

        // After response is complete:
        Crossfuzz.collectInstrumentation();
    }
}
```

Configure in `crossfuzz.toml` as `type = "server"`:

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

application { mainClass = 'MyTarget' }

dependencies {
    implementation files('../../harness/java/build/libs/crossfuzz.jar')
}
```

## Common pitfalls

- **Missing `-javaagent`**: binary runs but produces no coverage.
- **`/proc/self/fd/3` and `/proc/self/fd/4`**: the Java harness uses these to open pipes; requires Linux.
- **Classpath order**: `crossfuzz.jar` must appear before your target classes in `-cp` for the agent to instrument them.
- **Java version**: requires JDK 11+.
