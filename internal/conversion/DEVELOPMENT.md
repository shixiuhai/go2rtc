# RTSP to FLV 转换模块 - 开发文档

> 本文档面向 Java 开发者，使用 Go 语言在 go2rtc 基础上开发的 RTSP 转 FLV 功能详解

---

## 一、功能概述

### 1.1 这个模块是做什么的？

简单说：**把 RTSP 监控摄像头视频流转换成浏览器能播放的格式**。

- **输入**: RTSP 流（如 `rtsp://192.168.1.100:554/stream`）
- **输出**: HTTP-FLV 流（浏览器 + flv.js 可播放）
- **核心价值**: 无需 FFmpeg 转码，复用 go2rtc 内部连接，资源占用低

### 1.2 为什么选择 go2rtc？

| 方案 | 资源消耗 | 延迟 | 开发复杂度 |
|------|---------|------|-----------|
| FFmpeg 独立进程 | 高（每路 50-100MB） | 中（2-5 秒） | 低 |
| go2rtc 原生转换 | 低（多路共享连接） | 低（<1 秒） | 中 |

---

## 二、架构设计

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      前端浏览器                              │
│  ┌─────────────┐     ┌─────────────┐                        │
│  │   flv.js    │◄────│  /flv/{id}.flv │                      │
│  └─────────────┘     └─────────────┘                        │
└─────────────────────────────────────────────────────────────┘
                              ▲
                              │ HTTP 长连接
                              │
┌─────────────────────────────────────────────────────────────┐
│                    go2rtc Conversion 模块                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              StreamManager (任务管理器)                │   │
│  │  ┌────────────────────────────────────────────────┐  │   │
│  │  │  tasks: map[string]*TaskInfo                   │  │   │
│  │  │  - AddTask()    - RemoveTask()                 │  │   │
│  │  │  - KeepAlive()  - cleanupWorker()              │  │   │
│  │  └────────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
│                              │                               │
│                              ▼                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              go2rtc Streams (流路由)                  │   │
│  │  streams["conv_a1b2c3d4"] ──► RTSP 连接               │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ RTSP 协议
                              ▼
                    ┌─────────────────┐
                    │   摄像头/NVR     │
                    │ rtsp://...      │
                    └─────────────────┘
```

### 2.2 核心组件说明

#### (1) StreamManager - 流管理器

**Java 类比**: 类似 Spring 中的 `@Service` + `ConcurrentHashMap`

```go
type StreamManager struct {
    tasks      map[string]*TaskInfo  // 类似 HashMap<String, TaskInfo>
    mu         sync.RWMutex          // 类似 ReentrantReadWriteLock
    maxStreams int                   // 最大流数量限制
    ctx        context.Context       // 类似线程池的 Context
    cancel     context.CancelFunc    // 关闭信号
}
```

**为什么用 RWMutex？**
- 读多写少场景（ListTasks 频繁，AddTask 少）
- `RLock()` 允许多个 goroutine 同时读
- `Lock()` 写操作独占锁

#### (2) TaskInfo - 任务信息

```go
type TaskInfo struct {
    TaskID     string    // 任务唯一标识
    RTSPPath   string    // RTSP 源地址
    FLVURL     string    // FLV 播放 URL
    StreamName string    // 内部流名称（conv_前缀）
    CreateTime time.Time // 创建时间
    LastKeep   time.Time // 最后保活时间
    Expires    int64     // 过期时间戳（秒级）
}
```

---

## 三、核心功能实现详解

### 3.1 添加任务流程

```go
// conversion.go:80-112
func (m *StreamManager) AddTask(rtspPath, flvURL, taskID string) bool {
    m.mu.Lock()        // 加写锁
    defer m.mu.Unlock()

    // 1. 检查是否超限
    if len(m.tasks) >= m.maxStreams {
        return false
    }

    // 2. 检查是否已存在（幂等性）
    if _, exists := m.tasks[taskID]; exists {
        return true
    }

    // 3. 生成内部流名称
    streamName := "conv_" + taskID

    // 4. 创建任务信息
    info := &TaskInfo{
        TaskID:     taskID,
        RTSPPath:   rtspPath,
        FLVURL:     flvURL,
        StreamName: streamName,
        CreateTime: now,
        LastKeep:   now,
        Expires:    now.Unix() + ForceExpireTime,  // 180 秒后过期
    }

    // 5. 调用 go2rtc 创建流（核心！）
    if err := m.createStream(streamName, rtspPath); err != nil {
        return false
    }

    // 6. 注册到任务列表
    m.tasks[taskID] = info
    return true
}
```

#### 关键点：`streams.Patch()` 做了什么？

```go
// conversion.go:114-122
func (m *StreamManager) createStream(streamName, rtspPath string) error {
    _, err := streams.Patch(streamName, rtspPath)
    return err
}
```

**`streams.Patch()` 的作用**（go2rtc 核心功能）:

1. **检查是否已有同名流** - 有则直接返回
2. **解析 RTSP 地址** - 验证协议是否支持
3. **创建 Stream 对象** - 内部建立 RTSP 连接
4. **注册到全局 streams 地图** - 其他模块可复用此流

**Java 类比**:
```java
// 类似这样的逻辑
Stream stream = streamRegistry.computeIfAbsent(streamName, name -> {
    return new Stream(rtspPath);  // 懒加载创建
});
```

### 3.2 FLV 流输出

```go
// api.go:190-230
func apiFLV(w http.ResponseWriter, r *http.Request) {
    // 1. 从 URL 提取 taskID
    taskID := strings.TrimPrefix(r.URL.Path, "/flv/")
    taskID = strings.TrimSuffix(taskID, ".flv")

    // 2. 查找任务
    info := manager.GetTaskInfo(taskID)
    if info == nil {
        http.Error(w, "task not found", 404)
        return
    }

    // 3. 获取内部流
    stream := streams.Get(info.StreamName)
    if stream == nil {
        http.Error(w, "stream not found", 404)
        return
    }

    // 4. 创建 FLV 消费者
    cons := flv.NewConsumer()
    cons.WithRequest(r)

    // 5. 将消费者添加到流
    if err := stream.AddConsumer(cons); err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    // 6. 设置响应头
    h := w.Header()
    h.Set("Content-Type", "video/x-flv")
    h.Set("Cache-Control", "no-cache")
    h.Set("Access-Control-Allow-Origin", "*")

    // 7. 流式传输（阻塞直到客户端断开）
    _, _ = cons.WriteTo(w)

    // 8. 清理
    stream.RemoveConsumer(cons)
}
```

#### 数据流向

```
摄像头 ──RTSP──► go2rtc Stream ──► flv.Consumer ──► HTTP Response ──► 浏览器
                    │
                    └──► 其他 Consumer (RTSP/WebRTC/HLS 等)
```

**关键点**: 一个 Stream 可以有多个 Consumer，实现**一份 RTSP 连接，多客户端共享**

### 3.3 保活机制

```go
// conversion.go:139-152
func (m *StreamManager) KeepAlive(taskID string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()

    info, exists := m.tasks[taskID]
    if !exists {
        return false
    }

    now := time.Now()
    info.LastKeep = now
    info.Expires = now.Unix() + ForceExpireTime  // 重置为 180 秒后
    return true
}
```

**为什么需要保活？**
- 前端页面关闭时可能不会调用删除接口
- 防止僵尸任务占用资源
- 类似 Java 中的 `WeakReference` + `ReferenceQueue`

### 3.4 自动清理 Worker

```go
// conversion.go:185-197
func (m *StreamManager) cleanup() {
    m.mu.Lock()
    defer m.mu.Unlock()

    now := time.Now().Unix()
    for taskID, info := range m.tasks {
        if now > info.Expires {
            log.Info().Str("taskId", taskID).Msg("task expired, removing")
            streams.Delete(info.StreamName)  // 删除内部流
            delete(m.tasks, taskID)          // 删除任务记录
        }
    }
}
```

**运行机制**:
- 每 10 秒执行一次（见 `cleanupWorker()`）
- 类似 Java 的 `ScheduledExecutorService`

---

## 四、go2rtc 核心功能使用

### 4.1 使用的 go2rtc 内部模块

| 模块 | 用途 | 源码位置 |
|------|------|---------|
| `internal/streams` | 流管理路由 | `streams.go` |
| `internal/api` | HTTP API 注册 | `api/api.go` |
| `internal/app` | 配置加载、日志 | `app/app.go` |
| `pkg/flv` | FLV 编解码 | `flv/consumer.go` |
| `internal/rtsp` | RTSP 协议处理 | `rtsp/rtsp.go` |

### 4.2 关键 API 调用

#### (1) 注册 HTTP API

```go
// conversion.go:66-72
api.HandleFunc("api/rtsp/flv/add", apiAddTask)
api.HandleFunc("api/rtsp/flv/delete", apiDeleteTask)
api.HandleFunc("api/rtsp/flv/keepLive", apiKeepAlive)
api.HandleFunc("api/rtsp/flv/list", apiListTasks)
api.HandleFunc("api/health", apiHealth)

http.HandleFunc("/flv/", apiFLV)
```

**说明**: 
- `api.HandleFunc` - 注册到 go2rtc 内部 API 路由（需要认证可配置）
- `http.HandleFunc` - 注册到标准 HTTP 服务器（公开访问）

#### (2) 创建/获取流

```go
// 创建或更新流
stream, err := streams.Patch(streamName, rtspPath)

// 获取已有流
stream := streams.Get(streamName)

// 删除流
streams.Delete(streamName)
```

#### (3) 添加消费者

```go
cons := flv.NewConsumer()
stream.AddConsumer(cons)
_, _ = cons.WriteTo(w)  // 流式写入 HTTP 响应
stream.RemoveConsumer(cons)
```

### 4.3 配置加载

```go
// conversion.go:47-62
func Init() {
    var cfg struct {
        Mod struct {
            MaxStreams int `yaml:"max_streams"`
        } `yaml:"conversion"`
    }

    cfg.Mod.MaxStreams = DefaultMaxStreams  // 默认 50
    app.LoadConfig(&cfg)                    // 从 go2rtc.yml 加载

    log = app.GetLogger("conversion")       // 获取日志实例
    // ...
}
```

**配置文件示例** (`go2rtc.yml`):
```yaml
conversion:
  max_streams: 100  # 自定义最大流数量
```

---

## 五、使用指南

### 5.1 启动服务

```bash
# 1. 编译 go2rtc（包含 conversion 模块）
go build -o go2rtc

# 2. 创建配置文件
cat > go2rtc.yml <<EOF
rtsp:
  port: 8554  # RTSP 服务器端口

conversion:
  max_streams: 50
EOF

# 3. 启动服务
./go2rtc -config go2rtc.yml
```

### 5.2 API 调用示例

#### Java 客户端

```java
import java.net.http.*;
import java.net.URI;
import com.fasterxml.jackson.databind.*;

public class ConversionClient {
    private final HttpClient httpClient = HttpClient.newHttpClient();
    private final ObjectMapper mapper = new ObjectMapper();
    private final String baseUrl = "http://localhost:8083";

    // 添加转换任务
    public TaskResult addTask(String rtspPath) throws Exception {
        var request = HttpRequest.newBuilder()
            .uri(URI.create(baseUrl + "/api/rtsp/flv/add"))
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(
                mapper.writeValueAsString(Map.of("path", rtspPath))
            ))
            .build();

        var response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        return mapper.readValue(response.body(), TaskResult.class);
    }

    // 保活任务
    public boolean keepAlive(String taskId) throws Exception {
        var request = HttpRequest.newBuilder()
            .uri(URI.create(baseUrl + "/api/rtsp/flv/keepLive"))
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(
                mapper.writeValueAsString(Map.of("taskId", taskId))
            ))
            .build();

        var response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        JsonNode node = mapper.readTree(response.body());
        return node.get("code").asInt() == 200;
    }

    // 删除任务
    public boolean deleteTask(String taskId) throws Exception {
        var request = HttpRequest.newBuilder()
            .uri(URI.create(baseUrl + "/api/rtsp/flv/delete"))
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(
                mapper.writeValueAsString(Map.of("taskId", taskId))
            ))
            .build();

        var response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        JsonNode node = mapper.readTree(response.body());
        return node.get("code").asInt() == 200;
    }

    // 获取任务列表
    public ListTasksResult listTasks() throws Exception {
        var request = HttpRequest.newBuilder()
            .uri(URI.create(baseUrl + "/api/rtsp/flv/list"))
            .GET()
            .build();

        var response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        return mapper.readValue(response.body(), ListTasksResult.class);
    }

    // DTO 类
    public static class TaskResult {
        public int code;
        public String result;
        public String taskId;
        public String flvUrl;
        public String path;
    }

    public static class ListTasksResult {
        public int code;
        public String result;
        public Map<String, TaskInfo> taskDict;
        public Map<String, Long> taskLiveDict;
        public int num;
    }

    public static class TaskInfo {
        public String taskId;
        public String rtspPath;
        public String flvUrl;
        public long expires;
    }
}
```

#### 前端播放示例

```html
<!DOCTYPE html>
<html>
<head>
    <title>RTSP 转 FLV 播放</title>
    <script src="https://cdn.jsdelivr.net/npm/flv.js/dist/flv.min.js"></script>
</head>
<body>
    <video id="videoPlayer" width="640" height="480" controls></video>
    
    <script>
        class StreamPlayer {
            constructor() {
                this.taskId = null;
                this.keepAliveTimer = null;
                this.player = null;
            }

            async start(rtspUrl) {
                // 1. 创建转换任务
                const resp = await fetch('/api/rtsp/flv/add', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({path: rtspUrl})
                });
                const data = await resp.json();

                if (data.code !== 200) {
                    throw new Error(data.result);
                }

                this.taskId = data.taskId;

                // 2. 创建播放器
                this.player = flvjs.createPlayer({
                    type: 'flv',
                    url: window.location.origin + data.flvUrl,
                    hasAudio: false,  // 根据实际调整
                    isLive: true
                });

                const videoElement = document.getElementById('videoPlayer');
                this.player.attachMediaElement(videoElement);
                this.player.load();
                this.player.play();

                // 3. 启动保活定时器（25 秒一次）
                this.startKeepAlive();

                return data;
            }

            startKeepAlive() {
                this.keepAliveTimer = setInterval(async () => {
                    try {
                        await fetch('/api/rtsp/flv/keepLive', {
                            method: 'POST',
                            headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify({taskId: this.taskId})
                        });
                    } catch (e) {
                        console.error('KeepAlive failed:', e);
                    }
                }, 25000);
            }

            async stop() {
                // 清除定时器
                if (this.keepAliveTimer) {
                    clearInterval(this.keepAliveTimer);
                }

                // 停止播放
                if (this.player) {
                    this.player.pause();
                    this.player.unload();
                    this.player.detachMediaElement();
                    this.player.destroy();
                    this.player = null;
                }

                // 删除任务
                if (this.taskId) {
                    try {
                        await fetch('/api/rtsp/flv/delete', {
                            method: 'POST',
                            headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify({taskId: this.taskId})
                        });
                    } catch (e) {
                        console.error('Delete task failed:', e);
                    }
                    this.taskId = null;
                }
            }
        }

        // 使用示例
        const player = new StreamPlayer();

        // 页面加载时启动
        player.start('rtsp://192.168.1.100:554/stream')
            .then(data => console.log('播放启动:', data))
            .catch(err => console.error('启动失败:', err));

        // 页面关闭时清理
        window.addEventListener('beforeunload', () => {
            player.stop();
        });
    </script>
</body>
</html>
```

### 5.3 cURL 测试

```bash
# 添加任务
curl -X POST http://localhost:8083/api/rtsp/flv/add \
  -H "Content-Type: application/json" \
  -d '{"path": "rtsp://192.168.1.100:554/stream"}'

# 查看任务列表
curl http://localhost:8083/api/rtsp/flv/list | jq

# 保活任务
curl -X POST http://localhost:8083/api/rtsp/flv/keepLive \
  -H "Content-Type: application/json" \
  -d '{"taskId": "a1b2c3d4e5f6g7h8"}'

# 删除任务
curl -X POST http://localhost:8083/api/rtsp/flv/delete \
  -H "Content-Type: application/json" \
  -d '{"taskId": "a1b2c3d4e5f6g7h8"}'

# 健康检查
curl http://localhost:8083/api/health
```

---

## 六、设计决策说明

### 6.1 为什么 TaskID 用 MD5？

```go
func GenerateTaskID(rtspPath string) string {
    hash := md5.Sum([]byte(rtspPath))
    return hex.EncodeToString(hash[:])[:16]
}
```

**原因**:
1. **幂等性保证** - 相同 RTSP 路径永远生成相同 ID
2. **避免重复连接** - 多次调用 `add` 返回同一任务
3. **长度适中** - 16 字符，比完整 MD5(32) 短，比 UUID 短

**Java 类比**:
```java
String taskId = MD5.md5Hex(rtspPath).substring(0, 16);
```

### 6.2 为什么过期时间设为 180 秒？

- **30 秒保活间隔** × 6 = 180 秒
- 允许客户端 5 次保活失败（网络抖动）
- 平衡资源占用和容错性

### 6.3 为什么用 HTTP-FLV 而不是 HLS？

| 特性 | HTTP-FLV | HLS |
|------|---------|-----|
| 延迟 | <1 秒 | 5-10 秒 |
| 浏览器支持 | 需 flv.js | 原生支持 |
| 实时性 | 高 | 低 |
| 适用场景 | 监控直播 | 点播/时移 |

### 6.4 为什么需要 max_streams 限制？

**防止资源耗尽**:
- 每路 RTSP 连接占用带宽和内存
- 防止恶意调用创建无限任务
- 类似 Java 线程池的 `maximumPoolSize`

---

## 七、调试与监控

### 7.1 查看日志

```bash
# 启动时开启 debug 日志
./go2rtc -config go2rtc.yml -verbose

# 日志输出示例
{"level":"info","module":"conversion","taskId":"a1b2c3d4","rtsp":"rtsp://...","stream":"conv_a1b2c3d4","message":"conversion task added"}
{"level":"info","module":"conversion","taskId":"a1b2c3d4","message":"task expired, removing"}
```

### 7.2 查看当前流

```bash
# 调用 streams API
curl http://localhost:8083/api/streams | jq

# 调用 conversion 列表 API
curl http://localhost:8083/api/rtsp/flv/list | jq
```

### 7.3 性能监控

```go
// 可添加监控指标
type Metrics struct {
    TotalTasks   int64  // 累计创建任务数
    ActiveTasks  int64  // 当前活跃任务
    FailedTasks  int64  // 失败任务数
    AvgLatency   int64  // 平均延迟
}
```

---

## 八、常见问题

### Q1: 添加任务返回 "流数量已达上限"

**解决**: 
```yaml
# 增加 max_streams
conversion:
  max_streams: 100
```

### Q2: FLV 播放卡顿

**可能原因**:
1. 网络带宽不足
2. RTSP 源本身延迟高
3. 客户端解码能力不足

**排查**:
```bash
# 测试 RTSP 源延迟
ffplay -rtsp_transport tcp rtsp://192.168.1.100:554/stream
```

### Q3: 任务自动消失

**原因**: 保活间隔超过 180 秒

**解决**: 前端确保每 25-30 秒调用一次保活接口

### Q4: 如何支持 HTTPS？

**方案**: 在 go2rtc 前加 Nginx 反向代理

```nginx
server {
    listen 443 ssl;
    location / {
        proxy_pass http://localhost:8083;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

---

## 九、扩展开发建议

### 9.1 添加 WebRTC 输出

```go
// 类似 FLV，添加 WebRTC 消费者
func apiWebRTC(w http.ResponseWriter, r *http.Request) {
    // ...
    cons := webrtc.NewConsumer()
    stream.AddConsumer(cons)
    // ...
}
```

### 9.2 添加录制功能

```go
// 在 AddTask 时同时启动录制
func (m *StreamManager) AddTaskWithRecord(...) {
    // ...
    m.startRecording(streamName)
}
```

### 9.3 添加鉴权

```go
// 在 apiAddTask 开头添加
token := r.Header.Get("Authorization")
if !auth.Validate(token) {
    http.Error(w, "unauthorized", 401)
    return
}
```

---

## 十、总结

### 核心设计思想

1. **复用 go2rtc 流管理** - 不重复造轮子
2. **懒加载 + 自动回收** - 资源高效利用
3. **幂等性设计** - 相同输入产生相同结果
4. **保活机制** - 防止资源泄漏

### 与 Java 生态对比

| Go 概念 | Java 类比 |
|--------|----------|
| goroutine | 线程池线程 |
| channel | BlockingQueue |
| sync.RWMutex | ReentrantReadWriteLock |
| context.Context | 线程池 + 中断标志 |
| defer | try-finally |
| map | HashMap |
| slice | ArrayList |

### 下一步学习建议

1. 阅读 `internal/streams/streams.go` 了解流管理
2. 阅读 `pkg/flv/consumer.go` 了解 FLV 编码
3. 尝试添加新的输出格式（如 WebRTC）
4. 添加 Prometheus 监控指标
