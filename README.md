# TTS Service

一个基于 Go 实现的 Edge-TTS 服务端。

## 项目介绍

本项目提供一个轻量的 HTTP TTS 服务，使用 Go 调用 Edge TTS 生成语音，并返回 `mp3` 音频流。

### 功能特性

- 使用 Go 实现服务端
- 仅严格匹配 `GET /?t={msg}` 时返回语音
- 其他所有请求统一返回默认 HTML 状态页
- 支持中文语音，默认音色为 `zh-CN-XiaoxiaoNeural`
- 支持本地日志输出
- 支持本地缓存：相同文本优先返回已缓存的 mp3 文件
- 支持通过 `config.yaml` 配置端口、页面标题、缓存开关和缓存目录
- 支持打包 Windows / Linux 可执行文件

## 路由规则

### 返回 mp3
仅当请求满足以下条件时返回 mp3：

- 请求方法为 `GET`
- 路径为 `/`
- 查询参数中只有一个 `t`
- `t` 非空

示例：

```text
/?t=你好
```

### 返回 HTML
以下请求都会返回 HTML 状态页：

```text
/
/abc
/?x=1
/?t=你好&x=1
```

## 项目结构

```text
TTS/
├─ main.go
├─ go.mod
├─ go.sum
├─ config.yaml
├─ build.bat
└─ README.md
```

## 环境要求

- Go 1.24 或更高版本
- 可访问 Edge TTS 服务的网络环境

## 安装依赖

在项目目录执行：

```bash
go mod tidy
```

## 配置说明

配置文件为：

```text
config.yaml
```

默认内容：

```yaml
port: 8080
title: TTS Service
enable_cache: true
cache_dir: cache
```

### 配置项说明

- `port`: 服务监听端口
- `title`: HTML 页面标题
- `enable_cache`: 是否启用 mp3 缓存
- `cache_dir`: 缓存目录

### 环境变量

可选环境变量：

- `EDGE_TTS_VOICE`: 指定语音模型，默认 `zh-CN-XiaoxiaoNeural`
- `CONFIG_PATH`: 指定配置文件路径

示例：

```bash
set EDGE_TTS_VOICE=zh-CN-XiaoxiaoNeural
set CONFIG_PATH=F:\XArrPay\TTS\config.yaml
```

## 本地运行

### 方式一：直接运行源码

```bash
go run .
```

启动后默认访问：

```text
http://127.0.0.1:8080/
```

测试语音：

```text
http://127.0.0.1:8080/?t=你好
```

### 方式二：先编译再运行

```bash
go build -o tts.exe .
./tts.exe
```

## 部署教程

### Windows 部署

#### 1. 打包

直接运行：

```bat
build.bat
```

打包后会在 `build` 目录生成：

```text
build/tts-windows-amd64.exe
build/tts-linux-amd64
build/config.yaml
```

#### 2. 启动

进入 `build` 目录，双击：

```text
tts-windows-amd64.exe
```

或命令行启动：

```bat
cd build
tts-windows-amd64.exe
```

#### 3. 访问

```text
http://127.0.0.1:8080/
http://127.0.0.1:8080/?t=你好
```

#### 4. 日志

程序运行后会在可执行文件同目录写入日志：

```text
tts.log
```

#### 5. 缓存

启用缓存后，缓存文件默认保存在：

```text
cache/
```

### Linux 部署

#### 1. 编译 Linux 版本

Windows 下可通过：

```bat
build.bat
```

也可以在 Linux 服务器上直接执行：

```bash
go build -o tts-linux-amd64 .
```

#### 2. 上传文件

部署到 Linux 时建议同时上传：

- `tts-linux-amd64`
- `config.yaml`

#### 3. 添加执行权限

```bash
chmod +x tts-linux-amd64
```

#### 4. 运行

```bash
./tts-linux-amd64
```

#### 5. 后台运行（可选）

```bash
nohup ./tts-linux-amd64 > /dev/null 2>&1 &
```

#### 6. 访问

```text
http://服务器IP:8080/
http://服务器IP:8080/?t=你好
```

## 缓存说明

启用缓存后：

1. 首次请求某个 `msg`，会调用 TTS 生成 mp3
2. 生成完成后保存到本地缓存目录
3. 后续相同 `msg` 请求，直接返回缓存文件

缓存文件名会根据文本内容生成哈希值，因此：

- 相同文本 -> 命中同一个缓存文件
- 不同文本 -> 生成不同缓存文件

## 日志说明

服务日志包含：

- 服务启动日志
- 请求访问日志
- TTS 合成开始/成功/失败日志
- 缓存命中/未命中/写入日志

日志文件默认输出到：

```text
tts.log
```

## HTML 状态页

默认 HTML 页面包含：

- 服务状态
- 当前监听地址
- 默认语音
- 启动时间
- 运行时长
- 当前时间
- 当前请求信息
- 缓存状态

## 接口示例

### 成功返回 mp3

```text
GET /?t=你好，欢迎使用TTS服务
```

### 返回 HTML

```text
GET /
GET /abc
GET /?foo=bar
GET /?t=你好&x=1
```

## 常见问题

### 1. 双击 exe 一闪而过
请确认：

- `config.yaml` 与 exe 在同一目录
- 当前目录可写入 `tts.log`
- 端口未被占用

如有问题，请查看：

```text
tts.log
```

### 2. 中文请求是否支持
支持。示例：

```text
/?t=你好
/?t=欢迎使用语音服务
```

### 3. 为什么某些请求返回 HTML 而不是 mp3
因为服务采用严格匹配模式，只有：

```text
GET /?t={msg}
```

才返回 mp3。

## 许可证

当前项目未单独声明许可证，如需开源发布，建议补充 LICENSE 文件。
