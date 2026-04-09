# go2rtc Windows 一键安装包

## 文件说明

```
dist/
├── go2rtc_windows_amd64.exe    # go2rtc 主程序 (Windows amd64)
├── go2rtc_linux_amd64          # go2rtc 主程序 (Linux amd64)
├── go2rtc_darwin_amd64         # go2rtc 主程序 (macOS Intel)
├── go2rtc_darwin_arm64         # go2rtc 主程序 (macOS M1/M2)
├── install-service.bat         # 【Windows】一键安装服务脚本
├── uninstall-service.bat       # 【Windows】卸载服务脚本
├── manage-service.bat          # 【Windows】管理服务脚本
└── README.md                   # 本文件
```

---

## Windows 快速开始

### 1. 安装服务

**右键** `install-service.bat` → **以管理员身份运行**

脚本会自动：
- ✅ 检查管理员权限
- ✅ 创建默认配置文件
- ✅ 下载 NSSM 服务管理工具
- ✅ 注册 Windows 服务
- ✅ 配置开机自启动
- ✅ 自动启动服务

### 2. 验证安装

```cmd
# 方法 1: 使用管理脚本
manage-service.bat
选择 5 (测试 API)

# 方法 2: 浏览器访问
http://localhost:1984/api/health

# 方法 3: 命令行
curl http://localhost:1984/api/health
```

### 3. 管理服务

**运行** `manage-service.bat`

```
1. 启动服务
2. 停止服务
3. 重启服务
4. 查看日志
5. 测试 API
6. 退出
```

### 4. 卸载服务

**右键** `uninstall-service.bat` → **以管理员身份运行**

---

## 配置文件

安装后自动生成 `go2rtc.yaml`：

```yaml
api:
  listen: ":1984"
  origin: "*"

rtsp:
  port: 8554

conversion:
  max_streams: 50
```

**修改配置后重启服务**：
```
manage-service.bat → 3 (重启服务)
```

---

## 使用示例

### 添加 RTSP 转 FLV 任务

```bash
curl -X POST http://localhost:1984/api/rtsp/flv/add \
  -H "Content-Type: application/json" \
  -d "{\"path\": \"rtsp://192.168.6.2:8080/h264.sdp\"}"
```

**响应**：
```json
{
  "code": 200,
  "taskId": "a1b2c3d4e5f6g7h8",
  "flvUrl": "/flv/a1b2c3d4e5f6g7h8.flv"
}
```

### 播放 FLV 流

浏览器访问：
```
http://localhost:1984/flv/a1b2c3d4e5f6g7h8.flv
```

或使用 HTML 播放器：
```html
<video id="video" controls></video>
<script src="https://cdn.jsdelivr.net/npm/flv.js/dist/flv.min.js"></script>
<script>
  const player = flvjs.createPlayer({
    type: 'flv',
    url: 'http://localhost:1984/flv/a1b2c3d4e5f6g7h8.flv'
  });
  player.attachMediaElement(document.getElementById('video'));
  player.load();
  player.play();
</script>
```

---

## Linux/macOS 使用

### 1. 赋予执行权限

```bash
chmod +x go2rtc_linux_amd64
```

### 2. 运行

```bash
./go2rtc_linux_amd64 -config go2rtc.yaml
```

### 3. systemd 服务（Linux）

创建 `/etc/systemd/system/go2rtc.service`：

```ini
[Unit]
Description=go2rtc streaming server
After=network.target

[Service]
Type=simple
User=go2rtc
WorkingDirectory=/opt/go2rtc
ExecStart=/opt/go2rtc/go2rtc_linux_amd64 -config /opt/go2rtc/go2rtc.yaml
Restart=always

[Install]
WantedBy=multi-user.target
```

**启用服务**：
```bash
sudo systemctl enable go2rtc
sudo systemctl start go2rtc
sudo systemctl status go2rtc
```

---

## 故障排查

### 服务无法启动

1. 检查日志：
   ```
   go2rtc-service.log
   go2rtc-error.log
   ```

2. 检查端口占用：
   ```cmd
   netstat -ano | findstr 1984
   ```

3. 重新安装：
   ```
   运行 uninstall-service.bat 卸载
   运行 install-service.bat 重新安装
   ```

### CORS 错误

确保配置文件中有：
```yaml
api:
  origin: "*"
```

### Mixed Content 错误

HTTPS 页面不能请求 HTTP 资源，需要：
- 使用反向代理（Nginx）配置 HTTPS
- 或 HTTP 页面访问 HTTP API

---

## 技术支持

- 项目地址：https://github.com/AlexxIT/go2rtc
- 文档：https://github.com/AlexxIT/go2rtc/wiki
