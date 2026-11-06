package com.njydsz.trace;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * 极简 JSON 解析器（V2.4-02）。
 *
 * <p>内网离线环境下无法依赖 Jackson，因此实现一个只覆盖 SDK 所需的最小解析器：
 * <ul>
 *   <li>支持 object / array / string / number / boolean / null</li>
 *   <li>结果统一映射为 {@code Map<String,Object>} 与 {@code List<Object>}</li>
 *   <li>不保证完整 JSON 规范（例如不支持 Unicode 转义的完整还原）</li>
 * </ul>
 *
 * <p>适用范围仅限 SDK 内部；业务方不应直接使用本类。
 */
final class Json {

    private final String src;
    private int pos;

    private Json(String src) {
        this.src = src;
    }

    /**
     * 解析 JSON 文本。
     *
     * @param text JSON 字符串
     * @return 解析结果（Map / List / String / Number / Boolean / null）
     * @throws IllegalArgumentException 格式非法时抛出
     */
    static Object parse(String text) {
        if (text == null) {
            return null;
        }
        Json p = new Json(text);
        p.skipWs();
        Object v = p.value();
        p.skipWs();
        if (p.pos < p.src.length()) {
            throw new IllegalArgumentException("JSON 解析存在未消费内容: " + p.src.substring(p.pos));
        }
        return v;
    }

    /**
     * 序列化为 JSON 文本（仅支持 SDK 请求 DTO 用到的类型）。
     *
     * @param v 待序列化对象
     * @return JSON 字符串
     */
    static String write(Object v) {
        StringBuilder sb = new StringBuilder();
        writeValue(sb, v);
        return sb.toString();
    }

    private static void writeValue(StringBuilder sb, Object v) {
        if (v == null) {
            sb.append("null");
            return;
        }
        if (v instanceof String) {
            writeString(sb, (String) v);
            return;
        }
        if (v instanceof Number || v instanceof Boolean) {
            sb.append(v.toString());
            return;
        }
        if (v instanceof Map) {
            sb.append('{');
            boolean first = true;
            for (Map.Entry<?, ?> e : ((Map<?, ?>) v).entrySet()) {
                if (!first) {
                    sb.append(',');
                }
                first = false;
                writeString(sb, String.valueOf(e.getKey()));
                sb.append(':');
                writeValue(sb, e.getValue());
            }
            sb.append('}');
            return;
        }
        if (v instanceof Iterable) {
            sb.append('[');
            boolean first = true;
            for (Object item : (Iterable<?>) v) {
                if (!first) {
                    sb.append(',');
                }
                first = false;
                writeValue(sb, item);
            }
            sb.append(']');
            return;
        }
        writeString(sb, v.toString());
    }

    private static void writeString(StringBuilder sb, String s) {
        sb.append('"');
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"':
                    sb.append("\\\"");
                    break;
                case '\\':
                    sb.append("\\\\");
                    break;
                case '\n':
                    sb.append("\\n");
                    break;
                case '\r':
                    sb.append("\\r");
                    break;
                case '\t':
                    sb.append("\\t");
                    break;
                default:
                    if (c < 0x20) {
                        sb.append(String.format("\\u%04x", (int) c));
                    } else {
                        sb.append(c);
                    }
            }
        }
        sb.append('"');
    }

    // ─── 解析实现 ───

    private Object value() {
        if (pos >= src.length()) {
            throw new IllegalArgumentException("JSON 意外结束");
        }
        char c = src.charAt(pos);
        switch (c) {
            case '{':
                return object();
            case '[':
                return array();
            case '"':
                return string();
            case 't':
                expect("true");
                return Boolean.TRUE;
            case 'f':
                expect("false");
                return Boolean.FALSE;
            case 'n':
                expect("null");
                return null;
            default:
                return number();
        }
    }

    private Map<String, Object> object() {
        expectChar('{');
        Map<String, Object> map = new LinkedHashMap<String, Object>();
        skipWs();
        if (peek() == '}') {
            pos++;
            return map;
        }
        while (true) {
            skipWs();
            String key = string();
            skipWs();
            expectChar(':');
            skipWs();
            map.put(key, value());
            skipWs();
            char c = next();
            if (c == '}') {
                return map;
            }
            if (c != ',') {
                throw new IllegalArgumentException("期望 ',' 或 '}' 在位置 " + pos);
            }
        }
    }

    private List<Object> array() {
        expectChar('[');
        List<Object> list = new ArrayList<Object>();
        skipWs();
        if (peek() == ']') {
            pos++;
            return list;
        }
        while (true) {
            skipWs();
            list.add(value());
            skipWs();
            char c = next();
            if (c == ']') {
                return list;
            }
            if (c != ',') {
                throw new IllegalArgumentException("期望 ',' 或 ']' 在位置 " + pos);
            }
        }
    }

    private String string() {
        expectChar('"');
        StringBuilder sb = new StringBuilder();
        while (pos < src.length()) {
            char c = src.charAt(pos++);
            if (c == '"') {
                return sb.toString();
            }
            if (c == '\\') {
                char esc = src.charAt(pos++);
                switch (esc) {
                    case '"':
                        sb.append('"');
                        break;
                    case '\\':
                        sb.append('\\');
                        break;
                    case '/':
                        sb.append('/');
                        break;
                    case 'b':
                        sb.append('\b');
                        break;
                    case 'f':
                        sb.append('\f');
                        break;
                    case 'n':
                        sb.append('\n');
                        break;
                    case 'r':
                        sb.append('\r');
                        break;
                    case 't':
                        sb.append('\t');
                        break;
                    case 'u':
                        String hex = src.substring(pos, pos + 4);
                        pos += 4;
                        sb.append((char) Integer.parseInt(hex, 16));
                        break;
                    default:
                        throw new IllegalArgumentException("未知转义: \\" + esc);
                }
                continue;
            }
            sb.append(c);
        }
        throw new IllegalArgumentException("字符串未闭合");
    }

    private Object number() {
        int start = pos;
        while (pos < src.length()) {
            char c = src.charAt(pos);
            if ((c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E') {
                pos++;
            } else {
                break;
            }
        }
        String num = src.substring(start, pos);
        if (num.isEmpty()) {
            throw new IllegalArgumentException("期望数字在位置 " + start);
        }
        if (num.indexOf('.') >= 0 || num.indexOf('e') >= 0 || num.indexOf('E') >= 0) {
            return Double.parseDouble(num);
        }
        try {
            return Long.parseLong(num);
        } catch (NumberFormatException e) {
            return Double.parseDouble(num);
        }
    }

    private void expect(String literal) {
        if (!src.startsWith(literal, pos)) {
            throw new IllegalArgumentException("期望 " + literal + " 在位置 " + pos);
        }
        pos += literal.length();
    }

    private void expectChar(char c) {
        char actual = next();
        if (actual != c) {
            throw new IllegalArgumentException("期望 '" + c + "' 实际 '" + actual + "' 在位置 " + (pos - 1));
        }
    }

    private char next() {
        if (pos >= src.length()) {
            throw new IllegalArgumentException("JSON 意外结束");
        }
        return src.charAt(pos++);
    }

    private char peek() {
        return pos < src.length() ? src.charAt(pos) : '\0';
    }

    private void skipWs() {
        while (pos < src.length()) {
            char c = src.charAt(pos);
            if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                pos++;
            } else {
                return;
            }
        }
    }

    // ─── 便捷访问 ───

    /**
     * 安全取出 Map 字段。
     *
     * @param map 源 map
     * @param key 键
     * @return 值（不存在返回 null）
     */
    static Object get(Map<String, Object> map, String key) {
        return map == null ? null : map.get(key);
    }

    /**
     * 取出字符串字段。
     *
     * @param map 源 map
     * @param key 键
     * @param def 默认值
     * @return 字符串值
     */
    static String getString(Map<String, Object> map, String key, String def) {
        Object v = get(map, key);
        return v == null ? def : v.toString();
    }

    /**
     * 取出整型字段。
     *
     * @param map 源 map
     * @param key 键
     * @param def 默认值
     * @return 整型值
     */
    static int getInt(Map<String, Object> map, String key, int def) {
        Object v = get(map, key);
        if (v instanceof Number) {
            return ((Number) v).intValue();
        }
        return def;
    }

    /**
     * 取出长整型字段。
     *
     * @param map 源 map
     * @param key 键
     * @param def 默认值
     * @return 长整型值
     */
    static long getLong(Map<String, Object> map, String key, long def) {
        Object v = get(map, key);
        if (v instanceof Number) {
            return ((Number) v).longValue();
        }
        return def;
    }

    /**
     * 取出布尔字段。
     *
     * @param map 源 map
     * @param key 键
     * @param def 默认值
     * @return 布尔值
     */
    static boolean getBool(Map<String, Object> map, String key, boolean def) {
        Object v = get(map, key);
        if (v instanceof Boolean) {
            return (Boolean) v;
        }
        return def;
    }

    /**
     * 取出 Map 类型字段。
     *
     * @param map 源 map
     * @param key 键
     * @return 子 map（不存在返回 null）
     */
    @SuppressWarnings("unchecked")
    static Map<String, Object> getMap(Map<String, Object> map, String key) {
        Object v = get(map, key);
        return v instanceof Map ? (Map<String, Object>) v : null;
    }

    /**
     * 取出 List 类型字段。
     *
     * @param map 源 map
     * @param key 键
     * @return 子 list（不存在返回 null）
     */
    @SuppressWarnings("unchecked")
    static List<Object> getList(Map<String, Object> map, String key) {
        Object v = get(map, key);
        return v instanceof List ? (List<Object>) v : null;
    }
}
