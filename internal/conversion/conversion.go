package conversion

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/rs/zerolog"
)

type TaskInfo struct {
	TaskID     string    `json:"taskId"`
	RTSPPath   string    `json:"rtspPath"`
	FLVURL     string    `json:"flvUrl"`
	StreamName string    `json:"-"`
	CreateTime time.Time `json:"createTime"`
	LastKeep   time.Time `json:"lastKeep"`
	Expires    int64     `json:"expires"`
}

type StreamManager struct {
	tasks      map[string]*TaskInfo
	mu         sync.RWMutex
	maxStreams int
	ctx        context.Context
	cancel     context.CancelFunc
}

var (
	manager *StreamManager
	log     zerolog.Logger
)

const (
	DefaultMaxStreams = 50
	ForceExpireTime   = 180
)

func Init() {
	var cfg struct {
		Mod struct {
			MaxStreams int `yaml:"max_streams"`
		} `yaml:"conversion"`
	}

	cfg.Mod.MaxStreams = DefaultMaxStreams
	app.LoadConfig(&cfg)

	log = app.GetLogger("conversion")

	manager = &StreamManager{
		tasks:      make(map[string]*TaskInfo),
		maxStreams: cfg.Mod.MaxStreams,
	}
	manager.ctx, manager.cancel = context.WithCancel(context.Background())

	go manager.cleanupWorker()

	api.HandleFunc("api/rtsp/flv/add", apiAddTask)
	api.HandleFunc("api/rtsp/flv/delete", apiDeleteTask)
	api.HandleFunc("api/rtsp/flv/keepLive", apiKeepAlive)
	api.HandleFunc("api/rtsp/flv/list", apiListTasks)
	api.HandleFunc("api/health", apiHealth)

	http.HandleFunc("/flv/", apiFLV)
}

func GenerateTaskID(rtspPath string) string {
	hash := md5.Sum([]byte(rtspPath))
	return hex.EncodeToString(hash[:])[:16]
}

func (m *StreamManager) AddTask(rtspPath, flvURL, taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.tasks) >= m.maxStreams {
		return false
	}

	if _, exists := m.tasks[taskID]; exists {
		return true
	}

	streamName := "conv_" + taskID
	now := time.Now()
	info := &TaskInfo{
		TaskID:     taskID,
		RTSPPath:   rtspPath,
		FLVURL:     flvURL,
		StreamName: streamName,
		CreateTime: now,
		LastKeep:   now,
		Expires:    now.Unix() + ForceExpireTime,
	}

	if err := m.createStream(streamName, rtspPath); err != nil {
		log.Error().Err(err).Msg("create stream failed")
		return false
	}

	m.tasks[taskID] = info
	log.Info().Str("taskId", taskID).Str("rtsp", rtspPath).Str("stream", streamName).Msg("conversion task added")
	return true
}

func (m *StreamManager) createStream(streamName, rtspPath string) error {
	_, err := streams.Patch(streamName, rtspPath)
	if err != nil {
		return err
	}

	log.Debug().Str("stream", streamName).Msg("stream created")
	return nil
}

func (m *StreamManager) RemoveTask(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.tasks[taskID]
	if !exists {
		return false
	}

	streams.Delete(info.StreamName)
	delete(m.tasks, taskID)
	log.Info().Str("taskId", taskID).Msg("conversion task removed")
	return true
}

func (m *StreamManager) KeepAlive(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.tasks[taskID]
	if !exists {
		return false
	}

	now := time.Now()
	info.LastKeep = now
	info.Expires = now.Unix() + ForceExpireTime
	return true
}

func (m *StreamManager) GetTaskInfo(taskID string) *TaskInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

func (m *StreamManager) ListTasks() map[string]*TaskInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*TaskInfo, len(m.tasks))
	for k, v := range m.tasks {
		result[k] = v
	}
	return result
}

func (m *StreamManager) cleanupWorker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *StreamManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	for taskID, info := range m.tasks {
		if now > info.Expires {
			log.Info().Str("taskId", taskID).Msg("task expired, removing")
			streams.Delete(info.StreamName)
			delete(m.tasks, taskID)
		}
	}
}

func (m *StreamManager) Shutdown() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, info := range m.tasks {
		streams.Delete(info.StreamName)
	}
	m.tasks = make(map[string]*TaskInfo)
}

func Shutdown() {
	if manager != nil {
		manager.Shutdown()
	}
}

func GetRTSPPort() string {
	return rtsp.Port
}
