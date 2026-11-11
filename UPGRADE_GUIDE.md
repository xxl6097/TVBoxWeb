# TVBoxWeb v1.3.1 升级功能指南

## 🚀 功能概述

TVBoxWeb v1.3.1 实现了完整的基于 GitHub Release 的自动升级功能，完全复刻了 go-fs 项目的升级机制。

### 特性

- ✅ 自动检测 GitHub 最新 Release
- ✅ 多平台支持（Linux/macOS/Windows，amd64/arm64）
- ⚠️ 断点续传下载（暂未实现，弱网络建议重试；详见文末 v1.5.0 变更）
- ✅ 下载进度实时推送（SSE）
- ✅ 自动备份和恢复
- ✅ 平台架构自动匹配
- ✅ Web UI 集成

## 📋 API 端点

### 1. 获取版本信息
```http
GET /api/version
```

**响应示例**:
```json
{
  "appVersion": "v1.3.1",
  "buildTime": "2024-11-10 00:10:00",
  "osType": "darwin",
  "arch": "arm64"
}
```

### 2. 检查更新
```http
GET /api/upgrade/check
```

**响应示例（有更新）**:
```json
{
  "current": "v1.3.1",
  "enabled": true,
  "updateable": true,
  "latest": "v1.3.2",
  "release": {
    "version": "v1.3.2",
    "notes": "Release notes in markdown...",
    "publishedAt": "2024-11-10T00:00:00Z",
    "pageUrl": "https://github.com/xxl6097/TVBoxWeb/releases/tag/v1.3.2",
    "downloadUrl": "https://github.com/xxl6097/TVBoxWeb/releases/download/v1.3.2/yezhen-darwin-arm64",
    "size": 12582912
  }
}
```

**响应示例（无更新）**:
```json
{
  "current": "v1.3.1",
  "enabled": true,
  "updateable": false
}
```

### 3. 执行升级
```http
POST /api/upgrade/run
```

**响应格式**: Server-Sent Events (SSE)

**事件流示例**:
```
event: progress
data: {"stage":"checking","message":"开始升级到 v1.3.2"}

event: progress
data: {"stage":"downloading","loaded":1048576,"total":12582912,"message":"已下载 8.3%"}

event: progress
data: {"stage":"downloading","loaded":12582912,"total":12582912,"message":"已下载 100.0%"}

event: progress
data: {"stage":"verifying","message":"验证下载文件"}

event: progress
data: {"stage":"replacing","message":"替换可执行文件"}

event: done
data: {"version":"v1.3.2","message":"升级完成，重启服务后生效"}
```

**错误响应**:
```
event: error
data: {"message":"下载失败: connection timeout"}
```

## 🎨 前端集成

### Vue 3 组件示例

`UpgradePanel.vue` 组件已集成到设置页面，提供完整的升级 UI。

#### 使用方法

```vue
<template>
  <div class="settings-page">
    <!-- 其他设置 -->
    
    <div class="section-label">系统更新</div>
    <UpgradePanel />
  </div>
</template>

<script setup>
import UpgradePanel from '@/components/UpgradePanel.vue'
</script>
```

### JavaScript 示例

```javascript
// 检查更新
async function checkUpdate() {
  const res = await fetch('/api/upgrade/check')
  const data = await res.json()
  
  if (data.updateable) {
    console.log(`发现新版本: ${data.latest}`)
    return data.release
  }
  return null
}

// 执行升级（使用 EventSource 接收进度）
async function performUpgrade() {
  const eventSource = new EventSource('/api/upgrade/run')
  
  eventSource.addEventListener('progress', (e) => {
    const progress = JSON.parse(e.data)
    console.log(`${progress.stage}: ${progress.message}`)
    
    if (progress.stage === 'downloading') {
      const percent = (progress.loaded / progress.total * 100).toFixed(1)
      console.log(`下载进度: ${percent}%`)
    }
  })
  
  eventSource.addEventListener('done', (e) => {
    const data = JSON.parse(e.data)
    console.log(`升级成功: ${data.message}`)
    eventSource.close()
    
    // 提示用户重启服务
    alert('升级完成，请重启服务以应用新版本')
  })
  
  eventSource.addEventListener('error', (e) => {
    if (e.data) {
      const error = JSON.parse(e.data)
      console.error(`升级失败: ${error.message}`)
    }
    eventSource.close()
  })
}
```

## 🔧 开发者集成

### 在自己的项目中使用

```go
package main

import (
    "log"
    
    tvbox "github.com/xxl6097/TVBoxWeb"
    "github.com/xxl6097/TVBoxWeb/githubupgrade"
)

func main() {
    // 创建升级器
    upgrader, err := githubupgrade.New(githubupgrade.Options{
        Owner: "xxl6097",
        Repo:  "TVBoxWeb",
    })
    if err != nil {
        log.Printf("升级功能未启用: %v", err)
    }
    
    // 启动服务
    opts := []tvbox.Option{
        tvbox.WithDataDir("./data"),
    }
    
    if upgrader != nil {
        opts = append(opts, tvbox.WithUpgrader(upgrader))
    }
    
    if err := tvbox.ListenAndServe(":8090", opts...); err != nil {
        log.Fatal(err)
    }
}
```

## 📦 构建说明

### 编译带版本信息的二进制

```bash
#!/bin/bash

VERSION="v1.3.1"
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')
LDFLAGS="-X 'github.com/xxl6097/TVBoxWeb/internal/api.AppVersion=${VERSION}' \
         -X 'github.com/xxl6097/TVBoxWeb/internal/api.BuildTime=${BUILD_TIME}'"

# 编译当前平台
go build -ldflags "$LDFLAGS" -o tvboxweb ./cmd/tvbox

# 交叉编译
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o tvboxweb_linux_amd64 ./cmd/tvbox
GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o tvboxweb_linux_arm64 ./cmd/tvbox
GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o tvboxweb_darwin_amd64 ./cmd/tvbox
GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o tvboxweb_darwin_arm64 ./cmd/tvbox
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o tvboxweb_windows_amd64.exe ./cmd/tvbox
```

### GitHub Release 资源文件命名规范

升级功能会自动查找匹配当前平台的资源文件，命名格式：

```
TVBoxWeb_{version}_{os}_{arch}[.exe]
```

示例：
- `TVBoxWeb_1.3.1_linux_amd64`
- `TVBoxWeb_1.3.1_darwin_arm64`
- `TVBoxWeb_1.3.1_windows_amd64.exe`

## 🔒 安全性

1. **版本验证**: 升级前会重新向 GitHub API 确认版本，不信任客户端传入的 URL
2. **文件验证**: 下载后验证文件大小和可执行权限
3. **自动备份**: 替换前自动备份当前文件，失败时自动恢复
4. **超时保护**: 下载限时 5 分钟，API 请求限时 10 秒

## 📝 升级流程

1. **检查阶段** (`checking`)
   - 向 GitHub API 请求最新 Release
   - 比较版本号
   - 查找当前平台的资源文件

2. **下载阶段** (`downloading`)
   - 从 GitHub 下载资源文件
   - 实时报告下载进度
   - 支持断点续传

3. **验证阶段** (`verifying`)
   - 验证文件完整性
   - 设置执行权限（Unix 系统）

4. **替换阶段** (`replacing`)
   - 备份当前可执行文件
   - 复制新文件到当前位置
   - 失败时自动恢复备份

5. **完成** (`done`)
   - 清理临时文件
   - 通知用户重启服务

## 🐛 故障排查

### 升级功能未启用

**问题**: API 返回 `"enabled": false`

**原因**: 升级器初始化失败

**解决**: 检查日志，确认 GitHub API 可访问

### 未找到适用平台的安装包

**问题**: 检查更新时提示"未找到适用于 xxx/xxx 平台的安装包"

**原因**: GitHub Release 中没有当前平台的资源文件

**解决**: 确保 Release 包含所有平台的构建文件，文件名符合命名规范

### 下载失败

**问题**: 下载过程中断或超时

**原因**: 网络问题或 GitHub 访问受限

**解决**: 
- 检查网络连接
- 配置代理或使用 GitHub 镜像

### 替换失败

**问题**: 文件替换失败，权限错误

**原因**: 当前用户没有写权限

**解决**: 使用有权限的用户运行服务

## 📚 参考资料

- [go-fs 升级实现](https://github.com/xxl6097/go-fs)
- [GitHub API 文档](https://docs.github.com/rest)
- [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)

## 🆕 版本历史

### v1.3.1
- ✅ 实现基于 GitHub Release 的完整升级功能
- ✅ Web UI 集成（设置页面）
- ✅ SSE 实时进度推送
- ✅ 多平台支持
- ✅ 自动备份和恢复机制

---

## v1.5.0 升级器变更

- 资源名与 CI 产物对齐为 `yezhen-<os>-<arch>`，修复了此前「检查更新后始终提示未找到安装包」的问题
- 下载后会读取同版本 Release 的 `checksums.txt` 并做 SHA256 比对，校验不通过立即中止，不会替换可执行文件
- 版本比较改为 semver 数字比较，只升不降
- 替换前保留 `<exe>.backup`；新版本下次启动成功后才清理，起不来可直接用它手工回滚
- 说明：目前完整性校验已覆盖，**断点续传暂未实现**，弱网络下建议重试
