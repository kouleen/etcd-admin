package handler

import (
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

var (
	errEmptyKey       = errors.New("key 不能为空")
	errKeyNotFound    = errors.New("key 不存在")
	errInvalidTTL     = errors.New("ttl 必须大于 0")
	errEmptyEndpoints = errors.New("endpoints 不能为空")
	errInvalidBody    = errors.New("请求参数不合法")
	errNotConnected   = errors.New("etcd 未连接，请先在连接入口页建立连接")
)

// APIResp 统一响应结构
type APIResp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func ok(c *app.RequestContext, data interface{}) {
	c.JSON(consts.StatusOK, APIResp{Code: 0, Message: "ok", Data: data})
}

func fail(c *app.RequestContext, status int, err error) {
	c.JSON(status, APIResp{Code: status, Message: err.Error()})
}
