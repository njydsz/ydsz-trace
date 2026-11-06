package com.njydsz.trace;

import java.io.IOException;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * 云顶追踪 HTTP API 客户端（V2.4-02）。
 *
 * <p>设计要点：
 * <ul>
 *   <li>零外部依赖：仅 JDK 自带 {@code java.net.http} 与内置 Json 解析器，
 *       保证内网离线环境可直接构建</li>
 *   <li>统一错误模型：非 200 或业务码非 0 抛出 {@link APIException}</li>
 *   <li>统一重试：网络错误 / 429 / 5xx 指数退避重试</li>
 *   <li>鉴权：优先 YDSZ_TOKEN 环境变量，其次构造参数</li>
 * </ul>
 *
 * <p>请求参数以 {@code Map<String,Object>} 传入，响应以
 * {@code Map<String,Object>} 返回，字段名与 REST API 的 JSON 输出一致。
 *
 * <pre>{@code
 * YdszClient client = YdszClient.builder()
 *         .baseUrl("http://localhost:2021")
 *         .token("your-token")
 *         .build();
 *
 * Map<String, Object> req = new LinkedHashMap<>();
 * req.put("item", 1L);
 * req.put("date", "20261106");
 * req.put("key", "timeout");
 * req.put("level", "ERROR");
 *
 * Map<String, Object> resp = client.searchLogs(req);
 * List<Object> lines = (List<Object>) resp.get("lines");
 * }</pre>
 */
public class YdszClient {

    /** 默认请求超时。 */
    public static final Duration DEFAULT_TIMEOUT = Duration.ofSeconds(30);

    /** 默认重试次数。 */
    public static final int DEFAULT_MAX_RETRIES = 2;

    /** 默认重试退避基准。 */
    public static final Duration DEFAULT_RETRY_BASE_DELAY = Duration.ofMillis(300);

    private final String baseUrl;
    private final String token;
    private final HttpClient httpClient;
    private final int maxRetries;
    private final Duration retryBaseDelay;

    private YdszClient(Builder builder) {
        this.baseUrl = stripTrailingSlash(builder.baseUrl);
        this.token = builder.token != null ? builder.token : System.getenv("YDSZ_TOKEN");
        this.maxRetries = builder.maxRetries;
        this.retryBaseDelay = builder.retryBaseDelay;
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(builder.connectTimeout)
                .build();
    }

    /**
     * 创建 Builder。
     *
     * @return 新的 Builder 实例
     */
    public static Builder builder() {
        return new Builder();
    }

    /**
     * 便捷构造。
     *
     * @param baseUrl 服务地址
     * @param token   鉴权令牌（null 时回退环境变量）
     * @return 客户端实例
     */
    public static YdszClient create(String baseUrl, String token) {
        return builder().baseUrl(baseUrl).token(token).build();
    }

    public String getBaseUrl() {
        return baseUrl;
    }

    public String getToken() {
        return token;
    }

    // ─── API 方法 ───

    /**
     * 按条件检索日志。
     *
     * @param params 参数（item/date 必填，key/query/level/startTime/endTime 可选）
     * @return 响应 data 部分（含 lines / total / pageNo / pageSize）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> searchLogs(Map<String, Object> params) {
        return call("POST", "/logs/search", params);
    }

    /**
     * 上下文回溯。
     *
     * @param itemId 日志项 ID
     * @param params refTime / refLine / before / after / date
     * @return 响应 data 部分（含 before / match / after）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> getContext(long itemId, Map<String, Object> params) {
        return call("GET", "/logs/context/" + itemId, params);
    }

    /**
     * 链路查询。
     *
     * @param params item / date / traceId（client 可选）
     * @return 响应 data 部分（含 traceId / lines）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> getTrace(Map<String, Object> params) {
        return call("POST", "/logs/trace", params);
    }

    /**
     * 自然语言解析。
     *
     * @param query   自然语言检索意图
     * @param convId  会话 ID（多轮指代消解用，可为 null）
     * @return 响应 data 部分（含 level / key / strategy / confidence 等）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> nl2DslParse(String query, String convId) {
        Map<String, Object> params = new LinkedHashMap<String, Object>();
        params.put("query", query);
        if (convId != null && !convId.isEmpty()) {
            params.put("convId", convId);
        }
        return call("POST", "/logs/nl2dsl/parse", params);
    }

    /**
     * 语义检索。
     *
     * @param query 自然语言检索内容
     * @param type  检索目标类型（template / sample）
     * @param topK  召回条数
     * @return 响应 data 部分（含 results / degraded）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> semanticSearch(String query, String type, int topK) {
        Map<String, Object> params = new LinkedHashMap<String, Object>();
        params.put("query", query);
        params.put("type", type);
        params.put("topK", topK);
        return call("POST", "/api/v2/semantic/search", params);
    }

    /**
     * 模板列表。
     *
     * @param timeRange 时间范围（1h/6h/24h/7d）
     * @param topN      返回条数
     * @return 响应 data 部分（含 templates / total）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> listTemplates(String timeRange, int topN) {
        return call("GET", "/logs/templates/search?timeRange="
                + urlEncode(timeRange) + "&topN=" + topN, null);
    }

    /**
     * 异常事件查询。
     *
     * @param timeRange 时间范围（1h/6h/24h/7d）
     * @param limit     返回条数
     * @return 响应 data 部分（含 points / total）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> getAnomalies(String timeRange, int limit) {
        return call("GET", "/logs/anomalies/timeline?timeRange="
                + urlEncode(timeRange) + "&limit=" + limit, null);
    }

    /**
     * 根因推理。
     *
     * @param query 排障问题描述
     * @param item  日志项 ID（可为 null 表示自动选择）
     * @return 响应 data 部分（含 hypothesis / conclusion / toolCalls / totalTokens）
     * @throws APIException 调用失败时抛出
     */
    public Map<String, Object> reasonIncident(String query, Long item) {
        Map<String, Object> params = new LinkedHashMap<String, Object>();
        params.put("query", query);
        if (item != null) {
            params.put("item", item);
        }
        return call("POST", "/logs/agent/reason", params);
    }

    // ─── 内部实现 ───

    /**
     * 执行请求（含重试），返回 data 部分。
     *
     * @param method HTTP 方法
     * @param path   请求路径
     * @param body   请求体（可为 null）
     * @return 响应 data 部分（Map）
     * @throws APIException 失败时抛出
     */
    private Map<String, Object> call(String method, String path, Map<String, Object> body) {
        Exception last = null;

        for (int attempt = 0; attempt <= maxRetries; attempt++) {
            if (attempt > 0) {
                long delayMs = retryBaseDelay.toMillis() * (1L << (attempt - 1));
                try {
                    Thread.sleep(delayMs);
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    throw new APIException(-1, "请求被中断", null);
                }
            }

            byte[] payload = body == null ? null : Json.write(body).getBytes(java.nio.charset.StandardCharsets.UTF_8);

            HttpRequest.Builder rb = HttpRequest.newBuilder()
                    .uri(URI.create(baseUrl + path))
                    .timeout(DEFAULT_TIMEOUT)
                    .header("Content-Type", "application/json");
            if (token != null && !token.isEmpty()) {
                rb.header("Authorization", "Bearer " + token);
            }

            if ("GET".equals(method)) {
                rb.GET();
            } else {
                rb.POST(payload == null
                        ? HttpRequest.BodyPublishers.noBody()
                        : HttpRequest.BodyPublishers.ofByteArray(payload));
            }

            HttpResponse<byte[]> resp;
            try {
                resp = httpClient.send(rb.build(), HttpResponse.BodyHandlers.ofByteArray());
            } catch (IOException e) {
                last = e;
                continue;
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new APIException(-1, "请求被中断", null);
            }

            int status = resp.statusCode();
            byte[] raw = resp.body();
            String text = new String(raw, java.nio.charset.StandardCharsets.UTF_8);

            // 空响应体按状态码处理（网关常对 4xx/5xx 不返回 body）
            if (text.trim().isEmpty()) {
                if (status == 200) {
                    return new LinkedHashMap<String, Object>();
                }
                APIException err = new APIException(status, "HTTP " + status, null);
                if (!isRetryable(status)) {
                    throw err;
                }
                last = err;
                continue;
            }

            Object parsed;
            try {
                parsed = Json.parse(text);
            } catch (RuntimeException e) {
                if (status != 200) {
                    throw new APIException(status, "HTTP " + status, null);
                }
                last = e;
                continue;
            }

            if (!(parsed instanceof Map)) {
                throw new APIException(-1, "响应格式非法: 顶层非对象", null);
            }

            @SuppressWarnings("unchecked")
            Map<String, Object> root = (Map<String, Object>) parsed;

            int code = Json.getInt(root, "code", 0);
            String message = Json.getString(root, "message", "");
            String traceId = Json.getString(root, "traceId", null);

            if (status != 200) {
                APIException err = new APIException(status, message.isEmpty() ? "HTTP " + status : message, traceId);
                // 4xx 客户端错误不重试（429/408 除外）
                if (!isRetryable(status)) {
                    throw err;
                }
                last = err;
                continue;
            }

            if (code != 0) {
                throw new APIException(code, message, traceId);
            }

            Object data = root.get("data");
            if (data instanceof Map) {
                @SuppressWarnings("unchecked")
                Map<String, Object> out = (Map<String, Object>) data;
                return out;
            }
            return new LinkedHashMap<String, Object>();
        }

        throw new APIException(-1, "重试 " + maxRetries + " 次后仍失败: "
                + (last == null ? "未知原因" : last.getMessage()), null);
    }

    /**
     * 判断 HTTP 状态码是否值得重试。
     *
     * @param status HTTP 状态码
     * @return true 表示可重试
     */
    private static boolean isRetryable(int status) {
        return status == 429 || status == 408 || status >= 500;
    }

    /**
     * URL 编码。
     *
     * @param s 原始字符串
     * @return 编码后字符串（失败返回原值）
     */
    private static String urlEncode(String s) {
        try {
            return URLEncoder.encode(s, "UTF-8");
        } catch (Exception e) {
            return s;
        }
    }

    /**
     * 去除尾部斜杠。
     *
     * @param s 原始地址
     * @return 规整后地址
     */
    private static String stripTrailingSlash(String s) {
        if (s == null) {
            return "";
        }
        String out = s;
        while (out.endsWith("/")) {
            out = out.substring(0, out.length() - 1);
        }
        return out;
    }

    // ─── Builder ───

    /** 客户端构造器。 */
    public static final class Builder {
        private String baseUrl;
        private String token;
        private Duration connectTimeout = Duration.ofSeconds(10);
        private int maxRetries = DEFAULT_MAX_RETRIES;
        private Duration retryBaseDelay = DEFAULT_RETRY_BASE_DELAY;

        private Builder() {
        }

        /** 设置服务地址。 */
        public Builder baseUrl(String baseUrl) {
            this.baseUrl = baseUrl;
            return this;
        }

        /** 设置鉴权令牌。 */
        public Builder token(String token) {
            this.token = token;
            return this;
        }

        /** 设置连接超时。 */
        public Builder connectTimeout(Duration d) {
            this.connectTimeout = d;
            return this;
        }

        /** 设置重试次数。 */
        public Builder maxRetries(int n) {
            this.maxRetries = Math.max(0, n);
            return this;
        }

        /** 设置重试退避基准。 */
        public Builder retryBaseDelay(Duration d) {
            this.retryBaseDelay = d;
            return this;
        }

        /** 构建客户端。 */
        public YdszClient build() {
            if (baseUrl == null || baseUrl.isEmpty()) {
                throw new IllegalStateException("baseUrl 不能为空");
            }
            return new YdszClient(this);
        }
    }

    /**
     * 便捷方法：从响应中取出 List 字段。
     *
     * @param resp 响应 map
     * @param key  字段名
     * @return List（不存在返回空 list）
     */
    public static List<Object> listOf(Map<String, Object> resp, String key) {
        Object v = resp == null ? null : resp.get(key);
        if (v instanceof List) {
            @SuppressWarnings("unchecked")
            List<Object> out = (List<Object>) v;
            return out;
        }
        return new ArrayList<Object>();
    }

    /**
     * 便捷方法：从响应中取出 Map 字段。
     *
     * @param resp 响应 map
     * @param key  字段名
     * @return Map（不存在返回 null）
     */
    public static Map<String, Object> mapOf(Map<String, Object> resp, String key) {
        Object v = resp == null ? null : resp.get(key);
        if (v instanceof Map) {
            @SuppressWarnings("unchecked")
            Map<String, Object> out = (Map<String, Object>) v;
            return out;
        }
        return null;
    }
}
