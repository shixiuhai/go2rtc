# RTSP to FLV Conversion Module

将 RTSP 流转换为 FLV 格式，供浏览器播放。

## 功能特性

- **原生流集成**: 使用 go2rtc 内置流管理系统
- **任务管理**: 创建、删除、保活转换任务
- **自动过期**: 180 秒无保活自动清理任务
- **流式传输**: HTTP FLV 流式输出
- **最大流数量限制**: 防止资源耗尽
- **零额外进程**: 复用 go2rtc 内部连接，无需额外 FFmpeg 进程

## 配置

```yaml
conversion:
  max_streams: 50  # 最大同时转换流数量，默认 50
```

## 架构说明

```
RTSP 摄像头
    │
    ▼
go2rtc RTSP 客户端 (内部连接)
    │
    ▼
go2rtc Stream (流路由)
    │
    ├──► RTSP 服务器 (rtsp://localhost:8554/conv_xxx)
    │
    └──► FLV Consumer (HTTP FLV 输出)
            │
            ▼
        前端 flv.js 播放
```

## API 接口

### 1. 添加转推任务

**请求**
```http
POST /api/rtsp/flv/add
Content-Type: application/json

{
  "path": "rtsp://admin:password@192.168.1.100:554/stream",
  "cameraId": "可选，摄像头 ID"
}
```

**响应**
```json
{
  "code": 200,
  "result": "rtsp 流转换成功",
  "path": "rtsp://...",
  "taskId": "a1b2c3d4e5f6g7h8",
  "flvUrl": "/flv/a1b2c3d4e5f6g7h8.flv"
}
```

### 2. 删除转推任务

**请求**
```http
POST /api/rtsp/flv/delete
Content-Type: application/json

{
  "taskId": "a1b2c3d4e5f6g7h8"
}
```

**响应**
```json
{
  "code": 200,
  "result": "关闭 a1b2c3d4e5f6g7h8 任务成功"
}
```

### 3. 保活接口

**请求**
```http
POST /api/rtsp/flv/keepLive
Content-Type: application/json

{
  "taskId": "a1b2c3d4e5f6g7h8"
}
```

**响应**
```json
{
  "code": 200,
  "result": "a1b2c3d4e5f6g7h8 保活成功"
}
```

### 4. 查看任务列表

**请求**
```http
GET /api/rtsp/flv/list
```

**响应**
```json
{
  "code": 200,
  "result": "查看转换流列表成功",
  "taskDict": {...},
  "taskLiveDict": {...},
  "num": 5
}
```

### 5. 健康检查

**请求**
```http
GET /api/health
```

**响应**
```json
{
  "status": "ok",
  "streams": 5
}
```

### 6. FLV 流播放

**请求**
```http
GET /flv/{taskId}.flv
```

**前端播放示例**
```javascript
// 使用 flv.js 播放
const flvPlayer = flvjs.createPlayer({
  type: 'flv',
  url: 'http://localhost:1984/flv/a1b2c3d4e5f6g7h8.flv'
});
flvPlayer.attachMediaElement(document.getElementById('videoElement'));
flvPlayer.load();
flvPlayer.play();
```

## 保活机制

- **保活间隔**: 客户端需每 30 秒调用一次保活接口
- **自动过期**: 180 秒无保活任务自动删除
- **主动关闭**: 前端页面关闭前调用删除接口释放资源

## 前端使用示例

```javascript
class StreamManager {
  constructor() {
    this.taskId = null;
    this.keepAliveTimer = null;
  }

  async startStream(rtspUrl) {
    // 1. 创建转换任务
    const res = await fetch('/api/rtsp/flv/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: rtspUrl })
    });
    const data = await res.json();
    
    if (data.code === 200) {
      this.taskId = data.taskId;
      
      // 2. 播放 FLV 流
      const player = flvjs.createPlayer({
        type: 'flv',
        url: `http://localhost:1984/flv/${data.taskId}.flv`
      });
      player.attachMediaElement(document.getElementById('video'));
      player.load();
      player.play();
      
      // 3. 启动保活定时器（每 25 秒一次）
      this.startKeepAlive();
      
      return data;
    }
    throw new Error(data.result);
  }

  startKeepAlive() {
    this.keepAliveTimer = setInterval(async () => {
      await fetch('/api/rtsp/flv/keepLive', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ taskId: this.taskId })
      });
    }, 25000); // 25 秒，留 5 秒余量
  }

  async stopStream() {
    // 清除保活定时器
    if (this.keepAliveTimer) {
      clearInterval(this.keepAliveTimer);
    }
    
    // 删除任务
    if (this.taskId) {
      await fetch('/api/rtsp/flv/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ taskId: this.taskId })
      });
      this.taskId = null;
    }
  }
}

// 页面关闭时清理资源
window.addEventListener('beforeunload', () => {
  streamManager.stopStream();
});
```

## 与独立 FFmpeg 方案对比

| 特性 | 独立 FFmpeg 方案 | go2rtc 原生方案 |
|------|----------------|----------------|
| 进程开销 | 每流 1 个 FFmpeg 进程 | 无额外进程 |
| 连接复用 | 每个客户端独立连接 | 多客户端共享 RTSP 连接 |
| 硬件加速 | 需手动配置 | 复用 go2rtc 配置 |
| 日志监控 | 独立日志 | 集成 go2rtc 统计 |
| 资源占用 | 高 | 低 |

## 注意事项

1. 确保 RTSP 服务器已启用（默认端口 8554）
2. FLV 流仅在请求时传输数据，无连接自动断开
3. 合理设置 `max_streams` 避免资源耗尽
4. 前端必须使用 flv.js 或类似库播放 FLV 流
