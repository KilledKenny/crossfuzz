# Java Harness

The harness is published to Maven Central as `io.killedkenny.crossfuzz:crossfuzz`. Add it to your existing Maven or Gradle project — it serves as both the javaagent (coverage) and classpath dependency (harness API).

## Setup

### Maven — `pom.xml`

```xml
<dependency>
  <groupId>io.killedkenny.crossfuzz</groupId>
  <artifactId>crossfuzz</artifactId>
  <version>0.0.1</version>
</dependency>
```

The jar needs to be available as a local file for `-javaagent`. Add this to `<build><plugins>` to copy it alongside compiled output during `mvn compile`:

```xml
<plugin>
  <groupId>org.apache.maven.plugins</groupId>
  <artifactId>maven-dependency-plugin</artifactId>
  <executions>
    <execution>
      <phase>compile</phase>
      <goals><goal>copy-dependencies</goal></goals>
      <configuration>
        <includeArtifactIds>crossfuzz</includeArtifactIds>
        <outputDirectory>${project.basedir}</outputDirectory>
        <stripVersion>true</stripVersion>
      </configuration>
    </execution>
  </executions>
</plugin>
```

### Gradle — `build.gradle`

Requires `mavenCentral()` in `repositories`. Add to your existing `build.gradle`:

```groovy
configurations { crossfuzzAgent }
dependencies { crossfuzzAgent('io.killedkenny.crossfuzz:crossfuzz:0.0.1') { transitive = false } }

tasks.register('downloadAgent', Copy) {
    from configurations.crossfuzzAgent
    into '.'
    rename '.*', 'crossfuzz.jar'
}
```

## Fuzz target

```java
import io.killedkenny.crossfuzz.Crossfuzz;

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

## TOML config entry

### Maven

```toml
[[target]]
name = "java_impl"
language = "java"
binary = "java"
args = ["-javaagent:crossfuzz.jar", "-cp", "crossfuzz.jar:target/classes", "MyTarget"]
build_cmd = "mvn compile"
```

### Gradle

```toml
[[target]]
name = "java_impl"
language = "java"
binary = "java"
args = ["-javaagent:crossfuzz.jar", "-cp", "crossfuzz.jar:build/classes/java/main", "MyTarget"]
build_cmd = "gradle downloadAgent compileJava"
```

The `-javaagent` flag activates coverage instrumentation. Without it the binary runs but produces no coverage signal.

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
import io.killedkenny.crossfuzz.Crossfuzz;

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
import io.killedkenny.crossfuzz.Crossfuzz;

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
import io.killedkenny.crossfuzz.Crossfuzz;
import io.killedkenny.crossfuzz.CoverageRuntime;

public class MyServer {
    public static void main(String[] args) throws Exception {
        Crossfuzz.initServer();  // opens SHM + starts instrumentation (no-op if CROSSFUZZ_SHM not set)

        // Before handling each request:
        CoverageRuntime.clear();

        // ... handle request ...

        // After response is complete:
        CoverageRuntime.collect();
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
args = ["-javaagent:crossfuzz.jar", "-cp", "crossfuzz.jar:target/classes", "MyServer"]
```

## Coverage scope

The javaagent instruments all classes loaded by the **application classloader** — this includes your target code and any third-party libraries (Guava, Jackson, Apache Commons, etc.). No extra configuration is needed.

**JDK classes** (`java.*`, `javax.*`, `sun.*`, `jdk.*`) are loaded by the bootstrap classloader and are intentionally excluded. The coverage runtime itself uses `ByteBuffer` internally; instrumenting those same classes would cause infinite recursion. This is the same limitation accepted by Jazzer and JQF.

If you need coverage from JDK-delegating code (e.g. `java.util.Base64`), implement the logic yourself. The `examples/base64` Java target shows this pattern: a hand-rolled base64 encoder instead of delegating to `java.util.Base64`.

## Common pitfalls

- **Missing `-javaagent`**: binary runs but produces no coverage.
- **Classpath order**: `crossfuzz.jar` must appear before your target classes in `-cp` for the agent to instrument them.
- **Java version**: requires JDK 11+.
