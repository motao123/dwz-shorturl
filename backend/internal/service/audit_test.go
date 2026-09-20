package service

import (
	"errors"
	"testing"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

type stubAuditRepo struct {
	created []*model.AuditLog
	err     error
}

func (s *stubAuditRepo) Create(log *model.AuditLog) error {
	s.created = append(s.created, log)
	return s.err
}
func (s *stubAuditRepo) List(int, int, repository.AuditFilters) ([]model.AuditLog, int64, error) {
	return nil, 0, nil
}
func (s *stubAuditRepo) FindByID(uint64) (*model.AuditLog, error) { return nil, nil }

// #24：detail 列是 json 类型，而历史上 20 个调用点都传 ""。把 "" 写进 json 列
// 会被数据库判为非法值，整条审计随之丢失——删除短链、改域名这类动作因此查不到
// 「谁做的」。空快照必须是 NULL，而不是空字符串。
func TestAuditDetailEmptyBecomesNull(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if got := auditDetail(in); got != nil {
			t.Fatalf("空快照 %q 应存 NULL，得到 %s", in, string(*got))
		}
	}

	repo := &stubAuditRepo{}
	svc := NewAuditService(repo)
	if err := svc.Log(nil, "short_url_delete", "short_url", "42", "", "127.0.0.1", "test"); err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 row, got %d", len(repo.created))
	}
	if repo.created[0].Detail != nil {
		t.Fatalf("无快照的审计应写 NULL，得到 %s", string(*repo.created[0].Detail))
	}
}

// 合法 JSON 原样入库（前端按对象渲染 detail）。
func TestAuditDetailKeepsValidJSON(t *testing.T) {
	got := auditDetail(`{"uid":"ab12cd","count":3}`)
	if got == nil || string(*got) != `{"uid":"ab12cd","count":3}` {
		t.Fatalf("合法 JSON 应原样保存, got %v", got)
	}
}

// 非 JSON 的文本不能像过去那样把整条审计连带打死：包成 JSON 字符串保住证据。
func TestAuditDetailWrapsInvalidJSON(t *testing.T) {
	got := auditDetail(`broken {`)
	if got == nil {
		t.Fatal("非 JSON 文本不该被丢弃")
	}
	if string(*got) != `"broken {"` {
		t.Fatalf("应包装成 JSON 字符串, got %s", string(*got))
	}
}

// 写失败必须返回给调用方：handler 侧的 `_ =` 之前把它吞得一干二净，
// 审计丢了没人知道（#24）。
func TestAuditLogSurfacesWriteError(t *testing.T) {
	repo := &stubAuditRepo{err: errors.New("db down")}
	err := NewAuditService(repo).Log(nil, "user_password_reset", "user", "1", "", "127.0.0.1", "test")
	if err == nil {
		t.Fatal("审计写入失败必须向上传递，不能再被静默丢弃")
	}
}
