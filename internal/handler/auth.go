package handler

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/kouleen/etcd-admin/internal/etcdcli"
)

// AuthHandler 处理 etcd 认证、用户与角色管理。
type AuthHandler struct {
	manager *etcdcli.Manager
}

func NewAuthHandler(m *etcdcli.Manager) *AuthHandler {
	return &AuthHandler{manager: m}
}

type userRow struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type permRow struct {
	Key      string `json:"key"`
	RangeEnd string `json:"rangeEnd"`
	Prefix   bool   `json:"prefix"`
	Perm     string `json:"perm"`
}

type roleRow struct {
	Name        string    `json:"name"`
	Permissions []permRow `json:"permissions"`
}

// Status 查询认证开关状态。
// GET /api/auth/status
func (h *AuthHandler) Status(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	resp, err := cli.Auth.AuthStatus(c)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, map[string]interface{}{"enabled": resp.Enabled, "authRevision": resp.AuthRevision})
}

// Enable 开启认证。
// POST /api/auth/enable
func (h *AuthHandler) Enable(c context.Context, ctx *app.RequestContext) {
	if _, err := h.manager.Get().Auth.AuthEnable(c); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// Disable 关闭认证。
// POST /api/auth/disable
func (h *AuthHandler) Disable(c context.Context, ctx *app.RequestContext) {
	if _, err := h.manager.Get().Auth.AuthDisable(c); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// Users 列出全部用户及其角色。
// GET /api/auth/users
func (h *AuthHandler) Users(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	list, err := cli.UserList(c)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	rows := make([]userRow, 0, len(list.Users))
	for _, name := range list.Users {
		row := userRow{Name: name, Roles: []string{}}
		if detail, err := cli.UserGet(c, name); err == nil {
			row.Roles = detail.Roles
		}
		rows = append(rows, row)
	}
	ok(ctx, map[string]interface{}{"items": rows})
}

// UserCreate 创建用户。
// POST /api/auth/users  body: {name, password}
func (h *AuthHandler) UserCreate(c context.Context, ctx *app.RequestContext) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := ctx.BindJSON(&body); err != nil || body.Name == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().UserAdd(c, body.Name, body.Password); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// UserDelete 删除用户。
// DELETE /api/auth/users?name=
func (h *AuthHandler) UserDelete(c context.Context, ctx *app.RequestContext) {
	name := string(ctx.Query("name"))
	if name == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().UserDelete(c, name); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// UserChangePassword 修改用户密码。
// POST /api/auth/users/password  body: {name, password}
func (h *AuthHandler) UserChangePassword(c context.Context, ctx *app.RequestContext) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := ctx.BindJSON(&body); err != nil || body.Name == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().UserChangePassword(c, body.Name, body.Password); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// GrantRole 给用户授予角色。
// POST /api/auth/users/roles  body: {user, role}
func (h *AuthHandler) GrantRole(c context.Context, ctx *app.RequestContext) {
	var body struct {
		User string `json:"user"`
		Role string `json:"role"`
	}
	if err := ctx.BindJSON(&body); err != nil || body.User == "" || body.Role == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().UserGrantRole(c, body.User, body.Role); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// RevokeRole 撤销用户角色。
// DELETE /api/auth/users/roles?user=&role=
func (h *AuthHandler) RevokeRole(c context.Context, ctx *app.RequestContext) {
	user := string(ctx.Query("user"))
	role := string(ctx.Query("role"))
	if user == "" || role == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().UserRevokeRole(c, user, role); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// Roles 列出全部角色及其权限。
// GET /api/auth/roles
func (h *AuthHandler) Roles(c context.Context, ctx *app.RequestContext) {
	cli := h.manager.Get()
	list, err := cli.RoleList(c)
	if err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	rows := make([]roleRow, 0, len(list.Roles))
	for _, name := range list.Roles {
		row := roleRow{Name: name, Permissions: []permRow{}}
		if detail, err := cli.RoleGet(c, name); err == nil && detail.Perm != nil {
			for _, p := range detail.Perm {
				row.Permissions = append(row.Permissions, permRow{
					Key:      string(p.Key),
					RangeEnd: string(p.RangeEnd),
					Prefix:   len(p.RangeEnd) > 0,
					Perm:     p.PermType.String(),
				})
			}
		}
		rows = append(rows, row)
	}
	ok(ctx, map[string]interface{}{"items": rows})
}

// RoleCreate 创建角色。
// POST /api/auth/roles  body: {name}
func (h *AuthHandler) RoleCreate(c context.Context, ctx *app.RequestContext) {
	var body struct {
		Name string `json:"name"`
	}
	if err := ctx.BindJSON(&body); err != nil || body.Name == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().RoleAdd(c, body.Name); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// RoleDelete 删除角色。
// DELETE /api/auth/roles?name=
func (h *AuthHandler) RoleDelete(c context.Context, ctx *app.RequestContext) {
	name := string(ctx.Query("name"))
	if name == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().RoleDelete(c, name); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// GrantPermission 给角色授予键权限。
// POST /api/auth/roles/permissions  body: {role, key, prefix, perm}
func (h *AuthHandler) GrantPermission(c context.Context, ctx *app.RequestContext) {
	var body struct {
		Role   string `json:"role"`
		Key    string `json:"key"`
		Prefix bool   `json:"prefix"`
		Perm   string `json:"perm"`
	}
	if err := ctx.BindJSON(&body); err != nil || body.Role == "" || body.Key == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	permType, err := clientv3.StrToPermissionType(body.Perm)
	if err != nil {
		fail(ctx, consts.StatusBadRequest, err)
		return
	}
	rangeEnd := ""
	if body.Prefix {
		rangeEnd = clientv3.GetPrefixRangeEnd(body.Key)
	}
	if _, err := h.manager.Get().RoleGrantPermission(c, body.Role, body.Key, rangeEnd, permType); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}

// RevokePermission 撤销角色的键权限。
// DELETE /api/auth/roles/permissions?role=&key=&rangeEnd=
func (h *AuthHandler) RevokePermission(c context.Context, ctx *app.RequestContext) {
	role := string(ctx.Query("role"))
	key := string(ctx.Query("key"))
	rangeEnd := string(ctx.Query("rangeEnd"))
	if role == "" || key == "" {
		fail(ctx, consts.StatusBadRequest, errInvalidBody)
		return
	}
	if _, err := h.manager.Get().RoleRevokePermission(c, role, key, rangeEnd); err != nil {
		fail(ctx, consts.StatusInternalServerError, err)
		return
	}
	ok(ctx, nil)
}
