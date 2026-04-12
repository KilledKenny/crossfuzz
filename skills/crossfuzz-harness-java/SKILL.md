---
name: crossfuzz-harness-java
description: Use this skill when the user is writing a Java target for cross_fuzz, needs to know the Java harness API, wants to know how to build and run a Java target with the javaagent, or is setting up a Java fuzzing target. Trigger for questions like "how do I write a Java target?", "how do I build the Java harness?", "what javaagent flag do I need?", "how do I implement FuzzTarget?", "how do I run a Java target with cross_fuzz?", or "how do I write a Java filter or comparator?".
---

# Java Harness

## JAR

The harness is distributed as a single JAR built with Gradle:

```bash
cd harness/java && gradle jar
# Produces: harness/java/build/libs/crossfuzz.jar
```

The JAR serves as both the agent (`-javaagent:crossfuzz.jar`) and the classpath dependency (`-cp crossfuzz.jar`).

## Fuzz target

```java
import crossfuzz.Crossfuzz;

public class MyTarget implements Crossfuzz.FuzzTarget {
    @Override
    public byte[] fuzz(byte[] input) throws Exception {
        // Process input, return output bytes.
        // Throw any exception to mark execution as error-status.
        return input;
    }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new MyTarget());
    }
}
```

Or with a lambda (Java 8+):

```java
import crossfuzz.Crossfuzz;

public class MyTarget {
    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(input -> {
            // process and return result
            return input;
        });
    }
}
```

### Interface

```java
@FunctionalInterface
public interface FuzzTarget {
    byte[] fuzz(byte[] input) throws Exception;
}
```

## Build and run

### Build (compile target + harness JAR)

```bash
cd ../../harness/java && gradle jar
cd -
javac -cp ../../harness/java/build/libs/crossfuzz.jar MyTarget.java
```

### Run (via cross_fuzz coordinator)

The coordinator handles launching. Your TOML entry:

```toml
[[target]]
name = "java_impl"
language = "java"
binary = "java"
args = ["-javaagent:../../harness/java/build/libs/crossfuzz.jar",
        "-cp", "../../harness/java/build/libs/crossfuzz.jar:.", "MyTarget"]
build_cmd = "cd ../../harness/java && gradle jar && cd - && javac -cp ../../harness/java/build/libs/crossfuzz.jar MyTarget.java"
```

The `-javaagent` flag activates the coverage instrumentation. Without it the binary runs but produces no coverage signal.

For Settings, Filter/Compare entry points, and server mode read `<skill-dir>/references/java-harness.md`.
