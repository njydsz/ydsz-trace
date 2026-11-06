"""云顶追踪 HTTP API Python 客户端（V2.4-02）。

设计要点：
  - 仅依赖标准库（urllib），保证内网离线环境可直接安装
  - 统一错误模型：非 200 或业务码非 0 抛出 APIError
  - 统一重试：网络错误 / 429 / 5xx 指数退避重试
  - 鉴权：优先 YDSZ_TOKEN 环境变量，其次构造参数

快速上手::

    from ydsz_trace import YdszClient

    client = YdszClient("http://localhost:2021", token="your-token")
    resp = client.search_logs(item=1, date="20261106", key="timeout", level="ERROR")
    for line in resp.get("lines", []):
        print(line)
"""

from .client import APIError, YdszClient

__all__ = ["APIError", "YdszClient"]
__version__ = "2.4.0"
