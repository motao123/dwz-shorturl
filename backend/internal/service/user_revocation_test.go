package service

import (
	"context"
	"errors"
	"testing"

	"dwz-admin/internal/repository"
)

// recRevoker 记录"哪些改动真的触发了会话吊销"。
type recRevoker struct {
	calls []uint64
	err   error
}

func (r *recRevoker) RevokeUser(_ context.Context, userID uint64) error {
	r.calls = append(r.calls, userID)
	return r.err
}

func newRevokingUserService(repo repository.UserRepo) (*userService, *recRevoker) {
	rev := &recRevoker{}
	return NewUserService(repo).WithSessionRevocation(rev, nil), rev
}

// disabledStatus / enabledStatus 对应 model.User.Status。
func ptrStatus(v int8) *int8 { return &v }

func TestRevocationOnAccountChanges(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(s *userService) error
		wantIDS []uint64
	}{
		{
			name:    "禁用账号立即作废在授会话（#42）",
			mutate:  func(s *userService) error { _, err := s.Update(3, "", "", "", ptrStatus(0)); return err },
			wantIDS: []uint64{3},
		},
		{
			name:    "只改昵称邮箱不动凭证效力，不该踢人下线",
			mutate:  func(s *userService) error { _, err := s.Update(3, "a@b.c", "新昵称", "", nil); return err },
			wantIDS: nil,
		},
		{
			name:    "状态没变（重复提交同一个值）不算变更",
			mutate:  func(s *userService) error { _, err := s.Update(3, "", "", "", ptrStatus(1)); return err },
			wantIDS: nil,
		},
		{
			name:    "改角色要作废：角色写在 claims 里",
			mutate:  func(s *userService) error { return s.AssignRoles(4, []uint64{2}) },
			wantIDS: []uint64{4},
		},
		{
			name:    "重置密码要作废：旧会话不该继续有效",
			mutate:  func(s *userService) error { return s.ResetPassword(5, "Rotated-pass-2026") },
			wantIDS: []uint64{5},
		},
		{
			name:    "删除账号要作废",
			mutate:  func(s *userService) error { return s.Delete(6) },
			wantIDS: []uint64{6},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, rev := newRevokingUserService(&guardRepo{})
			if err := tc.mutate(svc); err != nil {
				t.Fatalf("改动本身失败: %v", err)
			}
			if len(rev.calls) != len(tc.wantIDS) {
				t.Fatalf("吊销调用 = %v，期望 %v", rev.calls, tc.wantIDS)
			}
			for i := range tc.wantIDS {
				if rev.calls[i] != tc.wantIDS[i] {
					t.Errorf("吊销第 %d 次 = 用户 %d，期望 %d", i+1, rev.calls[i], tc.wantIDS[i])
				}
			}
		})
	}
}

// TestRevocationFailureKeepsAccountChange 记录一个刻意的取舍：DB 变更已经落库，
// 吊销失败不回滚它——禁用账号本身才是要紧的一步。但失败必须被看见（日志 Error），
// 所以这里锁住"不返回错误"这个行为，避免日后有人把它改成半回滚的第三种状态。
func TestRevocationFailureKeepsAccountChange(t *testing.T) {
	rev := &recRevoker{err: errors.New("redis down")}
	svc := NewUserService(&guardRepo{}).WithSessionRevocation(rev, nil)

	user, err := svc.Update(3, "", "", "", ptrStatus(0))
	if err != nil {
		t.Fatalf("吊销失败不应让禁用操作失败，得到 %v", err)
	}
	if user.Status != 0 {
		t.Errorf("账号状态未落库: %d", user.Status)
	}
	if len(rev.calls) != 1 {
		t.Errorf("仍应尝试过一次吊销，实际 %d 次", len(rev.calls))
	}
}

// TestNoRevokerKeepsLegacyBehaviour 保证未接线（无 Redis 或测试构造）时行为与改动前一致。
func TestNoRevokerKeepsLegacyBehaviour(t *testing.T) {
	svc := NewUserService(&guardRepo{}) // 没有 WithSessionRevocation
	if _, err := svc.Update(3, "", "", "", ptrStatus(0)); err != nil {
		t.Fatalf("未接吊销时的禁用应照常成功: %v", err)
	}
}
