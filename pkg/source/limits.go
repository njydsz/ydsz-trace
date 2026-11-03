package source

// MaxLineSize 单条日志行最大字节数。
//
// journald 的 MESSAGE 与 syslog 报文都可能很长，统一 1MB 上限：
// 超过此长度的行会被扫描器截断（而非报错），避免单条异常日志阻塞整个流。
const MaxLineSize = 1024 * 1024
