"""云顶追踪 HTTP API Python 客户端实现（V2.4-02）。

仅依赖标准库，保证内网离线环境可安装。
"""

from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Mapping, Optional

__all__ = ["APIError", "YdszClient"]

# 默认配置
DEFAULT_TIMEOUT = 30
DEFAULT_MAX_RETRIES = 2
DEFAULT_RETRY_BASE_DELAY = 0.3


class APIError(Exception):
    """统一错误模型。

    Attributes:
        code: 业务码（HTTP 状态码或响应体中的 code）
        message: 错误信息
        trace_id: 链路 ID（用于排查）
        status_code: HTTP 状态码
    """

    def __init__(
        self,
        code: int,
        message: str,
        trace_id: Optional[str] = None,
        status_code: Optional[int] = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.trace_id = trace_id
        self.status_code = status_code

    def __str__(self) -> str:
        return f"ydsz: code={self.code} message={self.message!r} traceId={self.trace_id}"

    def __repr__(self) -> str:
        return (
            f"APIError(code={self.code!r}, message={self.message!r}, "
            f"trace_id={self.trace_id!r}, status_code={self.status_code!r})"
        )


class YdszClient:
    """云顶追踪 API 客户端。

    Args:
        base_url: 服务地址
        token: 鉴权令牌（为空时读取 YDSZ_TOKEN 环境变量）
        timeout: 单次请求超时（秒）
        max_retries: 重试次数（0 表示不重试）
        retry_base_delay: 重试退避基准（秒）
    """

    def __init__(
        self,
        base_url: str,
        token: Optional[str] = None,
        timeout: float = DEFAULT_TIMEOUT,
        max_retries: int = DEFAULT_MAX_RETRIES,
        retry_base_delay: float = DEFAULT_RETRY_BASE_DELAY,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._token = token or os.environ.get("YDSZ_TOKEN", "")
        self._timeout = timeout
        self._max_retries = max_retries
        self._retry_base_delay = retry_base_delay

    @property
    def base_url(self) -> str:
        """服务地址。"""
        return self._base_url

    @property
    def token(self) -> str:
        """当前鉴权令牌。"""
        return self._token

    # ─── API 方法 ───

    def search_logs(
        self,
        item: int,
        date: str,
        key: Optional[str] = None,
        query: Optional[str] = None,
        regex: Optional[bool] = None,
        level: Optional[str] = None,
        start_time: Optional[str] = None,
        end_time: Optional[str] = None,
        client: Optional[int] = None,
        page_no: Optional[int] = None,
        page_size: Optional[int] = None,
    ) -> Dict[str, Any]:
        """按条件检索日志。

        Args:
            item: 日志项 ID
            date: 日志日期 YYYYMMDD
            key: 搜索关键词
            query: 布尔查询表达式
            regex: 是否正则匹配
            level: 日志级别过滤
            start_time: 起始时间 HH:MM:SS
            end_time: 结束时间 HH:MM:SS
            client: 客户端 ID
            page_no: 页码
            page_size: 每页条数

        Returns:
            响应 data 部分（lines / total / pageNo / pageSize）
        """
        params: Dict[str, Any] = {"item": item, "date": date}
        _put_if_present(
            params,
            key=key,
            query=query,
            regex=regex,
            level=level,
            startTime=start_time,
            endTime=end_time,
            client=client,
            pageNo=page_no,
            pageSize=page_size,
        )
        return self._call("POST", "/logs/search", params)

    def get_context(
        self,
        item_id: int,
        ref_time: Optional[str] = None,
        ref_line: Optional[str] = None,
        before: Optional[int] = None,
        after: Optional[int] = None,
        date: Optional[str] = None,
    ) -> Dict[str, Any]:
        """上下文回溯。"""
        params: Dict[str, Any] = {}
        _put_if_present(
            params,
            refTime=ref_time,
            refLine=ref_line,
            before=before,
            after=after,
            date=date,
        )
        return self._call("GET", f"/logs/context/{item_id}", params)

    def get_trace(
        self,
        item: int,
        date: str,
        trace_id: str,
        client: Optional[int] = None,
    ) -> Dict[str, Any]:
        """链路查询。"""
        params: Dict[str, Any] = {"item": item, "date": date, "traceId": trace_id}
        _put_if_present(params, client=client)
        return self._call("POST", "/logs/trace", params)

    def nl2dsl_parse(self, query: str, conv_id: Optional[str] = None) -> Dict[str, Any]:
        """自然语言解析。

        Args:
            query: 自然语言检索意图
            conv_id: 会话 ID（多轮指代消解用）
        """
        params: Dict[str, Any] = {"query": query}
        _put_if_present(params, convId=conv_id)
        return self._call("POST", "/logs/nl2dsl/parse", params)

    def semantic_search(
        self, query: str, type_: Optional[str] = None, top_k: Optional[int] = None
    ) -> Dict[str, Any]:
        """语义检索。

        Args:
            query: 自然语言检索内容
            type_: 检索目标类型（template / sample）
            top_k: 召回条数
        """
        params: Dict[str, Any] = {"query": query}
        _put_if_present(params, **{"type": type_, "topK": top_k})
        return self._call("POST", "/api/v2/semantic/search", params)

    def list_templates(self, time_range: str = "1h", top_n: int = 10) -> Dict[str, Any]:
        """模板列表。"""
        qs = urllib.parse.urlencode({"timeRange": time_range, "topN": top_n})
        return self._call("GET", f"/logs/templates/search?{qs}", None)

    def get_anomalies(self, time_range: str = "24h", limit: int = 20) -> Dict[str, Any]:
        """异常事件查询。"""
        qs = urllib.parse.urlencode({"timeRange": time_range, "limit": limit})
        return self._call("GET", f"/logs/anomalies/timeline?{qs}", None)

    def reason_incident(self, query: str, item: Optional[int] = None) -> Dict[str, Any]:
        """根因推理。"""
        params: Dict[str, Any] = {"query": query}
        _put_if_present(params, item=item)
        return self._call("POST", "/logs/agent/reason", params)

    # ─── 内部实现 ───

    def _call(
        self, method: str, path: str, params: Optional[Mapping[str, Any]]
    ) -> Dict[str, Any]:
        """执行请求（含重试），返回 data 部分。

        Args:
            method: HTTP 方法
            path: 请求路径
            params: 请求参数

        Returns:
            响应 data 部分

        Raises:
            APIError: 调用失败时抛出
        """
        last_err: Optional[Exception] = None

        for attempt in range(self._max_retries + 1):
            if attempt > 0:
                delay = self._retry_base_delay * (2 ** (attempt - 1))
                time.sleep(delay)

            try:
                status, raw = self._once(method, path, params)
            except _NetworkError as e:
                # 网络层错误可重试
                last_err = e
                continue

            # 空响应体
            if not raw or not raw.strip():
                if status == 200:
                    return {}
                err = APIError(status, f"HTTP {status}", status_code=status)
                if not _is_retryable(status):
                    raise err
                last_err = err
                continue

            try:
                parsed = json.loads(raw.decode("utf-8"))
            except (ValueError, UnicodeDecodeError):
                if status != 200:
                    raise APIError(status, f"HTTP {status}", status_code=status)
                last_err = ValueError("响应解析失败")
                continue

            if not isinstance(parsed, dict):
                raise APIError(-1, "响应格式非法: 顶层非对象", status_code=status)

            code = _as_int(parsed.get("code"), 0)
            message = parsed.get("message") or ""
            trace_id = parsed.get("traceId")

            if status != 200:
                err = APIError(
                    status, message or f"HTTP {status}", trace_id=trace_id, status_code=status
                )
                if not _is_retryable(status):
                    raise err
                last_err = err
                continue

            if code != 0:
                raise APIError(code, message, trace_id=trace_id, status_code=status)

            data = parsed.get("data")
            return data if isinstance(data, dict) else {}

        raise APIError(
            -1,
            f"重试 {self._max_retries} 次后仍失败: {last_err}",
            status_code=None,
        )

    def _once(self, method: str, path: str, params: Optional[Mapping[str, Any]]):
        """执行单次请求。

        Returns:
            (HTTP 状态码, 响应字节)
        """
        body = None
        if params is not None:
            body = json.dumps(params).encode("utf-8")

        req = urllib.request.Request(
            self._base_url + path,
            data=body,
            method=method,
        )
        req.add_header("Content-Type", "application/json")
        if self._token:
            req.add_header("Authorization", f"Bearer {self._token}")

        try:
            with urllib.request.urlopen(req, timeout=self._timeout) as resp:
                return resp.status, resp.read()
        except urllib.error.HTTPError as e:
            # HTTPError 也是响应：读取其 body 作为错误信息
            try:
                payload = e.read()
            except Exception:  # noqa: BLE001 - 读取失败按空处理
                payload = b""
            return e.code, payload
        except urllib.error.URLError as e:
            # 网络层错误：抛出由调用方决定是否重试
            raise _NetworkError(str(e)) from e


class _NetworkError(Exception):
    """网络层错误（可重试）。"""


def _is_retryable(status: int) -> bool:
    """判断 HTTP 状态码是否值得重试。"""
    return status == 429 or status == 408 or status >= 500


def _put_if_present(target: Dict[str, Any], **kwargs: Any) -> None:
    """把非 None 的参数写入目标 dict。"""
    for key, value in kwargs.items():
        if value is not None:
            target[key] = value


def _as_int(value: Any, default: int) -> int:
    """安全转 int。"""
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value)
    return default
