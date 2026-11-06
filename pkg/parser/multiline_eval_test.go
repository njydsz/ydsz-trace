// Package parser 子文件：多行合并准确率评测（V2.2-07，FR-104）。
//
// PRD FR-104 验收标准：Java 堆栈、Python traceback 合并准确率 ≥ 99%。
// 本文件构建带标注的评测集，对 MergeMultiLine 跑端到端准确率验证，
// 并暴露当前实现的弱点（漏合并 / 误合并 / 类型误判）。
package parser

import (
	"strings"
	"testing"
)

// evalCase 单条评测样本。
//
// expectedEvents 为期望的逻辑条目数；
// expectedKinds 为每个条目的期望类型（长度须与 expectedEvents 一致）；
// mustContain 为合并后全文必须包含的关键片段（用于验证从属行未被截断）。
type evalCase struct {
	name           string
	lines          []string
	expectedEvents int
	expectedKinds  []string
	mustContain    []string
}

// javaStackEvalCases Java 堆栈评测集。
func javaStackEvalCases() []evalCase {
	return []evalCase{
		{
			name: "java_basic_stack",
			lines: []string{
				`2026-11-03 10:00:00 ERROR [main] com.example.Service - request failed`,
				`    at com.example.Service.handle(Service.java:42)`,
				`    at com.example.Controller.dispatch(Controller.java:17)`,
				`    ... 36 more`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindJavaStack},
			mustContain:    []string{"at com.example.Service.handle", "... 36 more"},
		},
		{
			name: "java_caused_by_chain",
			lines: []string{
				`2026-11-03 10:00:01 ERROR [http-nio-8080-exec-1] o.a.c.c.C.[.[.[/].[dispatcherServlet] - Servlet.service() threw exception`,
				`java.lang.RuntimeException: processing failed`,
				`    at com.example.OrderService.process(OrderService.java:88)`,
				`    at com.example.OrderController.submit(OrderController.java:34)`,
				`Caused by: java.lang.NullPointerException: user is null`,
				`    at com.example.UserService.find(UserService.java:51)`,
				`    ... 2 more`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindJavaStack},
			mustContain:    []string{"NullPointerException", "Caused by"},
		},
		{
			name: "java_suppressed",
			lines: []string{
				`2026-11-03 10:00:02 ERROR [main] com.example.App - batch failed`,
				`    at com.example.App.run(App.java:20)`,
				`Suppressed: java.io.IOException: resource closed`,
				`    at com.example.Resource.close(Resource.java:15)`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindJavaStack},
			mustContain:    []string{"Suppressed"},
		},
		{
			name: "java_two_stacks_separated",
			lines: []string{
				`2026-11-03 10:00:03 ERROR [main] com.example.A - fail A`,
				`    at com.example.A.a(A.java:1)`,
				`2026-11-03 10:00:04 ERROR [main] com.example.B - fail B`,
				`    at com.example.B.b(B.java:2)`,
			},
			expectedEvents: 2,
			expectedKinds:  []string{KindJavaStack, KindJavaStack},
			mustContain:    []string{"fail A", "fail B"},
		},
		{
			name: "java_stack_between_info",
			lines: []string{
				`2026-11-03 10:00:05 INFO [main] com.example.App - start`,
				`2026-11-03 10:00:06 ERROR [main] com.example.App - boom`,
				`    at com.example.App.run(App.java:9)`,
				`2026-11-03 10:00:07 INFO [main] com.example.App - end`,
			},
			expectedEvents: 3,
			expectedKinds:  []string{KindSingle, KindJavaStack, KindSingle},
			mustContain:    []string{"boom", "App.java:9"},
		},
		{
			name: "java_inner_class_frame",
			lines: []string{
				`2026-11-03 10:00:08 ERROR [main] com.example.App - inner`,
				`    at com.example.Outer$Inner.run(Outer.java:42)`,
				`    at com.example.Outer.loop(Outer.java:20)`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindJavaStack},
			mustContain:    []string{"Outer$Inner"},
		},
		{
			name: "java_native_method_frame",
			lines: []string{
				`2026-11-03 10:00:09 ERROR [main] com.example.App - native`,
				`    at java.base/java.lang.Thread.run(Thread.java:833)`,
				`    at com.example.App.main(App.java:5)`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindJavaStack},
			mustContain:    []string{"Thread.java:833"},
		},
		{
			name: "java_stack_no_timestamp_primary",
			lines: []string{
				`ERROR com.example.App - no timestamp`,
				`    at com.example.App.run(App.java:1)`,
				`    at com.example.App.main(App.java:9)`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindJavaStack},
			mustContain:    []string{"App.java:1"},
		},
	}
}

// pythonTracebackEvalCases Python traceback 评测集。
func pythonTracebackEvalCases() []evalCase {
	return []evalCase{
		{
			name: "python_basic_traceback",
			lines: []string{
				`2026-11-03 10:00:05 ERROR app - unhandled exception`,
				`Traceback (most recent call last):`,
				`  File "/app/main.py", line 12, in <module>`,
				`    run()`,
				`  File "/app/runner.py", line 30, in run`,
				`    raise ValueError("bad input")`,
				`ValueError: bad input`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindPythonTraceback},
			mustContain:    []string{"Traceback", "ValueError"},
		},
		{
			name: "python_caret_indicator",
			lines: []string{
				`2026-11-03 10:00:06 ERROR app - syntax`,
				`  File "/app/x.py", line 5`,
				`    x = `,
				`        ^`,
				`SyntaxError: invalid syntax`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindPythonTraceback},
			mustContain:    []string{"SyntaxError"},
		},
		{
			name: "python_chained_exception",
			lines: []string{
				`2026-11-03 10:00:07 ERROR app - chain`,
				`Traceback (most recent call last):`,
				`  File "/app/a.py", line 3, in a`,
				`    b()`,
				`  File "/app/b.py", line 5, in b`,
				`    c()`,
				`  File "/app/c.py", line 7, in c`,
				`    raise KeyError("missing")`,
				`KeyError: 'missing'`,
				`During handling of the above exception, another exception occurred:`,
				`Traceback (most recent call last):`,
				`  File "/app/main.py", line 10, in main`,
				`    a()`,
				`  File "/app/a.py", line 3, in a`,
				`    b()`,
				`ValueError: conversion failed`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindPythonTraceback},
			mustContain:    []string{"KeyError", "ValueError"},
		},
		{
			name: "python_two_tracebacks",
			lines: []string{
				`2026-11-03 10:00:08 ERROR app - first`,
				`Traceback (most recent call last):`,
				`  File "/app/a.py", line 1, in a`,
				`    x`,
				`NameError: x`,
				`2026-11-03 10:00:09 ERROR app - second`,
				`Traceback (most recent call last):`,
				`  File "/app/b.py", line 2, in b`,
				`    y`,
				`NameError: y`,
			},
			expectedEvents: 2,
			expectedKinds:  []string{KindPythonTraceback, KindPythonTraceback},
			mustContain:    []string{"NameError: x", "NameError: y"},
		},
		{
			name: "python_no_leading_error_line",
			lines: []string{
				`2026-11-03 10:00:10 INFO app - request`,
				`Traceback (most recent call last):`,
				`  File "/app/x.py", line 1, in x`,
				`    pass`,
			},
			// Traceback 不以时间戳/级别开头，会被 INFO 主行吞并为一个条目。
			// 这是可接受行为：堆栈属于触发它的日志事件。
			expectedEvents: 1,
			expectedKinds:  []string{KindPythonTraceback},
			mustContain:    []string{"INFO app - request", "Traceback"},
		},
	}
}

// goPanicEvalCases Go panic 评测集。
func goPanicEvalCases() []evalCase {
	return []evalCase{
		{
			name: "go_basic_panic",
			lines: []string{
				`2026-11-03 10:00:10 ERROR server - panic recovered`,
				`goroutine 1 [running]:`,
				`main.main()`,
				`	/app/cmd/main.go:25 +0x1a`,
				`created by runtime.main in goroutine 1`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindGoPanic},
			mustContain:    []string{"goroutine 1", "main.go:25"},
		},
		{
			name: "go_panic_with_callers",
			lines: []string{
				`2026-11-03 10:00:11 ERROR server - nil deref`,
				`goroutine 42 [running]:`,
				`main.(*Server).handle(0xc000123456, 0x0)`,
				`	/app/server.go:100 +0x55`,
				`main.main()`,
				`	/app/cmd/main.go:30 +0x22`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindGoPanic},
			mustContain:    []string{"server.go:100", "main.go:30"},
		},
		{
			name: "go_runtime_error",
			lines: []string{
				`2026-11-03 10:00:12 ERROR server - runtime`,
				`panic: runtime error: invalid memory address`,
				`goroutine 7 [running]:`,
				`runtime.panicmem(0x0)`,
				`	/usr/local/go/src/runtime/panic.go:260 +0x1`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindGoPanic},
			mustContain:    []string{"panicmem"},
		},
		{
			name: "go_two_panics",
			lines: []string{
				`2026-11-03 10:00:13 ERROR server - first`,
				`goroutine 1 [running]:`,
				`main.a()`,
				`	/app/a.go:10 +0x1`,
				`2026-11-03 10:00:14 ERROR server - second`,
				`goroutine 2 [running]:`,
				`main.b()`,
				`	/app/b.go:20 +0x2`,
			},
			expectedEvents: 2,
			expectedKinds:  []string{KindGoPanic, KindGoPanic},
			mustContain:    []string{"a.go:10", "b.go:20"},
		},
	}
}

// mixedEvalCases 混合与边界场景。
func mixedEvalCases() []evalCase {
	return []evalCase{
		{
			name: "mixed_java_then_python",
			lines: []string{
				`2026-11-03 10:00:20 ERROR app - java fail`,
				`    at com.example.A.a(A.java:1)`,
				`2026-11-03 10:00:21 ERROR app - py fail`,
				`Traceback (most recent call last):`,
				`  File "/app/x.py", line 1, in x`,
				`ValueError: x`,
			},
			expectedEvents: 2,
			expectedKinds:  []string{KindJavaStack, KindPythonTraceback},
			mustContain:    []string{"A.java:1", "ValueError"},
		},
		{
			name: "no_merge_plain_text",
			lines: []string{
				`some plain text`,
				`    indented continuation`,
				`    another indented line`,
			},
			expectedEvents: 3,
			expectedKinds:  []string{KindSingle, KindSingle, KindSingle},
			mustContain:    []string{"plain text"},
		},
		{
			name: "empty_lines_between",
			lines: []string{
				`2026-11-03 10:00:22 INFO a`,
				``,
				`2026-11-03 10:00:23 INFO b`,
			},
			expectedEvents: 3,
			expectedKinds:  []string{KindSingle, KindSingle, KindSingle},
			mustContain:    []string{"INFO a", "INFO b"},
		},
		{
			name: "json_log_no_merge",
			lines: []string{
				`{"time":"2026-11-03T10:00:24Z","level":"info","msg":"ok"}`,
				`{"time":"2026-11-03T10:00:25Z","level":"error","msg":"fail"}`,
			},
			expectedEvents: 2,
			expectedKinds:  []string{KindSingle, KindSingle},
			mustContain:    []string{"ok", "fail"},
		},
		{
			name: "bracket_level_primary",
			lines: []string{
				`[2026-11-03 10:00:26] [ERROR] boom`,
				`    at x.y.Z.a(Z.java:1)`,
				`[2026-11-03 10:00:27] [INFO] ok`,
			},
			// 方括号时间戳被识别为主行，后续 at 行触发 Java 栈合并。
			expectedEvents: 2,
			expectedKinds:  []string{KindJavaStack, KindSingle},
			mustContain:    []string{"boom", "ok"},
		},
		{
			name: "single_error_no_continuation",
			lines: []string{
				`2026-11-03 10:00:28 ERROR app - standalone`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindSingle},
			mustContain:    []string{"standalone"},
		},
		{
			name: "log_with_backslash_continuation",
			lines: []string{
				`2026-11-03 10:00:29 INFO app - processing \`,
				`    continued line`,
			},
			expectedEvents: 1,
			expectedKinds:  []string{KindGenericIndent},
			mustContain:    []string{"continued line"},
		},
	}
}

// allEvalCases 汇总全部评测样本。
func allEvalCases() []evalCase {
	var cases []evalCase
	cases = append(cases, javaStackEvalCases()...)
	cases = append(cases, pythonTracebackEvalCases()...)
	cases = append(cases, goPanicEvalCases()...)
	cases = append(cases, mixedEvalCases()...)
	return cases
}

// TestMergeMultiLine_Accuracy 端到端准确率验证（FR-104 ≥ 99%）。
//
// 统计维度：
//   - 条目数准确率：合并后条目数 == 期望条目数
//   - 类型准确率：每个条目的 Kind == 期望 Kind
//   - 内容完整性：mustContain 片段全部出现在合并结果中
//
// 综合准确率 = 三项全对的样本数 / 总样本数。
func TestMergeMultiLine_Accuracy(t *testing.T) {
	cases := allEvalCases()
	if len(cases) < 20 {
		t.Fatalf("评测集样本数过少: %d < 20", len(cases))
	}

	var passCount int
	var failures []string

	for _, tc := range cases {
		entries := MergeMultiLine(tc.lines)

		// 合并结果全文（用于 mustContain 校验）
		var allText strings.Builder
		for _, e := range entries {
			allText.WriteString(e.MergedMessage)
			allText.WriteString("\n")
		}
		mergedText := allText.String()

		// 三项校验
		countOK := len(entries) == tc.expectedEvents
		kindOK := true
		if countOK {
			for i, want := range tc.expectedKinds {
				if entries[i].Kind != want {
					kindOK = false
					failures = append(failures, tc.name+" kind["+itoa(i)+"]="+
						entries[i].Kind+" want "+want)
					break
				}
			}
		} else {
			failures = append(failures, tc.name+" count="+itoa(len(entries))+
				" want "+itoa(tc.expectedEvents))
		}

		contentOK := true
		for _, frag := range tc.mustContain {
			if !strings.Contains(mergedText, frag) {
				contentOK = false
				failures = append(failures, tc.name+" missing fragment: "+frag)
				break
			}
		}

		if countOK && kindOK && contentOK {
			passCount++
		}
	}

	accuracy := float64(passCount) / float64(len(cases))
	t.Logf("评测集: %d 样本, 通过: %d, 准确率: %.2f%%", len(cases), passCount, accuracy*100)
	if len(failures) > 0 {
		for _, f := range failures {
			t.Logf("  失败: %s", f)
		}
	}

	if accuracy < 0.99 {
		t.Errorf("多行合并准确率 %.2f%% < PRD 要求的 99%%", accuracy*100)
	}
}

// TestMergeMultiLine_PerCategoryAccuracy 分类别准确率。
//
// Java / Python / Go 三类各自统计，便于定位短板。
func TestMergeMultiLine_PerCategoryAccuracy(t *testing.T) {
	categories := map[string][]evalCase{
		"java":    javaStackEvalCases(),
		"python":  pythonTracebackEvalCases(),
		"go":      goPanicEvalCases(),
		"mixed":   mixedEvalCases(),
	}

	for name, cases := range categories {
		var pass int
		for _, tc := range cases {
			entries := MergeMultiLine(tc.lines)
			if len(entries) != tc.expectedEvents {
				continue
			}
			kindOK := true
			for i, want := range tc.expectedKinds {
				if entries[i].Kind != want {
					kindOK = false
					break
				}
			}
			if !kindOK {
				continue
			}
			var allText strings.Builder
			for _, e := range entries {
				allText.WriteString(e.MergedMessage)
				allText.WriteString("\n")
			}
			contentOK := true
			for _, frag := range tc.mustContain {
				if !strings.Contains(allText.String(), frag) {
					contentOK = false
					break
				}
			}
			if contentOK {
				pass++
			}
		}
		acc := float64(pass) / float64(len(cases))
		t.Logf("  %s: %d/%d = %.2f%%", name, pass, len(cases), acc*100)
		if acc < 0.99 {
			t.Errorf("  %s 准确率 %.2f%% < 99%%", name, acc*100)
		}
	}
}

// itoa 简单整数转字符串（避免 strconv 引入额外依赖噪音）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
