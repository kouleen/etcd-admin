package router

import (
	"context"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/kouleen/etcd-admin/web"
)

// embeddedStatic 处理 /static/*filepath 请求，资源全部来自编译期内嵌的 web.FS。
// 同时注册 GET 与 HEAD，行为与原 Hertz StaticFS 保持一致。
func embeddedStatic(_ context.Context, ctx *app.RequestContext) {
	// path.Join 会清洗 "/../" 片段，配合 embed.FS 自身的路径校验杜绝目录穿越
	rel := path.Join("static", strings.TrimPrefix(ctx.Param("filepath"), "/"))
	data, err := web.FS.ReadFile(rel)
	if err != nil {
		ctx.AbortWithMsg("Cannot open requested path", consts.StatusNotFound)
		return
	}
	ctx.Header("Content-Type", embeddedContentType(rel, data))
	ctx.Response.Header.SetContentLength(len(data))
	// HEAD 请求只回响应头，不发送响应体
	if !ctx.IsHead() {
		_, _ = ctx.Write(data)
	}
}

// writeEmbeddedHTML 输出内嵌的入口 HTML 页面（entry.html / index.html）。
func writeEmbeddedHTML(ctx *app.RequestContext, name string) {
	data, err := web.FS.ReadFile(name)
	if err != nil {
		ctx.String(consts.StatusInternalServerError, "前端页面未找到: %s", name)
		return
	}
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	// HTML 入口页始终重新校验：页面中带 ?v= 版本号的资源引用必须保持新鲜，
	// 否则浏览器沿用旧 HTML 会继续请求旧版本资源，长缓存的版本号失效机制就会断裂
	ctx.Header("Cache-Control", "no-cache")
	ctx.Response.Header.SetContentLength(len(data))
	if !ctx.IsHead() {
		_, _ = ctx.Write(data)
	}
}

// embeddedContentType 优先按扩展名取 MIME（Go 内置常见类型，不依赖容器内 mime.types），
// 兜底使用内容嗅探。
func embeddedContentType(name string, data []byte) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return http.DetectContentType(data)
}
