package perf

import (
	"testing"
	"time"
)

// TestSampler_PassThroughWhenRateIsOne 采样率 1.0 时全部保留。
func TestSampler_PassThroughWhenRateIsOne(t *testing.T) {
	s := NewSampler()
	s.SetRate(1.0)

	for i := 0; i < 1000; i++ {
		if !s.ShouldKeep("INFO") {
			t.Fatal("采样率 1.0 不应丢弃任何日志")
		}
	}
}

// TestSampler_ErrorAlwaysKept ERROR 永不被采样丢弃。
func TestSampler_ErrorAlwaysKept(t *testing.T) {
	s := NewSampler()
	s.SetRate(0.01) // 1% 采样率

	for i := 0; i < 1000; i++ {
		if !s.ShouldKeep("ERROR") {
			t.Fatal("ERROR 不应被采样丢弃")
		}
		if !s.ShouldKeep("FATAL") {
			t.Fatal("FATAL 不应被采样丢弃")
		}
	}
}

// TestSampler_DropsInfoAtLowRate 低采样率下 INFO 会被大量丢弃。
func TestSampler_DropsInfoAtLowRate(t *testing.T) {
	s := NewSampler()
	s.SetRate(0.1)

	kept := 0
	for i := 0; i < 10000; i++ {
		if s.ShouldKeep("INFO") {
			kept++
		}
	}

	// 10% 采样率下预期保留约 1000 条，允许较大偏差
	if kept < 500 || kept > 1500 {
		t.Errorf("保留数 = %d，预期约 1000", kept)
	}
}

// TestSampler_UnknownLevelKept 未知级别保守保留。
func TestSampler_UnknownLevelKept(t *testing.T) {
	s := NewSampler()
	s.SetRate(0.01)

	for i := 0; i < 100; i++ {
		if !s.ShouldKeep("SOMETHING_ELSE") {
			t.Fatal("未知级别应保守保留")
		}
	}
}

// TestSampler_EmptyLevelKept 空级别保守保留。
func TestSampler_EmptyLevelKept(t *testing.T) {
	s := NewSampler()
	s.SetRate(0.01)
	if !s.ShouldKeep("") {
		t.Fatal("空级别应保守保留")
	}
}

// TestSampler_SetRateClamped 采样率被限制在合法区间。
func TestSampler_SetRateClamped(t *testing.T) {
	s := NewSampler()

	s.SetRate(2.0)
	if s.Rate() != 1.0 {
		t.Errorf("上限应为 1.0, got %.2f", s.Rate())
	}

	s.SetRate(-1.0)
	if s.Rate() < 0.01 {
		t.Errorf("下限应为 0.01, got %.4f", s.Rate())
	}
}

// TestSampler_LevelRank 级别优先级排序。
func TestSampler_LevelRank(t *testing.T) {
	// 数值越大越严重
	if !(levelRank("TRACE") < levelRank("DEBUG") &&
		levelRank("DEBUG") < levelRank("INFO") &&
		levelRank("INFO") < levelRank("WARN") &&
		levelRank("WARN") < levelRank("ERROR") &&
		levelRank("ERROR") < levelRank("FATAL")) {
		t.Fatal("级别优先级排序错误")
	}
}

// TestNewSampler_FromEnv 环境变量覆盖采样率。
func TestNewSampler_FromEnv(t *testing.T) {
	t.Setenv("YDSZ_SAMPLE_RATE", "0.5")
	s := NewSampler()
	if s.Rate() != 0.5 {
		t.Errorf("rate = %.2f want 0.5", s.Rate())
	}
}

// TestNewSampler_InvalidEnvFallback 非法环境变量回退默认值。
func TestNewSampler_InvalidEnvFallback(t *testing.T) {
	t.Setenv("YDSZ_SAMPLE_RATE", "not-a-number")
	s := NewSampler()
	if s.Rate() != DefaultSampleRate {
		t.Errorf("rate = %.2f want %.2f", s.Rate(), DefaultSampleRate)
	}
}

// TestNewSampler_ZeroEnvFallback 零值回退默认值。
func TestNewSampler_ZeroEnvFallback(t *testing.T) {
	t.Setenv("YDSZ_SAMPLE_RATE", "0")
	s := NewSampler()
	if s.Rate() != DefaultSampleRate {
		t.Errorf("rate = %.2f want %.2f", s.Rate(), DefaultSampleRate)
	}
}

// TestGuardian_StartStop 护栏可正常启停。
func TestGuardian_StartStop(t *testing.T) {
	s := NewSampler()
	g := NewGuardian(s)

	g.Start()
	time.Sleep(100 * time.Millisecond)
	g.Stop()
	// 能走到这里说明启停无死锁
}

// TestGuardian_Stats 统计字段完整。
func TestGuardian_Stats(t *testing.T) {
	s := NewSampler()
	g := NewGuardian(s)

	st := g.Stats()
	if st.SampleRate <= 0 {
		t.Errorf("sampleRate = %f", st.SampleRate)
	}
	if st.MaxMemMB <= 0 {
		t.Errorf("maxMemMb = %f", st.MaxMemMB)
	}
	if st.CheckIntervalSec <= 0 {
		t.Errorf("checkIntervalSec = %d", st.CheckIntervalSec)
	}
}

// TestGuardian_FromEnv 环境变量覆盖阈值。
func TestGuardian_FromEnv(t *testing.T) {
	t.Setenv("YDSZ_MAX_MEM_MB", "128")
	t.Setenv("YDSZ_MAX_CPU_PERCENT", "40")

	g := NewGuardian(NewSampler())
	if g.maxMemMB != 128 {
		t.Errorf("maxMemMB = %f want 128", g.maxMemMB)
	}
	if g.maxCPUPercent != 40 {
		t.Errorf("maxCPUPercent = %f want 40", g.maxCPUPercent)
	}
}

// TestCurrentMemMB_NonNegative 内存读数非负。
func TestCurrentMemMB_NonNegative(t *testing.T) {
	if got := currentMemMB(); got < 0 {
		t.Errorf("currentMemMB = %f want >= 0", got)
	}
}
