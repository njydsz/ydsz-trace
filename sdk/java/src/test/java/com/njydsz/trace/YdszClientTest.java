package com.njydsz.trace;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Java SDK 单元测试（V2.4-02）。
 *
 * 使用 JDK 自带 HttpServer，无需外部依赖。
 */
public class YdszClientTest {

    private HttpServer server;
    private int port;
    private final AtomicInteger calls = new AtomicInteger();

    private volatile int status = 200;
    private volatile String responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"lines\":[\"a\",\"b\"],\"total\":2}}";

    public void setUp() throws IOException {
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", new HttpHandler() {
            @Override
            public void handle(HttpExchange exchange) throws IOException {
                calls.incrementAndGet();
                byte[] body = responseBody.getBytes(StandardCharsets.UTF_8);
                exchange.getResponseHeaders().set("Content-Type", "application/json");
                exchange.sendResponseHeaders(status, body.length);
                try (OutputStream os = exchange.getResponseBody()) {
                    os.write(body);
                }
            }
        });
        server.start();
        port = server.getAddress().getPort();
    }

    public void tearDown() {
        if (server != null) {
            server.stop(0);
        }
    }

    private YdszClient client(int maxRetries) {
        return YdszClient.builder()
                .baseUrl("http://127.0.0.1:" + port)
                .token("test-token")
                .maxRetries(maxRetries)
                .retryBaseDelay(java.time.Duration.ofMillis(1))
                .build();
    }

    public void testBaseUrlTrimmed() {
        YdszClient c = YdszClient.create("http://x/", "");
        assertEq("http://x", c.getBaseUrl());
    }

    public void testSearchLogs() {
        Map<String, Object> resp = client(0).searchLogs(new LinkedHashMap<String, Object>());
        assertNotNull0(resp);
        Object lines = resp.get("lines");
        assertTrue0(lines instanceof java.util.List);
        assertEq(2, ((java.util.List<?>) lines).size());
    }

    public void testApiErrorBusinessCode() {
        status = 200;
        responseBody = "{\"code\":403,\"message\":\"无权限\",\"data\":null,\"traceId\":\"t1\"}";
        try {
            client(0).searchLogs(new LinkedHashMap<String, Object>());
            fail0("应抛出 APIException");
        } catch (APIException e) {
            assertEq(403, e.getCode());
            assertEq("t1", e.getTraceId());
        }
    }

    public void testApiErrorHttpStatus() {
        status = 403;
        responseBody = "{\"code\":403,\"message\":\"forbidden\",\"data\":null}";
        try {
            client(0).searchLogs(new LinkedHashMap<String, Object>());
            fail0("应抛出 APIException");
        } catch (APIException e) {
            assertEq(403, e.getCode());
        }
    }

    public void testRetryThenSuccess() {
        calls.set(0);
        status = 200;
        responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"ok\":true}}";
        Map<String, Object> resp = client(2).searchLogs(new LinkedHashMap<String, Object>());
        assertTrue0(resp.containsKey("ok"));
    }

    public void test4xxNotRetried() {
        calls.set(0);
        status = 400;
        responseBody = "";
        try {
            client(3).searchLogs(new LinkedHashMap<String, Object>());
            fail0("应抛出 APIException");
        } catch (APIException e) {
            assertEq(1, calls.get());
        }
    }

    public void testBuilderRequiresBaseUrl() {
        try {
            YdszClient.builder().build();
            throw new AssertionError("应抛出 IllegalStateException");
        } catch (IllegalStateException expected) {
            // 期望行为
        }
    }

    public void testReasonIncident() {
        status = 200;
        responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"hypothesis\":\"连接池耗尽\",\"totalTokens\":120}}";
        Map<String, Object> resp = client(0).reasonIncident("服务超时", null);
        assertEq("连接池耗尽", resp.get("hypothesis"));
        assertEq(120, ((Number) resp.get("totalTokens")).intValue());
    }

    public void testListTemplates() {
        status = 200;
        responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"templates\":[],\"total\":0}}";
        Map<String, Object> resp = client(0).listTemplates("1h", 10);
        assertTrue0(resp.containsKey("templates"));
    }

    public void testGetAnomalies() {
        status = 200;
        responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"points\":[],\"total\":0}}";
        Map<String, Object> resp = client(0).getAnomalies("24h", 20);
        assertTrue0(resp.containsKey("points"));
    }

    public void testNl2DslParse() {
        status = 200;
        responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"level\":\"ERROR\",\"strategy\":\"rule\",\"confidence\":0.8}}";
        Map<String, Object> resp = client(0).nl2DslParse("近3小时 ERROR", null);
        assertEq("ERROR", resp.get("level"));
        assertEq("rule", resp.get("strategy"));
    }

    public void testSemanticSearch() {
        status = 200;
        responseBody = "{\"code\":0,\"message\":\"ok\",\"data\":{\"results\":[],\"degraded\":false}}";
        Map<String, Object> resp = client(0).semanticSearch("timeout", "template", 5);
        assertTrue0(resp.containsKey("results"));
    }

    public void testListHelper() {
        Map<String, Object> resp = new LinkedHashMap<String, Object>();
        java.util.List<Object> list = new java.util.ArrayList<Object>();
        list.add("x");
        resp.put("items", list);
        assertEq(1, YdszClient.listOf(resp, "items").size());
        assertEq(0, YdszClient.listOf(resp, "missing").size());
    }

    public void testMapHelper() {
        Map<String, Object> resp = new LinkedHashMap<String, Object>();
        Map<String, Object> nested = new LinkedHashMap<String, Object>();
        nested.put("k", "v");
        resp.put("obj", nested);
        assertEq("v", YdszClient.mapOf(resp, "obj").get("k"));
        assertNull0(YdszClient.mapOf(resp, "missing"));
    }

    // ─── 轻量断言（避免依赖 JUnit，内网离线可直接运行） ───

    private static void assertEq(Object expected, Object actual) {
        if (expected == null ? actual != null : !expected.equals(actual)) {
            throw new AssertionError("期望 " + expected + " 实际 " + actual);
        }
    }

    private static void assertTrue0(boolean cond) {
        if (!cond) {
            throw new AssertionError("断言失败: 期望为 true");
        }
    }

    private static void assertNotNull0(Object v) {
        if (v == null) {
            throw new AssertionError("断言失败: 期望非 null");
        }
    }

    private static void assertNull0(Object v) {
        if (v != null) {
            throw new AssertionError("断言失败: 期望 null 实际 " + v);
        }
    }

    private static void fail0(String msg) {
        throw new AssertionError("断言失败: " + msg);
    }

    public static void main(String[] args) throws Exception {
        String[] names = {
            "testBaseUrlTrimmed", "testSearchLogs", "testApiErrorBusinessCode",
            "testApiErrorHttpStatus", "testRetryThenSuccess", "test4xxNotRetried",
            "testBuilderRequiresBaseUrl", "testReasonIncident", "testListTemplates",
            "testGetAnomalies", "testNl2DslParse", "testSemanticSearch",
            "testListHelper", "testMapHelper"
        };
        int passed = 0;
        int failed = 0;
        for (String name : names) {
            YdszClientTest t = new YdszClientTest();
            try {
                t.setUp();
                try {
                    YdszClientTest.class.getMethod(name).invoke(t);
                    System.out.println("[PASS] " + name);
                    passed++;
                } finally {
                    t.tearDown();
                }
            } catch (Exception e) {
                Throwable cause = e.getCause() != null ? e.getCause() : e;
                System.out.println("[FAIL] " + name + " -> " + cause);
                cause.printStackTrace(System.out);
                failed++;
            }
        }
        System.out.println("PASSCOUNT=" + passed + " FAILCOUNT=" + failed);
        if (failed > 0) {
            System.exit(1);
        }
    }
}
