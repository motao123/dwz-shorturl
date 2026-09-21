package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// captureStdout 在 fn 执行期间把 os.Stdout 换成管道。initLogger 直接把 core 绑到
// os.Stdout，所以只能这样验它到底编出了什么。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	out := <-done
	_ = r.Close()
	return out
}

// #59：容器部署的日志必须能被 Loki/ELK 结构化解析。此前 JSON 编码器只在设置
// log.file 时才生效，stdout 永远是 console 格式——即推荐部署路径下日志不可解析。
func TestInitLoggerFormat(t *testing.T) {
	t.Run("json 格式产出可解析的一行一条", func(t *testing.T) {
		out := captureStdout(t, func() {
			log := initLogger("info", "", "json")
			log.Info("quota reached", zap.Uint64("member_id", 7))
			_ = log.Sync()
		})
		line := strings.TrimSpace(out)
		if line == "" {
			t.Fatal("没有输出")
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Fatalf("stdout 不是 JSON（容器日志无法解析）: %v\n%s", err, line)
		}
		if parsed["msg"] != "quota reached" {
			t.Errorf("msg 字段丢失: %v", parsed["msg"])
		}
		if parsed["level"] != "info" {
			t.Errorf("level 字段异常: %v", parsed["level"])
		}
	})

	t.Run("默认与未知取值回落 console，不会静默无输出", func(t *testing.T) {
		for _, format := range []string{"", "console", "YAML-谁写错的值"} {
			out := captureStdout(t, func() {
				log := initLogger("info", "", format)
				log.Info("hello")
				_ = log.Sync()
			})
			if !strings.Contains(out, "hello") {
				t.Errorf("format=%q 时丢了输出: %q", format, out)
			}
			trimmed := strings.TrimSpace(out)
			var probe map[string]any
			if json.Unmarshal([]byte(trimmed), &probe) == nil {
				t.Errorf("format=%q 不该输出 JSON: %s", format, trimmed)
			}
		}
	})
}
