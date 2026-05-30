# Examples

Each subdirectory is a self-contained crossfuzz campaign. Inside, `crossfuzz.toml` describes the targets, comparator, and corpus layout.

| Example                        | What it demonstrates                                                                                              |
|--------------------------------|-------------------------------------------------------------------------------------------------------------------|
| [`base64/`](./base64/)         | Differential fuzzing of base64 encoders in C, C++, Go, JS, TS, Python, and Rust. Comparator: `byte_equal`. Good starting point — exercises every language harness. |
| [`json_parse/`](./json_parse/) | JSON parser divergence across C, Go, Java, and JS. Comparator: `json_structural` (ignores key order and whitespace). |
| [`url_parse/`](./url_parse/)   | URL parser divergence between Go and Java. Demonstrates the `input_filter` feature (see `filter/`).                |
| [`server_fuzz/`](./server_fuzz/) | Fuzzing long-running HTTP servers (Go ingress + Go API + Java reverse proxy). Demonstrates `type = "server"` targets. |

To run any example:

```bash
cd examples/<name>
crossfuzz build
crossfuzz run
```
