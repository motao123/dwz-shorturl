// Command createadmin provisions the first administrator account (or resets an
// existing one) directly through the repository layer, so a deployment that is
// not using the Docker installer never has to fall back to a seeded default
// credential.
//
// The admin schema deliberately seeds no user row: a fixed bcrypt hash in the
// public repository is a known-default-credential vulnerability. This command is
// the supported way to create the first account on a fresh install, and to
// recover access afterwards.
//
// Usage (run from the backend dir):
//
//	./createadmin -username=admin -password='...'                 # create (idempotent)
//	./createadmin -username=admin -password='...' -reset          # force reset the password
//	./createadmin -username=admin -email=me@example.com -password='...'
//
// The password may also be supplied as DWZ_ADMIN_PASSWORD to keep it out of the
// shell history; the flag takes precedence when both are present.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// minPasswordLength matches the validation the admin API applies, so a password
// accepted here is always accepted by the login flow's own rules.
const minPasswordLength = 6

func main() {
	var (
		username    = flag.String("username", "admin", "administrator username")
		email       = flag.String("email", "", "administrator email (defaults to <username>@localhost)")
		password    = flag.String("password", "", "administrator password (or set DWZ_ADMIN_PASSWORD)")
		displayName = flag.String("display-name", "系统管理员", "display name")
		reset       = flag.Bool("reset", false, "reset the password when the account already exists")
		configPath  = flag.String("config", "configs/config.yaml", "path to the config yaml")
	)
	flag.Parse()

	if *password == "" {
		*password = os.Getenv("DWZ_ADMIN_PASSWORD")
	}
	if *password == "" {
		fmt.Fprintln(os.Stderr, "error: a password is required (use -password or DWZ_ADMIN_PASSWORD)")
		os.Exit(1)
	}
	if utf8.RuneCountInString(*password) < minPasswordLength {
		fmt.Fprintf(os.Stderr, "error: password must be at least %d characters\n", minPasswordLength)
		os.Exit(1)
	}
	*username = strings.TrimSpace(*username)
	if *username == "" {
		fmt.Fprintln(os.Stderr, "error: username must not be empty")
		os.Exit(1)
	}
	if *email == "" {
		*email = *username + "@localhost"
	}

	if err := config.Init(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}
	cfg := config.Get()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port,
		cfg.Database.DBName, cfg.Database.Charset)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect admin db failed:", err)
		os.Exit(1)
	}

	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	svc := newAdminProvisioner(userRepo, roleRepo)

	action, err := svc.provision(*username, *email, *password, *displayName, *reset)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("%s 管理员 %q 完成（%s）\n", action, *username, cfg.Database.DBName)
	fmt.Println("请立即登录管理台并修改为只有你知道的密码；建议同时开启两步验证。")
}

// provisioner holds the two repositories the account needs: the user row itself
// and the super_admin role it must be granted.
type provisioner struct {
	users repository.UserRepo
	roles repository.RoleRepo
}

func newAdminProvisioner(users repository.UserRepo, roles repository.RoleRepo) *provisioner {
	return &provisioner{users: users, roles: roles}
}

// provision creates the account, or resets its password when it already exists
// and reset is set. Reported action is "创建" / "重置" / "跳过".
func (p *provisioner) provision(username, email, password, displayName string, reset bool) (string, error) {
	hash, err := pkg.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	existing, err := p.users.FindByUsername(username)
	switch {
	case err == nil && existing != nil:
		if !reset {
			return "跳过（已存在，未改动密码）", nil
		}
		if err := p.users.UpdatePassword(existing.ID, hash); err != nil {
			return "", fmt.Errorf("reset password: %w", err)
		}
		if err := p.grantSuperAdmin(existing.ID); err != nil {
			return "", err
		}
		return "重置", nil
	case err != nil && err != gorm.ErrRecordNotFound:
		return "", fmt.Errorf("lookup user: %w", err)
	}

	user := &model.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Status:       1,
	}
	if err := p.users.Create(user); err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	if err := p.grantSuperAdmin(user.ID); err != nil {
		return "", err
	}
	return "创建", nil
}

// grantSuperAdmin attaches the super_admin role. It is idempotent, and reports a
// clear error when the role seed is missing rather than leaving an administrator
// with no permissions.
func (p *provisioner) grantSuperAdmin(userID uint64) error {
	role, err := p.roles.FindByName("super_admin")
	if err != nil {
		return fmt.Errorf("super_admin role not found — run the migrations first (cmd/migrate -all): %w", err)
	}
	if err := p.users.SetRoles(userID, []uint64{role.ID}); err != nil {
		return fmt.Errorf("grant super_admin: %w", err)
	}
	return nil
}
