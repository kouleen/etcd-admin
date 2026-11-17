// Package web 通过 go:embed 在编译期将前端静态资源打入二进制，
// 部署时只需分发单个可执行文件，运行时不再依赖工作目录下的 web/ 目录。
package web

import "embed"

// FS 内嵌全部前端资源：static/ 目录（CSS/JS）与两个入口 HTML。
// 显式列举嵌入模式，index.html.hertz.gz 等本地构建产物不会被打入二进制。
//
//go:embed static entry.html index.html
var FS embed.FS
