import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.concurrent.Executors;

public class JavaReverseProxy {

    // Upstream server (change this to your backend)
    static final String TARGET_BASE = "http://localhost:9000";

    static final HttpClient client = HttpClient.newHttpClient();

    public static void main(String[] args) throws Exception {
        crossfuzz.Harness.initServer();

        int port = 8080;

        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/", new ProxyHandler());
        server.setExecutor(Executors.newFixedThreadPool(10));

        server.start();

        System.out.println("🚀 Reverse proxy running");
        System.out.println("👉 Listening on: http://localhost:" + port);
        System.out.println("👉 Forwarding to: " + TARGET_BASE);
    }

    static class ProxyHandler implements HttpHandler {
        @Override
        public void handle(HttpExchange exchange) throws IOException {
            try {
                crossfuzz.CoverageRuntime.clear();
                String method = exchange.getRequestMethod();
                String path = exchange.getRequestURI().toString();

                URI targetUri = URI.create(TARGET_BASE + path);

                byte[] body = exchange.getRequestBody().readAllBytes();

                HttpRequest.Builder builder = HttpRequest.newBuilder()
                        .uri(targetUri)
                        .method(method,
                                body.length > 0
                                        ? HttpRequest.BodyPublishers.ofByteArray(body)
                                        : HttpRequest.BodyPublishers.noBody());
                
                exchange.getRequestHeaders().forEach((k, v) -> {
                    if (!k.equalsIgnoreCase("Host") && !k.equalsIgnoreCase("Connection")) {
                        builder.header(k, String.join(",", v));
                    }
                });
                crossfuzz.CoverageRuntime.collect();
                HttpResponse<byte[]> response = client.send(
                        builder.build(),
                        HttpResponse.BodyHandlers.ofByteArray()
                );

                response.headers().map().forEach((k, v) ->
                        exchange.getResponseHeaders().put(k, v)
                );

                byte[] resp = response.body();

                exchange.sendResponseHeaders(response.statusCode(), resp.length);

                try (OutputStream os = exchange.getResponseBody()) {
                    os.write(resp);
                }

            } catch (Exception e) {
                String msg = "Proxy error: " + e.getMessage();
                exchange.sendResponseHeaders(500, msg.length());
                try (OutputStream os = exchange.getResponseBody()) {
                    os.write(msg.getBytes());
                }
            }
        }
    }
}