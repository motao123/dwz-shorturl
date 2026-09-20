package repository

import (
	"gorm.io/gorm"
)

// MemberPurgeRepo performs the data-side of "delete this member" (#39).
//
// The problem it solves: a member's footprint spans two databases — the member
// row and the mirrored public `wjoy_log` short links live in the public DB,
// while the authoritative `short_urls` rows and the per-click `click_logs`
// (which carry the visitor IP) live in the admin DB. `memberService.Delete` only
// removed the member row, so after "删号":
//
//   - every short link the member ever created stayed redirectable forever, with
//     nobody able to revoke it (the member is gone, the rows still point at the
//     deleted id);
//   - the visitor IPs collected on those links stayed in click_logs, so a data
//     deletion request could not actually be honoured.
//
// The purge is deliberately one transaction per database (they may be separate
// servers, so a single cross-DB transaction is not available) and is written to
// be idempotent: running it twice is safe.
type MemberPurgeRepo interface {
	// MemberLinks returns the ids and uids of every short link owned by the
	// member, including soft-deleted ones (their mirror row may still be live).
	MemberLinks(memberID uint64) ([]MemberLinkRef, error)
	// PurgePublicLinks disables the member's mirrored wjoy_log rows so the PHP
	// redirect path stops serving them (status=0 -> 410). It returns how many
	// rows were affected.
	PurgePublicLinks(uids []string) (int64, error)
	// PurgeAdminLinks neutralises the member's short_urls rows: disable them and
	// clear member_id so a re-used id can never inherit ownership. It returns
	// the affected row count.
	PurgeAdminLinks(memberID uint64) (int64, error)
	// AnonymizeClickLogs blanks the visitor IP/UA/referer recorded for the
	// member's links so personal data does not outlive the account.
	AnonymizeClickLogs(shortURLIDs []uint64) (int64, error)
}

type memberPurgeRepo struct {
	adminDB  *gorm.DB
	publicDB *gorm.DB
}

func NewMemberPurgeRepo(adminDB, publicDB *gorm.DB) MemberPurgeRepo {
	return &memberPurgeRepo{adminDB: adminDB, publicDB: publicDB}
}

// MemberLinkRef is the (id, uid) pair needed to reach both the admin-side rows
// (click_logs keyed by id) and the public mirror (keyed by uid).
type MemberLinkRef struct {
	ID  uint64
	UID string
}

func (r *memberPurgeRepo) MemberLinks(memberID uint64) ([]MemberLinkRef, error) {
	if r.adminDB == nil {
		return nil, nil
	}
	var rows []MemberLinkRef
	err := r.adminDB.Table("short_urls").
		Select("id, uid").
		Where("member_id = ?", memberID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *memberPurgeRepo) PurgePublicLinks(uids []string) (int64, error) {
	if r.publicDB == nil || len(uids) == 0 {
		return 0, nil
	}
	res := r.publicDB.Table("wjoy_log").
		Where("uid IN ?", uids).
		Update("status", 0)
	return res.RowsAffected, res.Error
}

func (r *memberPurgeRepo) PurgeAdminLinks(memberID uint64) (int64, error) {
	if r.adminDB == nil {
		return 0, nil
	}
	// Disable + detach in one statement. GORM's soft delete is not used here on
	// purpose: a soft-deleted row keeps member_id and could be resurrected by
	// the admin recycle-bin restore, which would hand the links back to a
	// deleted (possibly id-reused) account.
	res := r.adminDB.Table("short_urls").
		Where("member_id = ?", memberID).
		Updates(map[string]interface{}{"status": 0, "member_id": nil})
	return res.RowsAffected, res.Error
}

func (r *memberPurgeRepo) AnonymizeClickLogs(shortURLIDs []uint64) (int64, error) {
	if r.adminDB == nil || len(shortURLIDs) == 0 {
		return 0, nil
	}
	res := r.adminDB.Table("click_logs").
		Where("short_url_id IN ?", shortURLIDs).
		Updates(map[string]interface{}{
			"ip":         "0.0.0.0",
			"user_agent": "",
			"referer":    "",
		})
	return res.RowsAffected, res.Error
}
