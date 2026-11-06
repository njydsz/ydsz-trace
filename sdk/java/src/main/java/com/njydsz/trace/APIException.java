package com.njydsz.trace;

/**
 * API 调用异常（统一错误模型）。
 */
public class APIException extends RuntimeException {

    private static final long serialVersionUID = 1L;

    /** 业务码（HTTP 状态码或响应体中的 code）。 */
    private final int code;

    /** 链路 ID（用于排查）。 */
    private final String traceId;

    /**
     * 构造异常。
     *
     * @param code    错误码
     * @param message 错误信息
     * @param traceId 链路 ID（可为 null）
     */
    public APIException(int code, String message, String traceId) {
        super(message);
        this.code = code;
        this.traceId = traceId;
    }

    public int getCode() {
        return code;
    }

    public String getTraceId() {
        return traceId;
    }

    @Override
    public String toString() {
        return "APIException{code=" + code + ", traceId='" + traceId + "', message='" + getMessage() + "'}";
    }
}
